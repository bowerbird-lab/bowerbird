package com.bowerbird.mobile

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Handler
import android.os.Looper
import dev.oneo.nsgo.NsgoView
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.Inet4Address
import java.net.InetSocketAddress
import java.net.NetworkInterface
import java.net.Socket
import java.net.URL
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * Combines mDNS (_bowerbird._tcp) with parallel /24 HTTP subnet probing across
 * all active IPv4 interfaces (including Wi-Fi and virtual LAN / VPN tunnels).
 */
class Discovery(context: Context) {
    private val nsd = context.getSystemService(Context.CAMERA_SERVICE.let { Context.NSD_SERVICE }) as NsdManager
    private val main = Handler(Looper.getMainLooper())
    private var listener: NsdManager.DiscoveryListener? = null
    private var activeReply: NsgoView.HostReply? = null
    private var generation = 0L

    fun browse(reply: NsgoView.HostReply) {
        stop()
        val gen = ++generation
        activeReply = reply
        val foundMdns = LinkedHashMap<String, NsdServiceInfo>()
        val merged = ConcurrentHashMap<String, Map<String, Any?>>()

        // 1. Parallel subnet scan across all active IPv4 interfaces (Wi-Fi + VPN/TUN virtual LANs)
        val scanThread = Thread {
            val subnets = activeIPv4Subnets()
            if (subnets.isNotEmpty() && gen == generation) {
                val pool = Executors.newFixedThreadPool(48)
                for (prefix in subnets) {
                    for (lastOctet in 1..254) {
                        if (gen != generation) break
                        val host = "$prefix.$lastOctet"
                        pool.execute {
                            if (gen != generation) return@execute
                            probeHost(host, DEFAULT_PORT)?.let { svc ->
                                merged["$host:$DEFAULT_PORT"] = svc
                            }
                        }
                    }
                }
                pool.shutdown()
                runCatching { pool.awaitTermination(2400, TimeUnit.MILLISECONDS) }
            }
        }
        scanThread.start()

        val finish = Runnable {
            if (gen != generation) return@Runnable
            val items = foundMdns.values.toList()
            val resolvedMdns = mutableListOf<Map<String, Any?>>()
            resolveAll(gen, items, 0, resolvedMdns) {
                if (gen != generation) return@resolveAll
                for (svc in resolvedMdns) {
                    val hosts = svc["hosts"] as? List<*>
                    val h = hosts?.firstOrNull()?.toString().orEmpty()
                    val p = (svc["port"] as? Number)?.toInt() ?: DEFAULT_PORT
                    if (h.isNotEmpty()) {
                        merged["$h:$p"] = svc
                    }
                }
                // Wait briefly for subnet probe thread if still finishing
                runCatching { scanThread.join(400) }
                if (gen != generation) return@resolveAll
                activeReply = null
                reply.resolve(merged.values.toList())
                stopListenerOnly()
            }
        }

        listener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(serviceType: String) {}
            override fun onServiceFound(service: NsdServiceInfo) {
                if (gen != generation) return
                if (service.serviceType.startsWith("_bowerbird._tcp")) {
                    val key = service.serviceName ?: return
                    if (!foundMdns.containsKey(key)) {
                        foundMdns[key] = service
                    }
                }
            }
            override fun onServiceLost(service: NsdServiceInfo) {
                foundMdns.remove(service.serviceName)
            }
            override fun onDiscoveryStopped(serviceType: String) {}
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                // Even if mDNS fails (e.g. on VPN-only networks), let subnet probing finish!
            }
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {}
        }
        try {
            nsd.discoverServices("_bowerbird._tcp", NsdManager.PROTOCOL_DNS_SD, listener)
        } catch (_: Exception) {
            // Fall through to subnet probing result
        }
        main.postDelayed(finish, 2600)
    }

    private fun activeIPv4Subnets(): List<String> {
        val out = LinkedHashSet<String>()
        runCatching {
            val ifaces = NetworkInterface.getNetworkInterfaces() ?: return emptyList()
            while (ifaces.hasMoreElements()) {
                val iface = ifaces.nextElement()
                if (!iface.isUp || iface.isLoopback) continue
                val addrs = iface.inetAddresses
                while (addrs.hasMoreElements()) {
                    val addr = addrs.nextElement()
                    if (addr is Inet4Address && !addr.isLoopbackAddress) {
                        val ip = addr.hostAddress ?: continue
                        val parts = ip.split('.')
                        if (parts.size == 4) {
                            out.add("${parts[0]}.${parts[1]}.${parts[2]}")
                        }
                    }
                }
            }
        }
        return out.toList()
    }

    private fun probeHost(host: String, port: Int): Map<String, Any?>? {
        return runCatching {
            Socket().use { sock ->
                sock.connect(InetSocketAddress(host, port), 260)
            }
            val conn = (URL("http://$host:$port/api/v1/status").openConnection() as HttpURLConnection).apply {
                connectTimeout = 400
                readTimeout = 500
                requestMethod = "GET"
            }
            try {
                if (conn.responseCode != 200) return null
                val body = conn.inputStream.bufferedReader().use { it.readText() }
                val json = JSONObject(body)
                val name = json.optString("name").ifBlank { "Bowerbird" }
                val version = json.optString("version")
                val pairing = json.optBoolean("pairingRequired", true)
                mapOf(
                    "name" to name,
                    "hosts" to listOf(host),
                    "port" to port,
                    "version" to version,
                    "pairingRequired" to pairing
                )
            } finally {
                conn.disconnect()
            }
        }.getOrNull()
    }

    private fun resolveAll(
        gen: Long,
        items: List<NsdServiceInfo>,
        index: Int,
        out: MutableList<Map<String, Any?>>,
        done: () -> Unit
    ) {
        if (gen != generation || index >= items.size) {
            done()
            return
        }
        @Suppress("DEPRECATION")
        nsd.resolveService(items[index], object : NsdManager.ResolveListener {
            override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                if (gen != generation) return
                main.post { resolveAll(gen, items, index + 1, out, done) }
            }

            override fun onServiceResolved(info: NsdServiceInfo) {
                if (gen != generation) return
                val hostAddr = info.host?.let { addr ->
                    if (addr is Inet4Address) addr.hostAddress else addr.hostAddress?.substringBefore('%')
                }
                if (!hostAddr.isNullOrBlank() && info.port > 0) {
                    val attrs = info.attributes ?: emptyMap()
                    val version = attrs["version"]?.toString(Charsets.UTF_8)?.trim()
                    val pairingStr = attrs["pairingRequired"]?.toString(Charsets.UTF_8)?.trim()
                    out.add(
                        mapOf(
                            "name" to (info.serviceName ?: "Bowerbird"),
                            "hosts" to listOf(hostAddr),
                            "port" to info.port,
                            "version" to version,
                            "pairingRequired" to (pairingStr?.equals("true", ignoreCase = true) ?: true)
                        )
                    )
                }
                main.post { resolveAll(gen, items, index + 1, out, done) }
            }
        })
    }

    private fun stopListenerOnly() {
        listener?.let { runCatching { nsd.stopServiceDiscovery(it) } }
        listener = null
        main.removeCallbacksAndMessages(null)
    }

    fun stop() {
        generation++
        activeReply = null
        stopListenerOnly()
    }

    companion object {
        private const val DEFAULT_PORT = 17878
    }
}
