package plugin

import (
	"fmt"
	"strings"
)

// validateSidebar rejects ambiguous trees and references outside the plugin.
func validateSidebar(items []PluginSidebarItem, windows []PluginUIEntry) error {
	known := make(map[string]bool, len(windows))
	for _, window := range windows {
		known[window.ID] = true
	}
	seen := map[string]bool{}
	var walk func([]PluginSidebarItem, int) error
	walk = func(nodes []PluginSidebarItem, depth int) error {
		if depth > 8 {
			return fmt.Errorf("plugin sidebar exceeds maximum depth")
		}
		for _, node := range nodes {
			if node.ID == "" || strings.ContainsAny(node.ID, "/\\") || seen[node.ID] || strings.TrimSpace(node.Title) == "" {
				return fmt.Errorf("invalid or duplicate sidebar item %q", node.ID)
			}
			seen[node.ID] = true
			switch node.Type {
			case "group":
				if node.Window != "" || len(node.Children) == 0 {
					return fmt.Errorf("invalid sidebar group %q", node.ID)
				}
				if err := walk(node.Children, depth+1); err != nil {
					return err
				}
			case "window":
				if len(node.Children) != 0 || !known[node.Window] {
					return fmt.Errorf("invalid sidebar window %q", node.ID)
				}
			default:
				return fmt.Errorf("unknown sidebar type %q", node.Type)
			}
		}
		return nil
	}
	return walk(items, 0)
}
