package app

import (
	"github.com/egoist/mygo/ui"
)

// applyVibrancyRoot clears the view root's default theme background so the
// window material can show through the surfaces that opt into it.
func (a *App) applyVibrancyRoot(c *ui.Context) {
	if c.Vibrancy() && a.vibrancyEnabled() {
		c.Root().Background(ui.Transparent)
	}
}

// vibrancyEnabled reports whether the user has the system blur material
// switched on. The switch lives in the core service; the cached value covers
// the views built before a service exists (tests and early startup).
func (a *App) vibrancyEnabled() bool {
	if a.svc != nil {
		return a.svc.GetVibrancyEnabled()
	}
	return a.vibrancyValue
}

// vibrancyShows reports whether the window's native material is on and
// actually visible, so a surface may leave itself transparent for it.
func (a *App) vibrancyShows(c *ui.Context) bool {
	return c.Vibrancy() && a.vibrancyEnabled()
}

// applySidebarSurface paints a sidebar: a translucent tint over the window
// material when the user opted in, or the opaque palette fill otherwise.
func (a *App) applySidebarSurface(c *ui.Context, e ui.Element) {
	p := a.palette(c)
	if a.vibrancyShows(c) && a.prefs.VibrancySidebar {
		e.Background(p.sidebar.Alpha(p.sidebarOpacity))
		return
	}
	e.Background(p.sidebar)
}
