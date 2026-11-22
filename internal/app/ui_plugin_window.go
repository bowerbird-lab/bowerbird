package app

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"bowerbird/core"
	"bowerbird/internal/plugin"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const pluginDownloadTempPrefix = "bowerbird-plugin-dl-"

// RegisterPluginUIProtocol must be called before creating any windows.
// Each request is associated with a page by mygo, never by the URL supplied
// by plugin code. Plugin pages are untrusted and receive no Go bindings.
func (a *App) RegisterPluginUIProtocol() error {
	return mygo.Protocol.HandleFunc("bowerplugin", func(w http.ResponseWriter, r *http.Request) {
		a.pluginUISessions.ServePage(mygo.CallerPage(r.Context()), w, r)
	})
}

func (a *App) openPluginUI(pluginID string, entry plugin.PluginUIEntry) error {
	return a.openPluginUIForItem(pluginID, entry, a.activeItem)
}

func (a *App) openPluginUIForItem(pluginID string, entry plugin.PluginUIEntry, item *core.Item) error {
	if a.pluginHost == nil {
		return fmt.Errorf("plugin host is unavailable")
	}
	p, ok := a.pluginHost.Get(pluginID)
	isExternal := entry.URL != ""
	if !ok || !p.Enabled || (!isExternal && p.Dir == "") {
		return fmt.Errorf("plugin is not enabled")
	}
	dir := p.Dir
	found := false
	for _, candidate := range p.Manifest.UI.Windows {
		if candidate.ID == entry.ID && candidate.Entry == entry.Entry && candidate.URL == entry.URL && candidate.InterceptDownloads == entry.InterceptDownloads {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("plugin window is not declared")
	}
	key := pluginID + ":" + entry.ID
	if !isExternal && item != nil {
		key += ":" + item.ID
	}
	if a.pluginUIWindows == nil {
		a.pluginUIWindows = make(map[string]*mygo.Window)
	}
	if existing := a.pluginUIWindows[key]; existing != nil {
		existing.Show()
		existing.Focus()
		return nil
	}
	title := entry.Title
	if title == "" {
		title = p.Manifest.Name
	}
	// Use a per-plugin persistent session so IndexedDB/localStorage survive
	// window closes while remaining isolated and Untrusted.
	session := mygo.NewSession("plugin:" + pluginID)
	var view *mygo.WebView
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: title, Width: 1100, Height: 760, MinWidth: 480, MinHeight: 360,
		Content: ui.View(func(c *ui.Context) {
			if view != nil {
				ui.WebView(c, view).Fill()
			}
		}),
	})
	created, err := win.NewWebView(mygo.WebViewOptions{
		Transparent: !isExternal,
		Page:        mygo.PageOptions{Untrusted: true, Session: session},
	})
	if err != nil {
		win.Destroy()
		return err
	}
	view = created
	page := view.Page()

	if isExternal {
		page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if !isAllowedExternalNavigation(e.URL) {
				e.PreventDefault()
			}
		})
		page.SetWindowOpenHandler(func(r mygo.WindowOpenRequest) *mygo.WindowOptions {
			if !isAllowedExternalNavigation(r.URL) {
				return nil
			}
			w, h := r.Width, r.Height
			if w <= 0 {
				w = 900
			}
			if h <= 0 {
				h = 700
			}
			return &mygo.WindowOptions{
				Title:  title,
				Width:  w,
				Height: h,
				Page:   mygo.PageOptions{Untrusted: true, Session: session},
			}
		})
	} else {
		var selected *core.Item
		if item != nil {
			copy := *item
			selected = &copy
		}
		a.pluginUISessions.Attach(page, dir, func() bool {
			current, ok := a.pluginHost.Get(pluginID)
			return ok && a.pluginHost.IsEnabled(pluginID) && current.Dir == dir
		}, a.pluginWindowAPI(pluginID, selected))
	}

	if entry.InterceptDownloads {
		a.attachPluginDownloadInterceptor(page)
	}

	a.pluginUIWindows[key] = win
	win.OnClosed(func() {
		if !isExternal {
			a.pluginUISessions.Detach(page)
		}
		delete(a.pluginUIWindows, key)
	})

	targetURL := entry.URL
	if !isExternal {
		// Escape each path segment without allowing entry to alter the URL origin.
		parts := strings.Split(entry.Entry, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		targetURL = "bowerplugin://localhost/" + strings.Join(parts, "/")
	}
	if err := page.LoadURL(targetURL); err != nil {
		if !isExternal {
			a.pluginUISessions.Detach(page)
		}
		delete(a.pluginUIWindows, key)
		win.Destroy()
		return err
	}
	win.Invalidate()
	return nil
}

func isAllowedExternalNavigation(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http", "about", "blob", "data":
		return true
	default:
		return false
	}
}

func (a *App) attachPluginDownloadInterceptor(page *mygo.Page) {
	page.OnWillDownload(func(e *mygo.DownloadEvent) {
		tmpDir, err := os.MkdirTemp("", pluginDownloadTempPrefix+"*")
		if err != nil {
			e.PreventDefault()
			return
		}
		name := filepath.Base(filepath.Clean("/" + strings.ReplaceAll(e.SuggestedName, `\`, "/")))
		if name == "/" || name == "." || name == "" {
			name = "download"
		}
		e.Path = filepath.Join(tmpDir, name)
	})
	page.OnDownloadDone(func(d *mygo.Download) {
		go a.handlePluginDownloadDone(d)
	})
}

func (a *App) handlePluginDownloadDone(d *mygo.Download) {
	if d == nil {
		return
	}
	cleanup := func() {
		if d.Path == "" {
			return
		}
		dir := filepath.Dir(d.Path)
		if strings.HasPrefix(filepath.Base(dir), pluginDownloadTempPrefix) {
			_ = os.RemoveAll(dir)
		}
	}
	defer cleanup()

	if d.Err != nil {
		errMsg := d.Err.Error()
		if a.win != nil {
			a.post(func() {
				a.showToast(a.tf("settings.plugins.downloadFailed", map[string]any{"error": errMsg}))
			})
		}
		return
	}
	if d.Path == "" {
		return
	}
	items, err := a.importPluginDownloadFile(d.Path)
	if err != nil {
		errMsg := err.Error()
		if a.win != nil {
			a.post(func() {
				a.showToast(a.tf("settings.plugins.downloadFailed", map[string]any{"error": errMsg}))
			})
		}
		return
	}
	name := filepath.Base(d.Path)
	if len(items) > 0 && items[0].Name != "" {
		name = items[0].Name
	}
	if a.win != nil {
		a.post(func() {
			a.refreshAll()
			a.showToast(a.tf("settings.plugins.downloadSaved", map[string]any{"name": name}))
		})
	}
}

func (a *App) importPluginDownloadFile(filePath string) ([]core.Item, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("service unavailable")
	}
	return a.svc.ImportFiles([]string{filePath})
}
