package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFindsUIOnlyPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "example.webview-demo")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"example.webview-demo","name":"Demo","version":"1","kind":"action","ui":{"windows":[{"id":"demo","entry":"ui/index.html"}]}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	host := NewHost(dir)
	p, ok := host.Get("example.webview-demo")
	if !ok || p.Manifest.Main != "" || len(p.Manifest.UI.Windows) != 1 {
		t.Fatalf("UI-only plugin not discovered correctly: %+v", p)
	}
}

func TestPluginUIEntriesValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []PluginUIEntry
		valid   bool
	}{
		{"valid", []PluginUIEntry{{ID: "studio", Entry: "ui/studio.html"}}, true},
		{"traversal", []PluginUIEntry{{ID: "studio", Entry: "../other.html"}}, false},
		{"absolute", []PluginUIEntry{{ID: "studio", Entry: "/etc/passwd"}}, false},
		{"windows separator", []PluginUIEntry{{ID: "studio", Entry: `..\other.html`}}, false},
		{"duplicate", []PluginUIEntry{{ID: "studio", Entry: "a.html"}, {ID: "studio", Entry: "b.html"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Manifest{ID: "example.ui", Name: "Example", Version: "1", UI: PluginUI{Windows: tc.entries}}
			if err := validateManifest(&m); (err == nil) != tc.valid {
				t.Fatalf("validateManifest() error = %v, want valid %v", err, tc.valid)
			}
		})
	}
}
