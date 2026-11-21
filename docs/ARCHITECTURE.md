<p align="center">
  <a href="ARCHITECTURE.md">English</a> · <a href="ARCHITECTURE.zh-CN.md">简体中文</a>
</p>

# BowerBird Technical Architecture

This document covers the repository layout, native plugin architecture, platform-specific drag-and-drop implementation, data & storage specification, and build internals. For quick start, running, and building, see [README.md](../README.md).

## Project Structure

```text
├── main_mygo.go                      # mygo application entry (windows, menus, tray, native OS file drop routing)
├── mygo.json                         # mygo build configuration (app name, bundle id, version, dev/build commands)
├── core/                             # Go backend package
│   ├── models.go                     # Domain models (Item, ImageMeta, Folder, Tag, LibraryInfo)
│   ├── library.go                    # SQLite indexing, import pipeline, folder/tag/trash CRUD
│   ├── color_extractor.go            # Perceptual color palette quantizer & EXIF/image metadata parser
│   ├── asset_server.go               # Local HTTP streaming server for originals, thumbnails & system icons
│   ├── service.go                    # Application service exposed to the frontend via mygo.Bind
│   ├── host.go                       # Host abstraction over the desktop runtime (events, dialogs, drag, main thread)
│   └── drag_*.go                     # Platform-specific native OS drag-out & debug tracing
├── internal/
│   ├── app/                          # Native mygo/ui views & main-thread state machine
│   │   ├── ui_main.go                # Workspace shell, routing, shortcuts
│   │   ├── ui_gallery.go             # Justified gallery, selection, native drag
│   │   ├── ui_inspector.go           # Native item inspector & plugin sections
│   │   ├── ui_preview.go             # Image, markdown, audio, video & PSD previews
│   │   └── ui_settings.go            # Settings, plugin manager and data locations
│   ├── plugin/                       # Native plugin host (manifest, sandbox, built-ins)
│   ├── player/                       # Go audio playback kernel
│   └── i18n/                         # zh-CN / en dictionaries
├── plugins/                          # Zero-build official plugins
│   ├── markdown-preview/             # plugin.json + inspect hook
│   ├── audio-player/
│   ├── psd-preview/
│   ├── prompt-metadata/
│   └── pinterest/                    # Sandboxed URL resolver
├── packages/plugin-sdk/              # TypeScript definitions for native plugins
└── packages/ui/                      # Shared React component kit used by the browser extension
```

---

## Native Plugin Architecture

The desktop app is rendered by `mygo/ui` (Go + GPU). There is no WebView, React runtime or plugin CSS: plugins return **plain JSON**, and the host paints the result with native views.

### Official plugins

| Plugin ID | Version | Kind | Native renderer / capability |
| :--- | :--- | :--- | :--- |
| `official.markdown-preview` | `2.0.0` | preview | `markdown` — GFM rendering, frontmatter and outline inspection |
| `official.audio-player` | `2.0.0` | preview | `audio` — playback, spectrum bars, scrubber, metadata |
| `official.psd-preview` | `2.0.0` | preview | `psd` — composite decode, zoom, layer tree |
| `official.prompt-metadata` | `2.0.0` | metadata | `prompt` — AI prompt field, copy action, full-text editor |
| `builtin.video` | `2.0.0` | preview | `video` — ffmpeg frame decode + `ui.Video`, mygo audio output |
| `official.pinterest` | `2.2.0` | resolver | `resolveUrl` in the embedded moejs runtime |

Sources live in `plugins/<plugin-id>/` (`plugin.json`, plus `index.js` only for resolver plugins). There is no build step. Packaged builds register the same manifests as compiled-in built-ins, and a local folder overrides the bundled copy.

### Manifest

```json
{
  "id": "official.markdown-preview",
  "name": "Markdown Preview",
  "version": "2.0.0",
  "kind": "preview",
  "native": "markdown",
  "extensions": ["md", "markdown"],
  "mimePrefixes": ["text/markdown"],
  "priority": 50
}
```

Native preview plugins may omit `main`. JavaScript hooks (`resolveUrl`, `inspect`, `actions`, `runAction`, `thumbnail`) are optional but must be explicitly listed in `hooks`; the host never invokes an undeclared hook. They execute in an embedded **moejs** runtime with explicit network/process/link permissions, a memory limit and a timeout. No Node process is involved. Every hook start/end, result, error and host API call is written to the native plugin log; plugins can add their own entries with `host.log()` or `console.*`. TypeScript definitions live in `packages/plugin-sdk`; see `plugins/README.md` for the complete guide.

---

## Native Drag & Drop Out of the Window

Dragging an asset to another application is platform-specific because an HTML5 drag from the webview only publishes text formats (`text/plain`, `text/uri-list`):

- **Windows** (`core/drag_windows.go`): `frontend/src/lib/drag-out.ts` cancels the webview drag and calls `Service.StartDragItems` with the dragged item IDs. The Go side builds a shell data object (`SHCreateDataObject` → `CF_HDROP`) and runs `DoDragDrop` with `DROPEFFECT_COPY`.
- **macOS** (`core/drag_darwin.go`): The webview drag session is preserved and its pasteboard is rewritten into `public.file-url` / `NSFilenamesPboardType`.
- **In-App Drop Visuals**: Both the sidebar and main workspace use a non-intrusive `10%` opacity mask with a dashed border (`bg-foreground/10 border-dashed`) without obscuring folder names or shifting layout.

### Drag & Drop Tracing

Every step of the drag-and-drop pipeline is logged to `drag-debug.log` in the user data directory (`%APPDATA%\bowerbird\drag-debug.log` on Windows, `~/Library/Application Support/bowerbird/drag-debug.log` on macOS). Set `BOWERBIRD_USER_DATA_DIR` to override the directory during tests.

---

## Data & Storage Architecture Specification

Bowerbird enforces a strict physical separation between **User Libraries** and **Application State Data**:

```text
┌────────────────────────────────────────────────────────┐
│ 1. User Asset Library (Portable Container Package)     │
│    e.g. ~/Pictures/Designs.library/                    │
│    ├── items/<id>/<filename>   (random uppercase ID)   │
│    ├── items/<id>/thumbnail.jpg                        │
│    ├── items/<id>/metadata.json                        │
│    └── index.db (SQLite cache + SHA-256 hex index)     │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ 2. Application User Data Directory (<UserDataDir>)     │
│    macOS:   ~/Library/Application Support/bowerbird/   │
│    Windows: %APPDATA%\bowerbird\                       │
│    Linux:   ~/.config/bowerbird/                       │
│    ├── user_data.json      # Global UI settings        │
│    ├── app.log             # Diagnostics runtime logs  │
│    ├── drag-debug.log      # Drag & Drop trace logs    │
│    └── plugins/            # Installed remote plugins  │
│        ├── installed.json  # Installed plugin registry │
│        └── <plugin-id>/    # Extracted plugin bundle   │
│            ├── manifest.json                           │
│            └── index.js                                │
└────────────────────────────────────────────────────────┘
```

### Plugin Ecosystem & Distribution Specification

1. **Native source layout** (`bowerbird-lab/plugins` `main` branch):
   - Official plugins live in `plugins/<plugin-id>/` as `plugin.json` plus an optional zero-build `index.js`.
   - Preview plugins declare a host renderer (`"native": "markdown"`); they ship no React components and need no Vite/Rollup output.
2. **Dual-channel distribution**:
   - Build artifacts are published from `bowerbird-lab/plugins` (`main` -> `repo` orphan branch + GitHub Releases).
   - **`repo` orphan branch**: compact index metadata (`index.min.json`, `registry.json`) and `repo/<id>/plugin.json`.
   - **GitHub Releases**: packaged zip bundles (`<plugin-id>-v<version>.zip`).
3. **Marketplace & edge resolution**:
   - The marketplace reads `PLUGIN_REPO_INDEX_URL` and downloads releases from `PLUGIN_RELEASE_BASE_URL` without proxying.
4. **Runtime state**:
   - `<UserDataDir>/plugins/installed.json` is the legacy WebView registry; the native host keeps enable/settings state in `native-plugins.json`.
   - Settings → Data & Storage reveals both the plugin directory and the application data directory.

---

## Build & Packaging Internals

### cgo Requirement

The native macOS implementations in `core/` (`sysicon_darwin.go`, `videothumb_darwin.go`, `drag_darwin.go`) contain `import "C"`. With `CGO_ENABLED=0` Go silently drops them, and their `!darwin || ios` fallbacks are also excluded on darwin — so symbols like `StartDragItems` / `GetSystemFileIconPNG` vanish and the build fails with `undefined: ...` errors. The `pnpm dev` / `pnpm build:app*` scripts route through `scripts/mygo.mjs`, which forces `CGO_ENABLED=1` on every platform.

### Workspace Package Resolution

`@bowerbird/plugin-sdk` and `@workspace/ui` are consumed straight from TypeScript sources — `@bowerbird/plugin-sdk` publishes `dist/` output that only `pnpm --filter @bowerbird/plugin-sdk build` produces, and only npm publishing needs.

- **Type checking** — map the package in the consumer's `tsconfig.json` `paths` (substitutions resolve relative to `baseUrl`):
  `"@bowerbird/plugin-sdk": ["../packages/plugin-sdk/src/index.ts"]`.
- **Vite** — set `resolve.tsconfigPaths: true` so dev resolution matches `tsc` (`frontend/vite.config.ts`); production builds inline the sources either way.

Without the Vite option, dev fails with `Failed to resolve import "@bowerbird/plugin-sdk" from ... Does the file exist?`.

### Windows Installer (NSIS)

On Windows, `go tool mygo build` (via `pnpm run build:app:windows`) produces both the portable executable and an NSIS installer under `build/mygo/windows-amd64/`. The installer registers the app under *Apps & features*, installs the WebView2 runtime when it is missing, and asks the user whether to create shortcuts (Start Menu on by default, Desktop optional) plus whether to launch the app.

The NSIS step runs as part of the mygo Windows packaging pipeline; GitHub's `windows-latest` runners build it out of the box. For local Windows builds, keep `makensis` available on `PATH`: install it with `scoop install nsis` (shim lands on `PATH`), `choco install nsis` (append `C:\Program Files (x86)\NSIS` to `PATH`), or `winget install NSIS.NSIS` (manual `PATH` entry required).

```bash
go tool mygo build
# -> build/mygo/windows-amd64/   portable executable + NSIS installer
```

CI (`.github/workflows/release-app.yml`) publishes both as `bowerbird-windows-x64-portable.exe` and `bowerbird-windows-x64-installer.exe`.
