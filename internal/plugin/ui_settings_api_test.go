package plugin

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUISettingsAreScopedAndDisabled(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"first", "second"} {
		folder := filepath.Join(dir, id)
		if err := os.Mkdir(folder, 0755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"id":"` + id + `","name":"Test","version":"1","settings":[{"key":"apiKey","label":"Key","secret":true}],"ui":{"windows":[{"id":"main","entry":"index.html"}]}}`
		if err := os.WriteFile(filepath.Join(folder, "plugin.json"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHost(dir)
	h.SetSetting("first", "apiKey", "first-key")
	h.SetSetting("second", "apiKey", "second-key")
	first, second := new(int), new(int)
	var sessions UISessions
	sessions.Attach(first, filepath.Join(dir, "first"), func() bool { return h.IsEnabled("first") }, h.UISettingsHandler("first"))
	sessions.Attach(second, filepath.Join(dir, "second"), func() bool { return h.IsEnabled("second") }, h.UISettingsHandler("second"))
	check := func(page any, path string, code int, body string) {
		t.Helper()
		w := httptest.NewRecorder()
		sessions.ServePage(page, w, httptest.NewRequest("GET", "http://localhost"+path, nil))
		if w.Code != code || (body != "" && w.Body.String() != body) {
			t.Fatalf("got %d %q", w.Code, w.Body.String())
		}
	}
	check(first, "/_api/settings/apiKey", 200, "{\"value\":\"first-key\"}\n")
	check(second, "/_api/settings/apiKey", 200, "{\"value\":\"second-key\"}\n")
	check(first, "/_api/settings/other", 404, "")
	h.SetEnabled("first", false)
	check(first, "/_api/settings/apiKey", 403, "")
}
