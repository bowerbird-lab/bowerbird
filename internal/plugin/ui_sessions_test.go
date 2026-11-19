package plugin

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUISessionsIsolateAndRevoke(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	for dir, text := range map[string]string{dirA: "A", dirB: "B"} {
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, b, unknown := new(int), new(int), new(int)
	var sessions UISessions
	allowed := true
	sessions.Attach(a, dirA, func() bool { return allowed }, nil)
	sessions.Attach(b, dirB, func() bool { return true }, nil)
	check := func(page any, wantStatus int, want string) {
		t.Helper()
		w := httptest.NewRecorder()
		sessions.ServePage(page, w, httptest.NewRequest("GET", "http://b/index.html", nil))
		if w.Code != wantStatus || (want != "" && w.Body.String() != want) {
			t.Fatalf("got %d %q, want %d %q", w.Code, w.Body.String(), wantStatus, want)
		}
	}
	check(a, 200, "A")
	check(b, 200, "B")
	check(unknown, 403, "")
	allowed = false
	check(a, 403, "")
	allowed = true
	sessions.Detach(a)
	check(a, 403, "")
	check(b, 200, "B")
}
