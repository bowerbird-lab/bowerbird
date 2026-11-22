package plugin

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// UIResourceHandler serves one plugin's packaged files. The caller must bind
// the handler to an authenticated WebView identity before exposing it.
// It does not serve bridge requests or grant network access.
func UIResourceHandler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || strings.Contains(name, "\\") || !filepath.IsLocal(name) || path.Clean(name) != name {
			http.NotFound(w, r)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer root.Close()
		f, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; worker-src 'self' blob:; connect-src 'self'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if typ := mime.TypeByExtension(filepath.Ext(name)); typ != "" {
			w.Header().Set("Content-Type", typ)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, f)
		}
	})
}
