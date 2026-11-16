# Bowerbird mobile — NSGo rewrite

This module is the replacement mobile client. It talks to the existing desktop
`/api/v1` HTTP + WebSocket API and runs as an NSGo Go app inside a small Kotlin
Android host.

Status: **P0–P5 vertical slices implemented**. Pairing, gallery paging,
search, detail, download/share/delete, inbox, multi-select, QR scanning,
library switching, appearance settings and desktop-compatible themes are in
place. See `reports/移动端NSGo实施任务计划.md`.

## Appearance

The settings screen is reachable from the gallery "更多" sheet and from the
connect screen. It offers system/light/dark mode, separate light and dark theme
variants, an optional Android system accent, and grid density.

Colour themes use the desktop schema, so `internal/theme/presets` can be copied
as-is. On the phone, use "导入主题" to pick a `*-light.json` / `*-dark.json` desktop
theme file, or put files in `<app data>/themes` and press "重新读取". Built-ins
(`default-light`, `default-dark`, `sand-light`, `midnight-dark`) cannot be
replaced. Mobile adds
one optional `success` token; desktop files without it inherit it from the
matching built-in theme.

## Layout

```
main.go                 // nsgo.Run(ui.Root)
nsgo.yaml               // app id com.bowerbird.mobile, Android permissions
internal/model          // per-endpoint DTOs
internal/query          // query-string builder + legacy fallback capability
internal/transport      // HTTP, errors, query/items fallback, upload/download, WS
internal/storage        // atomic versioned JSON: session, history, usage, inbox, tags, preferences
internal/theme          // desktop-compatible colour themes (built-in presets + user JSON files)
internal/platform       // host-channel contracts: secure store, discovery, share intake, system accent
internal/app            // session manager, gallery generations, event classification
internal/ui             // connect, gallery, search, detail, inbox, settings & theme pickers
android/                // Kotlin host: Keystore, NSD, share intake, deep links
```

## Build

Requirements: Go 1.27.3, Android SDK 36, JDK 21, Gradle from the wrapper.

```sh
# 1. Pin and verify dependencies
go mod download
go tool nsgo check
go test ./...

# 2. Build the NSGo AAR (arm64 device; android_x64 for an emulator)
./scripts/build-aar.sh android_arm64

# 3. Build the host APK
cd android && ./gradlew assembleDebug
# -> android/app/build/outputs/apk/debug/app-debug.apk
```

`android/local.properties` must contain `sdk.dir=...`; it is intentionally not
committed.

## Security / release notes

- `storage.Local` currently keeps the token in `session.json` (mode 0600). The
  Kotlin host already implements `secure.get/set/delete` with Android Keystore.
  Release work must move the token there and keep only non-secret session data
  in JSON.
- The debug APK is signed with the Android debug key. Before any update test,
  read the actual certificate and versionCode of the last published APK; do not
  assume the `1.0.1+2` values from the Flutter project.
- The host allows cleartext HTTP for LAN addresses. This is required by the
  desktop contract; note it in the store listing and consider a network security
  config that limits cleartext to private address ranges.
- `minSdk` is 28, set by the NSGo runtime; confirm the supported-device impact
  before release.
