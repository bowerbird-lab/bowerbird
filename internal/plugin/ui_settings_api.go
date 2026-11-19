package plugin

import (
	"encoding/json"
	"net/http"
	"strings"
)

// UISettingsHandler exposes only declared settings of the bound plugin. The
// handler must only be used behind a UISessions identity check.
func (h *Host) UISettingsHandler(pluginID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/_api/settings/") {
			http.NotFound(w, r)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/_api/settings/")
		h.mu.RLock()
		p, ok := h.plugins[pluginID]
		var value string
		declared := false
		if ok && p.Enabled {
			for _, setting := range p.Manifest.Settings {
				if setting.Key == key {
					declared = true
					var exists bool
					value, exists = p.Settings[key]
					if !exists {
						value = setting.Default
					}
					break
				}
			}
		}
		h.mu.RUnlock()
		if !declared {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": value})
	})
}
