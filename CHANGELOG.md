# Changelog

All notable changes to Bowerbird are documented here. Every release so far is a
pre-release of `1.0.0`. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the project aims at [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release tags are `v<version>`; the version is stamped into `mygo.json` by the release
workflow, so a release is defined by its tag rather than by a file in the repository.

Up to `v1.0.0-alpha-10` releases were numbered sequentially. Since the native renderer
release, tag names carry the release date as `v1.0.0-alpha-YYYY-MM-DD`; that first
date-based release is the renamed `alpha-10` and appears below as
`1.0.0-alpha-2026-11-15`.

## [1.0.0-alpha-2026-11-15] - 2026-11-15

The desktop app is now rendered natively by `mygo/ui`. The React 19 + Vite + Tailwind
frontend, the desktop part of `packages/ui` and the bundled WebView are gone: every
screen is declared in Go under `internal/app/`, and plugins are zero-build sandboxes.

### Added

- Native workspace shell with routing, a floating header shared with previews, and side
  panels resized by dragging their edges.
- Justified gallery with on-demand loading, shift-range and `Ctrl`/`Cmd` multi-select
  across layouts, native drag-out, drag previews and a film strip that drives the
  selection.
- Sidebar with a searchable library picker, folder reordering and moving between levels,
  a folder context menu (create subfolder, rename, remove) shared with subfolder cards,
  and a bare `+` revealed on header hover.
- Inspector rebuilt around the asset: color palette at the top, one star rating control,
  link and note first, folders picked from a searchable tree, tags picked from the whole
  library.
- Filter bar where every criterion carries an icon; the color filter is picked from
  swatches or a wheel and matched with perceptual ΔE.
- Theme system with JSON light/dark presets (default, sand-light, midnight-dark), a theme
  editor, desktop-accent following with an opt-out, and an appearance dropdown.
- Complete `zh-CN` / `en` dictionaries, language restored on launch, and a shortcuts
  dialog listing the whole keymap with live key caps and conflict warnings.
- Library identity and icon editing, plus preview artboards and palette-driven details.
- Context-aware deletion from smart views: the folders an asset lives in are listed and
  it can be detached or moved to the trash; removing it from a folder only drops the link.
- Native plugin system on **moejs**: `plugin.json` + dependency-free `index.js`, declared
  network/process/external-link permissions, memory caps, timeouts and host API logging.
- Zero-build built-ins: markdown, audio, native video (ffmpeg frames presented through
  `mygo/ui` with audio output), PSD/PSB, prompt metadata and Pinterest.
- Generic URL resolver plus Pinterest importer (`resolveUrl` hooks).
- Mobile settings surface: serving port and pairing QR code.

### Changed

- `packages/plugin-sdk` types and host API rewritten for the native renderer; plugin docs
  updated in both languages and the marketplace catalog moved to the new manifest shape.
- Release workflows build native desktop binaries and package zero-build plugins;
  `scripts/package-plugins.py` replaces the Node plugin build script.
- `packages/ui` reduced to the six components the browser extension still imports.
- Android launcher icons, web, extension and macOS menu bar branding unified on the new
  artwork, with a monochrome template icon in the menu bar.

### Fixed

- Memory is bounded: gallery items load on demand, the log buffer is capped with
  coalesced viewer updates, preview caches are isolated and folder relations cached
  (subfolder queries limited to one level).
- Dropped media downloads directly and honors configured shortcuts; drops land in the
  current folder, including cross-library drops.
- Smart views are scoped to their own query, and subfolder rows keep their drops and
  alignment.
- The workspace install no longer fails on the removed `@workspace/ui` dependency.

### Removed

- `frontend/` (React 19 + Vite + Tailwind), the desktop UI kit, `scripts/build-plugins.mjs`
  and every `manifest.json` + `src/index.tsx` plugin source.

Related: #51, #52, #53

## [1.0.0-alpha-9] - 2026-11-03

### Fixed

- The plugin store defaults to the deployed Worker address instead of falling back to
  `localhost:3000`. (#49)

## [1.0.0-alpha-8] - 2026-11-03

### Added

- PSD/PSB preview plugin, with plugin enable-state and cover-thumbnail fixes. (#44)
- AI prompt plugin with a cleaner metadata presentation, bumped to 1.0.1. (#48)
- Windows windows leave room for the system caption buttons in both collapsed and
  expanded header states. (#42)

### Changed

- README got a banner illustration, a rewritten feature tour and a simplified Chinese
  translation. (#39, #40)
- pnpm bumped to 12.16.1 and the Cloudflare PR preview build fixed. (#46)
- Marketplace theme switching no longer loses the selection; placeholder previews and
  fake plugins removed. (#47)

### Fixed

- Windows drag-and-drop import: routing fix plus streamed upload, no more main-thread
  lockups on large files. (#43, #45)

## [1.0.0-alpha-7] - 2026-10-27

### Added

- Mobile ChatGPT-2026 visual tokens, a full-height sidebar, round buttons and folder swipe
  gestures. (#35, #36)

### Changed

- Desktop UI overhaul across the workspace, gallery and inspector. (#37)
- CI no longer hardcodes pnpm 9, so it follows `packageManager`. (#38)

## [1.0.0-alpha-6] - 2026-10-24

### Added

- Lightweight mode with a settings toggle and a tray entry. (#29)
- Customizable shortcuts and screenshot settings, scoped as global vs in-app. (#30)
- Connection settings with a device list and API tokens. (#31)
- Mobile folder drawer, filter sheet and pinch zoom. (#32)

### Changed

- Library setup window simplified; closing a library returns to the picker. (#26)
- Window theme sync and an upgraded canvas context menu. (#27)
- Plugin settings simplified, built-in plugins listed in the store. (#28)

### Fixed

- Unused lucide imports no longer break the release type check. (#33)

## [1.0.0-alpha-5] - 2026-10-23

### Added

- System share inbox, generic query API and the desktop query engine. (#25)

### Changed

- README rewritten for the mygo architecture; Wails-era directories removed. (#24)

### Fixed

- Windows build uses a real PNG instead of the `appicon` symlink. (#23)

## [1.0.0-alpha-4] - 2026-10-21

The desktop architecture migration, done in WebView mode first.

### Changed

- Desktop moved from Wails to **mygo** (webview mode): a `Host` abstraction decouples
  `core` from the UI framework, a new `main_mygo.go` entry owns windows, menus, tray and
  native drop routing, and CI switched to the mygo build chain. (#22)

### Added

- Mobile v3: library switching, network discovery configuration, WebSocket push and
  upload. (#21)
- Mobile v3.1: copy to clipboard, video and GIF playback, delete, and per-field copy in
  the detail view. (#20)

### Fixed

- WebSocket broadcast coverage for asset changes, and the mobile client reconnects and
  refreshes after returning to the foreground. (#21)

## [1.0.0-alpha-3.1] - 2026-10-20

### Added

- Mobile: video and GIF playback in the detail view, copy to clipboard, delete, per-field
  copy, and GIF badges in the grid. (#20, follow-up commits)
- Android: copying asset content to the system clipboard, and a delete endpoint for
  mobile clients.

### Fixed

- Mobile refresh no longer drops concurrent updates, and returns from the background with
  a reconnecting WebSocket plus a catch-up refresh.
- Desktop broadcasts every asset change over the WebSocket channel.

## [1.0.0-alpha-2] - 2026-10-20

The mobile companion app lands.

### Added

- Flutter mobile app scaffold for Android, with an API contract specification.
- Desktop LAN API server with mDNS discovery, pairing and media streaming, plus pairing
  configuration in desktop settings and the pairing code logged at startup.
- Mobile pairing flow, QR scanning, candidate IP list, monthly timeline grid, filters,
  photo zoom, share intake and a detail viewer.
- Android APK build workflow (manual dispatch and tag releases).

### Fixed

- Screenshot, API server and error tests isolate their user data directory.

## [1.0.0-alpha-1] - 2026-10-19

### Added

- Marketplace web app migrated to `apps/marketplace` on Cloudflare Workers, restyled after
  Hyakume, with a separate documentation app built on Cloudflare Nimbus. (#10, #12)
- Network proxy settings, plugin install lifecycle logs and local plugin scanning. (#11)
- NSIS installer published as a release asset, with user-selectable shortcuts. (#16)
- New app launcher icons and website logo. (#17)

### Fixed

- CI: Windows NSIS PATH, Linux GTK4 dependencies, plugin sync shell syntax, and a resilient
  NSIS install action with a fallback. (#9, #19)
- Workspace packages resolve from source. (#13)
- Gallery wheel scrolling. (#17)

## [1.0.0-alpha] - 2026-10-18

First pre-release.

### Added

- Multi-layout gallery views with a zero-allocation layout engine, and instant
  delete/import that holds up with 37k+ items. (#4)
- Remote plugin system, plugin SDK, the `bowerbird://` protocol and a TanStack Start
  marketplace. (#5)
- Multi-platform release CI, plugin repository sync, SDK publishing to npm, and data
  storage settings. (#6)

### Fixed

- Infinite layout jitter when deleting items while zoomed in. (#3)

[1.0.0-alpha-2026-11-15]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-2026-11-15
[1.0.0-alpha-9]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-9
[1.0.0-alpha-8]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-8
[1.0.0-alpha-7]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-7
[1.0.0-alpha-6]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-6
[1.0.0-alpha-5]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-5
[1.0.0-alpha-4]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-4
[1.0.0-alpha-3.1]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-3.1
[1.0.0-alpha-2]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-2
[1.0.0-alpha-1]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha-1
[1.0.0-alpha]: https://github.com/owocc/bowerbird/releases/tag/v1.0.0-alpha
