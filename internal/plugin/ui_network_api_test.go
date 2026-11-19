package plugin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type uiRoundTrip func(*http.Request) (*http.Response, error)

func (f uiRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUINetworkRequiresConsentAndChecksRedirect(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample")
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"sample","name":"Sample","version":"1","permissions":{"network":["api.example.com"]},"ui":{"windows":[{"id":"main","entry":"index.html"}]}}`
	if err := os.WriteFile(filepath.Join(p, "plugin.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	h := NewHost(dir)
	calls := 0
	h.SetHTTPClient(&http.Client{Transport: uiRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.example/secret"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})})
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "http://local/_api/network", strings.NewReader(`{"url":"https://api.example.com/data"}`))
		h.UINetworkHandler("sample").ServeHTTP(w, r)
		return w
	}
	if w := call(); w.Code != 403 || calls != 0 {
		t.Fatalf("ungranted request: %d %d", w.Code, calls)
	}
	h.SetUIGrant("sample", "network", true)
	if w := call(); w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), `"status":302`) {
		t.Fatalf("redirect escaped: %d %d %s", w.Code, calls, w.Body.String())
	}
}
