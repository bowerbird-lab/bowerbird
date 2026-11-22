package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"bowerbird/core"
	"github.com/egoist/mygo/ui"
)

const maxPluginAssetBytes = 24 << 20

// pluginWindowAPI uses the selected item captured when the user opens the
// window. Requests cannot name or read arbitrary library items or paths.
func (a *App) pluginWindowAPI(id string, selected *core.Item) http.Handler {
	var libraryID, libraryPath string
	if a.svc != nil {
		if library, err := a.svc.GetActiveLibrary(); err == nil && library != nil {
			libraryID, libraryPath = library.ID, library.Path
		}
	}
	libraryAvailable := func() bool {
		if a.svc == nil || libraryPath == "" {
			return false
		}
		library, err := a.svc.GetActiveLibrary()
		return err == nil && library != nil && library.ID == libraryID && library.Path == libraryPath
	}
	mux := http.NewServeMux()
	mux.Handle("/_api/settings/", a.pluginHost.UISettingsHandler(id))
	mux.Handle("/_api/network", a.pluginHost.UINetworkHandler(id))
	mux.HandleFunc("/_api/theme", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mode":  a.themeMode(),
			"light": paletteToJSON(a.paletteLight),
			"dark":  paletteToJSON(a.paletteDark),
		})
	})
	mux.HandleFunc("/_api/asset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		if !a.pluginHost.UIGranted(id, "assets.read") {
			http.Error(w, "asset read permission denied", 403)
			return
		}
		if selected == nil || !libraryAvailable() {
			http.Error(w, "no image selected", 404)
			return
		}
		item, err := a.svc.GetItem(selected.ID)
		if err != nil || item == nil {
			http.Error(w, "asset not found", 404)
			return
		}
		file, err := os.Open(item.FilePath)
		if err != nil {
			http.Error(w, "cannot read asset", 500)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxPluginAssetBytes+1))
		if err != nil || len(data) > maxPluginAssetBytes {
			http.Error(w, "asset too large or unreadable", 413)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": item.ID, "name": item.Name, "filename": item.Filename, "mimeType": item.MimeType, "bodyBase64": base64.StdEncoding.EncodeToString(data)})
	})
	mux.HandleFunc("/_api/asset/raw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", 405)
			return
		}
		if !a.pluginHost.UIGranted(id, "assets.read") {
			http.Error(w, "asset read permission denied", 403)
			return
		}
		if selected == nil || !libraryAvailable() {
			http.Error(w, "no asset selected", 404)
			return
		}
		item, err := a.svc.GetItem(selected.ID)
		if err != nil || item == nil {
			http.Error(w, "asset not found", 404)
			return
		}
		file, err := os.Open(item.FilePath)
		if err != nil {
			http.Error(w, "cannot read asset", 500)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.Error(w, "cannot read asset", 500)
			return
		}
		contentType := item.MimeType
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(item.Filename))
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, item.Filename, info.ModTime(), file)
	})
	mux.HandleFunc("/_api/asset/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if !a.pluginHost.UIGranted(id, "assets.write") {
			http.Error(w, "asset write permission denied", 403)
			return
		}
		if !libraryAvailable() {
			http.Error(w, "library unavailable", 503)
			return
		}
		var input struct {
			Mode     string `json:"mode"`
			Filename string `json:"filename"`
			Body     string `json:"bodyBase64"`
			Prompt   string `json:"prompt"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxPluginAssetBytes*2)).Decode(&input); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		data, err := base64.StdEncoding.DecodeString(input.Body)
		if err != nil || len(data) == 0 || len(data) > maxPluginAssetBytes {
			http.Error(w, "invalid asset", 400)
			return
		}
		var item *core.Item
		switch input.Mode {
		case "create":
			item, err = a.svc.SaveNewItemFromBytes(data, filepath.Base(input.Filename), core.ImportItemOptions{Prompt: input.Prompt})
		case "replace":
			if selected == nil {
				http.Error(w, "no selected asset", 404)
				return
			}
			item, err = a.svc.ReplaceItemWithBackup(selected.ID, data, core.ReplaceItemOptions{Prompt: input.Prompt})
		case "variant", "page":
			if selected == nil {
				http.Error(w, "no selected asset", 404)
				return
			}
			parentID := selected.ID
			if input.Mode == "page" && selected.VariantOf != "" {
				parentID = selected.VariantOf
			}
			item, err = a.svc.SaveVariantFromBytes(parentID, data, filepath.Base(input.Filename), core.ImportItemOptions{Prompt: input.Prompt})
		default:
			http.Error(w, "invalid save mode", 400)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		a.post(func() {
			if selected != nil {
				a.invalidateItemBitmaps(selected)
			}
			if item != nil {
				a.invalidateItemBitmaps(item)
			}
			a.refreshAll()
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": item.ID})
	})
	return mux
}

func cssColor(c ui.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %.3f)", c.R, c.G, c.B, float64(c.A)/255.0)
}

func paletteToJSON(p palette) map[string]any {
	return map[string]any{
		"dark": p.dark,
		"colors": map[string]string{
			"bg":       cssColor(p.bg),
			"sidebar":  cssColor(p.sidebar),
			"surface":  cssColor(p.surface),
			"surface2": cssColor(p.surface2),
			"text":     cssColor(p.text),
			"muted":    cssColor(p.muted),
			"faint":    cssColor(p.faint),
			"hairline": cssColor(p.hairline),
			"hover":    cssColor(p.hover),
			"press":    cssColor(p.press),
			"accent":   cssColor(p.accent),
			"onAccent": cssColor(p.onAccent),
			"danger":   cssColor(p.danger),
			"warning":  cssColor(p.warning),
			"info":     cssColor(p.info),
		},
		"radius": map[string]float32{
			"button": p.buttonRadius,
			"menu":   p.menuRadius,
			"pill":   p.pillRadius,
		},
	}
}
