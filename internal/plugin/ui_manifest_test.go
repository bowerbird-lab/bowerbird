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

func TestInspectorPluginMatchesItem(t *testing.T) {
	p := Plugin{Manifest: Manifest{Extensions: []string{"png"}, MimePrefixes: []string{"image/"}}}
	if !p.MatchesItem("png", "image/png") || !p.MatchesItem("webp", "image/webp") || p.MatchesItem("mp3", "audio/mpeg") {
		t.Fatal("inspector asset matching is incorrect")
	}
	if !(&Plugin{}).MatchesItem("txt", "text/plain") {
		t.Fatal("unfiltered action should match")
	}
}

func TestInspectorActionsValidateWindow(t *testing.T) {
	base := Manifest{ID: "example.ui", Name: "Example", Version: "1", UI: PluginUI{Windows: []PluginUIEntry{{ID: "studio", Entry: "ui/index.html"}}, Inspector: []PluginInspectorAction{{ID: "edit", Type: "window", Title: "Edit", Window: "studio"}}}}
	if err := validateManifest(&base); err != nil {
		t.Fatal(err)
	}
	base.UI.Inspector[0].Window = "missing"
	if err := validateManifest(&base); err == nil {
		t.Fatal("expected unknown window to be rejected")
	}
}

func TestPluginUIEntriesValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []PluginUIEntry
		valid   bool
	}{
		{"valid", []PluginUIEntry{{ID: "studio", Entry: "ui/studio.html"}}, true},
		{"valid external url", []PluginUIEntry{{ID: "flow", URL: "https://flow.google.com/", InterceptDownloads: true}}, true},
		{"http external url rejected", []PluginUIEntry{{ID: "flow", URL: "http://flow.google.com/"}}, false},
		{"file external url rejected", []PluginUIEntry{{ID: "flow", URL: "file:///etc/passwd"}}, false},
		{"both entry and url rejected", []PluginUIEntry{{ID: "flow", Entry: "ui/index.html", URL: "https://flow.google.com/"}}, false},
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
	panelWithURL := Manifest{ID: "example.ui", Name: "Example", Version: "1", UI: PluginUI{Panels: []PluginUIEntry{{ID: "p", URL: "https://flow.google.com/"}}}}
	if err := validateManifest(&panelWithURL); err == nil {
		t.Fatal("expected panel with external url to be rejected")
	}
}
