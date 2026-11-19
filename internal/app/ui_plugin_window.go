package app

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"bowerbird/internal/plugin"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// RegisterPluginUIProtocol must be called before creating any windows.
// Each request is associated with a page by mygo, never by the URL supplied
// by plugin code. Plugin pages are untrusted and receive no Go bindings.
func (a *App) RegisterPluginUIProtocol() error {
	return mygo.Protocol.HandleFunc("bowerplugin", func(w http.ResponseWriter, r *http.Request) {
		a.pluginUISessions.ServePage(mygo.CallerPage(r.Context()), w, r)
	})
}

func (a *App) openPluginUI(pluginID string, entry plugin.PluginUIEntry) error {
	if a.pluginHost == nil {
		return fmt.Errorf("plugin host is unavailable")
	}
	p, ok := a.pluginHost.Get(pluginID)
	if !ok || !p.Enabled || p.Dir == "" {
		return fmt.Errorf("plugin is not enabled")
	}
	dir := p.Dir
	found := false
	for _, candidate := range p.Manifest.UI.Windows {
		if candidate.ID == entry.ID && candidate.Entry == entry.Entry {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("plugin window is not declared")
	}
	key := pluginID + ":" + entry.ID
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
	var view *mygo.WebView
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: title, Width: 900, Height: 650, MinWidth: 480, MinHeight: 360,
		Content: ui.View(func(c *ui.Context) {
			if view != nil {
				ui.WebView(c, view).Fill()
			}
		}),
	})
	created, err := win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewEphemeralSession()}})
	if err != nil {
		win.Destroy()
		return err
	}
	view = created
	page := view.Page()
	a.pluginUISessions.Attach(page, dir, func() bool {
		current, ok := a.pluginHost.Get(pluginID)
		return ok && a.pluginHost.IsEnabled(pluginID) && current.Dir == dir
	})
	a.pluginUIWindows[key] = win
	win.OnClosed(func() { a.pluginUISessions.Detach(page); delete(a.pluginUIWindows, key) })
	// Escape each path segment without allowing entry to alter the URL origin.
	parts := strings.Split(entry.Entry, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	if err := page.LoadURL("bowerplugin://localhost/" + strings.Join(parts, "/")); err != nil {
		a.pluginUISessions.Detach(page)
		delete(a.pluginUIWindows, key)
		win.Destroy()
		return err
	}
	win.Invalidate()
	return nil
}
