# @bowerbird/plugin-sdk

TypeScript SDK for **self-maintained, zero-build** Bowerbird plugins.

The Bowerbird desktop app itself is completely native (`mygo/ui` + Go). Plugins
are self-contained packages that extend Bowerbird without coupling plugin logic
into the host app:

1. **Isolated WebView pages** (`preview.entry`, `ui.windows`, `ui.panels`) —
   for custom file format previews, rich interactive tools, or controlled
   external HTTPS pages, with transparent background support and `/_api/*`
   bridge endpoints.
2. **moejs sandbox hooks** (`main` + `hooks`) — zero-build `.js`/`.ts` modules
   run in-process by the Go host for URL resolution, declarative inspector
   sections, context-menu actions, and thumbnail generation.

## Install

```bash
pnpm add -D @bowerbird/plugin-sdk
```

The SDK is types-only for most plugins. Runtime helpers such as `formatBytes`
are tiny and can be imported directly; there is no framework runtime to bundle.

## Plugin layout

```text
my-plugin/
├── plugin.json
├── index.js          # optional: moejs hooks (.js or .ts, no bundler needed)
├── view.html         # optional: WebView preview page (preview.entry)
└── ui/               # optional: WebView window pages (ui.windows)
```

```json
{
  "id": "example.ai-inspector",
  "name": "AI Inspector",
  "version": "2.0.0",
  "minAppVersion": "1.0.0",
  "kind": "metadata",
  "main": "index.js",
  "hooks": ["inspect"],
  "permissions": { "network": [], "process": [] },
  "settings": [
    { "key": "endpoint", "label": "API endpoint", "placeholder": "https://…" }
  ]
}
```

## Hooks (moejs sandbox)

```ts
// index.js — no imports and no build step required
export async function inspect({ item }) {
  if (!item.prompt) return null;
  return {
    sections: [{
      title: "AI Prompt",
      rows: [
        { label: "Characters", value: String(item.prompt.length) },
        { label: "Words", value: String(item.prompt.trim().split(/\s+/).length) },
      ],
      textBlocks: [{ title: "Prompt", text: item.prompt }],
    }],
  };
}
```

| Hook | Purpose |
| --- | --- |
| `resolveUrl({ url })` | Resolve a page/short link to direct media (or `items[]`). |
| `inspect({ item })` | Add declarative sections to the native inspector. |
| `actions(ctx)` | Contribute native context-menu actions. |
| `runAction({ id, item, items, folder })` | Execute one advertised action. |
| `thumbnail({ filePath, mimeType })` | Generate a static cover image for the gallery grid. |

## Preview & UI WebView plugins

Plugins own their own preview and tool UIs using standard HTML/JS/CSS/WASM:

```json
{
  "id": "example.psd-preview",
  "name": "PSD Preview",
  "version": "2.0.0",
  "kind": "preview",
  "extensions": ["psd", "psb"],
  "mimePrefixes": ["image/vnd.adobe.photoshop"],
  "priority": 95,
  "preview": {
    "renderer": "webview",
    "entry": "view.html",
    "supportsZoom": true
  },
  "permissions": {
    "assets": ["read"]
  }
}
```

Local WebView pages are served via `bowerplugin://localhost/<entry>` inside an
untrusted, isolated session and can call the host bridge endpoints (subject to
`permissions` and explicit user consent):

| Endpoint | Purpose |
| --- | --- |
| `GET /_api/asset` | Selected asset metadata + base64 body (up to 24 MB). |
| `GET /_api/asset/raw` | Streaming raw bytes of the selected asset (supports `Range`). |
| `POST /_api/asset/save` | Save `create`, `replace`, or `variant` bytes back to the library. |
| `GET /_api/settings/{key}` | Read a declared plugin setting. |
| `POST /_api/network` | HTTPS proxy scoped to `permissions.network`. |

External tool windows can also declare `ui.windows[].url` (`https://` only) with
`interceptDownloads: true` to import downloaded files straight into the library.

## URL resolver plugins

```json
{
  "kind": "resolver",
  "urlHosts": ["pinterest.com", "*.pinterest.com", "pin.it"],
  "urlPatterns": ["^https?://pin\\.it/[A-Za-z0-9]+"],
  "permissions": { "network": ["pinterest.com", "*.pinterest.com", "pin.it"] }
}
```

## Sandbox limits

`globalThis.host` exposes only `settings.get`, `fetch`, `spawn`, `platformOpen`,
and `log` (plus `console.*`). Network, filesystem, and child processes are
denied by default. Each hook invocation gets a fresh runtime with a 60-second
timeout and a 256 MB heap limit.

## Validation

```ts
import { validatePluginManifest, normalizePluginManifest } from "@bowerbird/plugin-sdk";

const result = validatePluginManifest(JSON.parse(raw));
if (!result.valid) console.error(result.errors);
```

## License

MIT
