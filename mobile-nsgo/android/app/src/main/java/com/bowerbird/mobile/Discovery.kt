package com.bowerbird.mobile

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Handler
import android.os.Looper
import dev.oneo.nsgo.NsgoView
import java.net.Inet4Address

/** mDNS discovery for _bowerbird._tcp through Android's NSD service. */
class Discovery(context: Context) {
    private val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager
    private val main = Handler(Looper.getMainLooper())
    private var listener: NsdManager.DiscoveryListener? = null
    private var activeReply: NsgoView.HostReply? = null
    private var generation = 0L

    fun browse(reply: NsgoView.HostReply) {
        stop()
        val gen = ++generation
        activeReply = reply
        val found = LinkedHashMap<String, NsdServiceInfo>()
        val finish = Runnable {
            if (gen != generation) return@Runnable
            val items = found.values.toList()
            val resolved = mutableListOf<Map<String, Any?>>()
            resolveAll(gen, items, 0, resolved) {
                if (gen != generation) return@resolveAll
                activeReply = null
                reply.resolve(resolved)
                stopListenerOnly()
            }
        }
        listener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(serviceType: String) {}
            override fun onServiceFound(service: NsdServiceInfo) {
                if (gen != generation) return
                if (service.serviceType.startsWith("_bowerbird._tcp")) {
                    val key = service.serviceName ?: return
                    if (!found.containsKey(key)) {
                        found[key] = service
                    }
                }
            }
            override fun onServiceLost(service: NsdServiceInfo) {
                found.remove(service.serviceName)
            }
            override fun onDiscoveryStopped(serviceType: String) {}
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                if (gen != generation) return
                activeReply = null
                reply.reject("discovery failed: $errorCode")
                stop()
            }
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {}
        }
        try {
            nsd.discoverServices("_bowerbird._tcp", NsdManager.PROTOCOL_DNS_SD, listener)
            main.postDelayed(finish, 2500)
        } catch (e: Exception) {
            activeReply = null
            reply.reject(e.message ?: "discovery failed")
            stop()
        }
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
}
