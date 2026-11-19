package plugin

import "testing"

func TestValidateSidebar(t *testing.T) {
	windows := []PluginUIEntry{{ID: "studio", Entry: "ui/index.html"}}
	for _, tc := range []struct {
		name  string
		items []PluginSidebarItem
		valid bool
	}{
		{"nested", []PluginSidebarItem{{ID: "tools", Type: "group", Title: "Tools", Children: []PluginSidebarItem{{ID: "open", Type: "window", Title: "Studio", Window: "studio"}}}}, true},
		{"unknown window", []PluginSidebarItem{{ID: "open", Type: "window", Title: "Studio", Window: "missing"}}, false},
		{"duplicate", []PluginSidebarItem{{ID: "a", Type: "window", Title: "A", Window: "studio"}, {ID: "a", Type: "window", Title: "B", Window: "studio"}}, false},
		{"unknown type", []PluginSidebarItem{{ID: "a", Type: "webview", Title: "A"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSidebar(tc.items, windows)
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
