package plugin

import "testing"

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
