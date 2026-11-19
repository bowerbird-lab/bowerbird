package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUIGrantsPersistAndRevokeOnScopeChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample")
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(domains string) {
		manifest := `{"id":"sample","name":"Sample","version":"1","permissions":{"network":` + domains + `,"assets":["read"]},"ui":{"windows":[{"id":"main","entry":"index.html"}]}}`
		if err := os.WriteFile(filepath.Join(p, "plugin.json"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`["example.com"]`)
	h := NewHost(dir)
	if h.UIGranted("sample", "network") {
		t.Fatal("new permission was granted implicitly")
	}
	if !h.SetUIGrant("sample", "network", true) || !h.UIGranted("sample", "network") {
		t.Fatal("grant failed")
	}
	if h.SetUIGrant("sample", "assets.write", true) {
		t.Fatal("undeclared write permission granted")
	}
	h = NewHost(dir)
	if !h.UIGranted("sample", "network") {
		t.Fatal("grant did not persist")
	}
	write(`["example.com","other.example"]`)
	h.Scan()
	if h.UIGranted("sample", "network") {
		t.Fatal("expanded domains retained old grant")
	}
	if !h.SetUIGrant("sample", "network", true) {
		t.Fatal("regrant failed")
	}
	h.SetEnabled("sample", false)
	if h.UIGranted("sample", "network") {
		t.Fatal("disabled plugin retained access")
	}
}
