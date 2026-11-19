package app

import (
	"runtime"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// commonHeaderOptions tunes the shared header panel. spansWindow is set by
// the full-window preview, whose header covers the sidebar and the inspector
// and therefore has to clear the window controls on both sides. pad is the
// gutter the row keeps inside those insets. height is the panel's full height:
// one row in the previews, the workspace toolbar's two rows in the gallery.
// floating pins the panel over the content from the parent's top edge, and
// contentUnder asks for the blur material once content scrolls beneath it.
type commonHeaderOptions struct {
	spansWindow  bool
	pad          float32
	height       float32
	floating     bool
	contentUnder bool
	// hideSidebarToggle leaves the row to its body alone. The full-window
	// preview uses it: that header only offers a close button.
	hideSidebarToggle bool
}

// commonHeader is the header panel every workspace view shares. It owns the
// surface — the workspace fill at rest, or the same blur material and tint as
// the settings header once content scrolls under — the window-drag strip and
// the sidebar toggle. The toggle stays visible in all views, so the panel can
// be collapsed or restored without leaving the workspace, the inline preview
// or the full-window preview. body fills the first row; extra, when set, adds
// the rows below it before the material's divider.
func (a *App) commonHeader(c *ui.Context, opts commonHeaderOptions, body, extra func()) {
	height := opts.height
	if height <= 0 {
		height = headerHeight
	}
	header := ui.Column(c).FillWidth().Height(height)
	if opts.floating {
		header.Absolute().Top(0).Left(0).Right(0)
	}
	// The surface belongs to the panel, so it paints below the controls
	// rather than over them.
	blurActive := a.applyHeaderSurface(c, header, opts.contentUnder)
	header.Children(func() {
		leftPad, rightPad := a.headerInsets(c, opts.spansWindow, opts.pad)
		ui.Row(c).FillWidth().Height(headerHeight).Padding(0, rightPad, 0, leftPad).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			if !opts.hideSidebarToggle {
				a.iconButton(c, "sidebar-toggle", iconSidebar, false, a.t("common.toggleSidebar"), a.toggleSidebar)
			}
			body()
		})
		if extra != nil {
			extra()
		}
		if blurActive {
			ui.Box(c).Absolute().Bottom(0).FillWidth().Height(1).Background(a.palette(c).hairline)
		}
	})
}

// headerInsets returns the row's horizontal padding. The header clears the
// traffic lights or window buttons only where it reaches the window edge:
// past the sidebar on the left, past the inspector on the right, or on both
// sides in the full-window preview.
func (a *App) headerInsets(c *ui.Context, spansWindow bool, pad float32) (left, right float32) {
	bar := c.TitleBar()
	left = pad
	if spansWindow || !a.prefs.SidebarOpen {
		left += bar.Left
	}
	right = pad
	if spansWindow || !a.inspectorVisible() {
		r := bar.Right
		if r == 0 && runtime.GOOS == "windows" {
			r = 138
		}
		right += r
	}
	return left, right
}

// viewHeader is the toolbar: history icons, search, sort, filters and the
// thumbnail zoom. Layout and panel visibility live in the content's context
// menu so the view-toggle icons stay out of the header.
func (a *App) viewHeader(c *ui.Context) {
	p := a.palette(c)
	if !a.searchAt.IsZero() && time.Now().After(a.searchAt) {
		a.searchAt = time.Time{}
		a.refreshAll()
	}
	a.commonHeader(c, commonHeaderOptions{
		pad:          gallerySidePadding,
		height:       mainHeaderHeight,
		floating:     true,
		contentUnder: a.headerHasContentUnder(),
	}, func() {
		// History always sits at the left end of the row, right after the
		// sidebar toggle; the preview keeps its step buttons there too.
		a.iconButton(c, "back", iconBack, false, a.t("common.back"), func() { a.goHistory(-1) })
		a.iconButton(c, "forward", iconForward, false, a.t("common.forward"), func() { a.goHistory(1) })

		zoomVal := float64(a.rowHeight)
		ui.Slider(c.Key("zoom-slider"), &zoomVal, 90, 420).Width(110)
		if int(zoomVal) != a.rowHeight {
			a.setRowHeight(int(zoomVal))
		}

		ui.Box(c).Grow(1)

		if a.nav == NavFolder && a.folderID != "" {
			name := a.folderDisplayName(a.folderID)
			ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(p.text).MaxWidth(240).SingleLine()
			ui.Box(c).Grow(1)
		}

		a.viewSearchField(c)
	}, func() {
		ui.Row(c).FillWidth().Height(headerSecondRowHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			// Sort order and the multi-facet filter live on their own row so
			// neither label gets squeezed on narrow windows.
			sortLabel := a.t("header.newest")
			sortIcon := iconArrowDown
			if a.sortOrder == "asc" {
				sortLabel = a.t("header.oldest")
				sortIcon = iconArrowUp
			}
			a.pillIconButton(c, "sort", sortIcon, sortLabel, false, func() {
				if a.sortOrder == "asc" {
					a.sortOrder = "desc"
				} else {
					a.sortOrder = "asc"
				}
				a.prefs.SortOrder = a.sortOrder
				a.savePrefs()
				a.refreshAll()
			})

			ui.Box(c).Width(1).Height(20).Background(p.hairline)
			a.viewFilterBar(c)
		})
	})
}

// applyHeaderSurface gives a header panel the workspace background while
// nothing scrolls under it, so it reads as part of the same surface (white in
// light mode, dark in dark mode). Once content reaches it, the shared blur
// material and tint take over and the caller adds a divider. The workspace,
// preview and settings headers all use it, so their chrome matches. It returns
// whether that scrolled state is active.
func (a *App) applyHeaderSurface(c *ui.Context, header ui.Element, contentUnder bool) bool {
	p := a.palette(c)
	if !contentUnder {
		header.Background(p.bg)
		return false
	}
	// Both the system material switch and this header's own option must be on;
	// otherwise the scrolled header stays opaque.
	if !a.vibrancyShows(c) || !a.prefs.VibrancyHeader {
		header.Background(p.bg)
		return false
	}
	header.Material(glass.Blur{Radius: p.headerBlur})
	tint := p.headerTint
	header.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(r, tint, 0)
	})
	return true
}

// headerHasContentUnder reports whether the gallery has scrolled far enough
// for its first row to reach the floating header. Until then the header stays
// flush with the workspace background and shows no blur or divider.
func (a *App) headerHasContentUnder() bool {
	return a.galleryScroll.Y > float32(galleryTopPadding)
}

// viewSearchField draws the search control at the right end of the header.
// The outer pill owns the border; the input itself is unstyled so it does
// not draw a second border inside it.
func (a *App) viewSearchField(c *ui.Context) {
	p := a.palette(c)
	row := ui.Row(c).Height(32).Grow(1).Shrink(1).MinWidth(160).MaxWidth(380).Padding(0, 10).Radius(10).Background(p.surface2).Border(1, p.hairline).AlignItems(ui.Center).Gap(7)
	row.Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.faint)
		field := ui.TextInputBase(c.Key("search"), &a.query).Bind(&a.searchBox).
			Placeholder(a.t("header.searchPlaceholder")).
			PlaceholderColor(p.faint).
			TextColor(p.text).
			FontSize(12).
			Grow(1).Shrink(1).MinWidth(0)
		if field.Focused() {
			row.Border(1, p.accent)
		}
		if field.Changed() {
			a.searchAt = time.Now().Add(320 * time.Millisecond)
			c.After(320 * time.Millisecond)
		}
		if field.Submitted() {
			a.searchAt = time.Time{}
			a.refreshAll()
		}
		if a.query != "" {
			if ui.ButtonBase(c.Key("search-clear")).Size(18, 18).Radius(999).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, "×").FontSize(13).FontWeight(500).TextColor(p.muted)
			}).Clicked() {
				a.query = ""
				a.refreshAll()
			}
		}
	})
}

func (a *App) folderDisplayName(id string) string {
	if f, ok := a.folderIndex().byID[id]; ok {
		return f.Name
	}
	return ""
}

var _ = core.ThemeDark
