package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	insetOuterPadding float32 = 10
	insetCardRadius   float32 = 12
)

// applyVibrancyRoot clears the view root's default theme background so the
// window material can show through the surfaces that opt into it.
func (a *App) applyVibrancyRoot(c *ui.Context) {
	if c.Vibrancy() && (a.vibrancyEnabled() || a.prefs.ExperimentalInsetUI) {
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

// InsetUIEnabled reports whether the experimental inset rounded UI is enabled.
func (a *App) InsetUIEnabled() bool {
	return a.prefs.ExperimentalInsetUI
}

// insetUIActive reports whether the experimental inset card layout should be
// rendered for the main window.
func (a *App) insetUIActive(c *ui.Context) bool {
	if a.isImmersiveFullscreen() {
		return false
	}
	return a.insetUIWindowActive(c, a.win)
}

// insetUIWindowActive reports whether the experimental inset card layout
// should be rendered for w (or the current window when w is nil).
func (a *App) insetUIWindowActive(c *ui.Context, w *mygo.Window) bool {
	if !a.prefs.ExperimentalInsetUI || !c.Vibrancy() {
		return false
	}
	if w != nil && w.IsFullScreen() {
		return false
	}
	return true
}

// WindowVibrancyMaterial returns the native window backdrop material to apply
// based on the vibrancy switch and experimental inset UI preference.
func (a *App) WindowVibrancyMaterial(vibrancyEnabled bool) mygo.Vibrancy {
	if a.prefs.ExperimentalInsetUI {
		if runtime.GOOS == "windows" {
			return mygo.VibrancyAcrylic
		}
		return mygo.VibrancySidebar
	}
	if vibrancyEnabled {
		return mygo.VibrancySidebar
	}
	return mygo.VibrancyNone
}

// syncWindowVibrancy updates the native backdrop material on all open windows.
func (a *App) syncWindowVibrancy() {
	mat := a.WindowVibrancyMaterial(a.vibrancyEnabled())
	for _, w := range []*mygo.Window{a.win, a.settingsWin, a.logsWin, a.setupWin} {
		if w != nil {
			w.SetVibrancy(mat)
			w.Invalidate()
		}
	}
}

func (a *App) insetFrameTint(p palette) ui.Color {
	if p.dark {
		return p.bg.Alpha(0.26)
	}
	return p.bg.Alpha(0.20)
}

func (a *App) insetFrameTopPadding(c *ui.Context, w *mygo.Window) float32 {
	bar := c.TitleBar()
	if runtime.GOOS == "windows" && bar.Right == 0 {
		return insetOuterPadding
	}
	if bar.Left == 0 && bar.Right == 0 {
		return insetOuterPadding
	}
	if bar.Height > 0 {
		return maxF(40, bar.Height)
	}
	return 44
}

func (a *App) customWindowControlsActive(c *ui.Context, w *mygo.Window) bool {
	return runtime.GOOS == "windows" && c.TitleBar().Right == 0
}

func (a *App) viewWindowControls(c *ui.Context, w *mygo.Window) {
	p := a.palette(c)
	if w == nil {
		return
	}
	winBtn := func(key string, icon *ui.SVG, danger bool, isMax bool, onClick func()) {
		b := ui.ButtonBase(c.Key(key)).Size(28, 28).Radius(7).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center)
		if isMax {
			b.MaximizeButton()
		}
		bg := ui.Transparent
		fg := p.muted
		if b.Hovered() {
			if danger {
				bg = ui.RGB(196, 43, 28)
				fg = ui.RGB(255, 255, 255)
			} else {
				bg = p.hover
				fg = p.text
			}
		}
		b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
		b.Children(func() {
			ui.Icon(c, icon).Size(13, 13).TextColor(fg)
		})
		if b.Clicked() {
			onClick()
		}
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		winBtn("win-min", iconMinus, false, false, w.Minimize)
		maxIcon := iconWindowMaximize
		if w.IsMaximized() {
			maxIcon = iconWindowRestore
		}
		winBtn("win-max", maxIcon, false, true, func() {
			if w.IsMaximized() {
				w.Unmaximize()
			} else {
				w.Maximize()
			}
		})
		winBtn("win-close", iconClose, true, false, w.Close)
	})
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
