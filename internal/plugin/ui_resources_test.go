package plugin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIResourceHandlerConfinesFiles(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	h := UIResourceHandler(dir)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/index.html", http.StatusOK},
		{http.MethodGet, "/../secret", http.StatusNotFound},
		{http.MethodGet, "/escape", http.StatusNotFound},
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodPost, "/index.html", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://plugin"+tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			if tc.status == http.StatusOK && (!strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") || w.Body.String() != "hello") {
				t.Fatalf("unexpected response: %#v %q", w.Header(), w.Body.String())
			}
		})
	}
}
