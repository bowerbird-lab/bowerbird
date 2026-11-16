package ui

import (
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
)

// SidebarView matches the desktop sidebar collections and folder navigation.
type SidebarView string

const (
	SidebarAll           SidebarView = "all"
	SidebarUntagged      SidebarView = "untagged"
	SidebarUncategorized SidebarView = "uncategorized"
	SidebarHistory       SidebarView = "history"
	SidebarTrash         SidebarView = "trash"
	SidebarFolder        SidebarView = "folder"
)

type visibleFolderRow struct {
	folder model.Folder
	depth  int
}

type bottomDrawerSpec struct {
	Open     func() bool
	OnClose  func()
	Title    string
	Header   []core.Node
	Children []core.Node
	TestID   string
}

// bottomDrawer is the unified bottom drawer used across menu, libraries,
// filters, quick preview, batch actions and metadata. CloseOnSwipe is off so
// vertical scroll views inside the drawer always scroll smoothly.
func (a *App) bottomDrawer(cx *core.Scope, s bottomDrawerSpec) core.Node {
	colors := nsui.ColorsOf(cx)
	var body []core.Node
	body = append(body, core.View{
		StyleFunc: func() core.Style {
			return core.Style{
				Width: core.Pt(38), Height: core.Pt(4), CornerRadius: 2,
				AlignSelf: core.AlignCenter, Background: colors.Border(),
			}
		},
	})
	if s.Title != "" || len(s.Header) > 0 {
		headChildren := make([]core.Node, 0, 1+len(s.Header))
		if s.Title != "" {
			headChildren = append(headChildren, core.Text{
				Content:   s.Title,
				Style:     nsui.TypeTitle.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}),
				StyleFunc: nsui.Colored(colors.Text),
			})
		}
		headChildren = append(headChildren, s.Header...)
		body = append(body, core.View{
			Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
			Children: headChildren,
		})
	}
	body = append(body, s.Children...)

	return base.Sheet{
		Open: s.Open,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open && s.OnClose != nil {
				s.OnClose()
			}
		},
		CloseOnSwipe:  false,
		BackdropStyle: core.Style{Background: themeRole(cx, "overlay")},
		PopupStyleFunc: func() core.Style {
			safe := cx.Env().SafeArea()
			maxH := cx.Env().Window().Height * 0.86
			if maxH <= 0 {
				maxH = 640
			}
			return core.Style{
				Gap:         12,
				MaxHeight:   core.Pt(maxH),
				Padding:     core.Edges{Top: 10, Right: 16, Bottom: safe.Bottom + 16, Left: 16},
				CornerRadii: core.Corners{TopLeft: 20, TopRight: 20},
				Background:  colors.Surface(),
			}
		},
		Children: body,
		Label:    s.Title,
		TestID:   s.TestID,
	}
}

func (a *App) drawerWidth(cx *core.Scope) core.Points {
	win := cx.Env().Window()
	w, h := win.Width, win.Height
	if w <= 0 {
		w = 390
	}
	// Size from the short edge, not the current orientation width: the
	// sidebar then keeps the same width when the phone rotates and its
	// content is fully visible in landscape without covering the gallery.
	short := min(w, h)
	dw := short * 0.78
	if dw < 260 {
		dw = 260
	}
	if dw > 360 {
		dw = 360
	}
	return dw
}

// drawerPosition is the current visual offset of the main panel. While a
// drag is active it follows drawerPos; otherwise it is simply open or shut.
func (a *App) drawerPosition(cx *core.Scope) core.Points {
	if a.drawerDragging.Get() {
		return a.drawerPos.Get()
	}
	if a.drawerOpen.Get() {
		return a.drawerWidth(cx)
	}
	return 0
}

// setDrawerOpen changes where the drawer is going. The open state changes at
// once, so taps, the back button and the scrim stay in step while the style
// transition animates the panel.
func (a *App) setDrawerOpen(cx *core.Scope, open bool) {
	wasOpen := a.drawerOpen.Get()
	if wasOpen != open {
		a.drawerOpen.Set(open)
	}
	if open && !wasOpen {
		a.loadSidebarData(cx)
	}
	target := core.Points(0)
	if open {
		target = a.drawerWidth(cx)
	}
	a.drawerPos.Set(target)
	a.drawerDragging.Set(false)
}

// drawerPan follows a horizontal drag on either the closed gallery, the
// open drawer or its scrim. It updates a signal only while the finger acts;
// release hands the panel to the style transition.
func (a *App) drawerPan(cx *core.Scope) func(core.PanEvent) {
	return func(ev core.PanEvent) {
		dw := a.drawerWidth(cx)
		switch ev.Phase {
		case core.GestureBegan:
			start := core.Points(0)
			if a.drawerOpen.Get() {
				start = dw
			}
			a.drawerDragStart = start
			a.drawerPos.Set(start)
			a.drawerDragging.Set(true)
		case core.GestureChanged:
			if !a.drawerDragging.Get() {
				return
			}
			next := a.drawerDragStart + core.Points(ev.Translation.X)
			if next < 0 {
				next = 0
			}
			if next > dw {
				next = dw
			}
			a.drawerPos.Set(next)
		case core.GestureEnded:
			pos := a.drawerPos.Get()
			open := pos > dw/2
			if ev.Velocity.X > 450 {
				open = true
			} else if ev.Velocity.X < -450 {
				open = false
			}
			a.setDrawerOpen(cx, open)
		case core.GestureCancelled:
			open := a.drawerOpen.Get()
			a.setDrawerOpen(cx, open)
		}
	}
}

// leftDrawerPanel renders Flutter's FolderDrawer as a left side panel with
// library switcher header, collections, and a vertically scrollable folder tree.
func (a *App) leftDrawerPanel(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)

	statText := func(v int) string {
		if !a.statsLoaded {
			return "…"
		}
		if !a.statsSupported {
			return "不可用"
		}
		return itoa(v)
	}

	collectionRow := func(icon func() *core.SVG, label string, view SidebarView, countText func() string, testID string) core.Node {
		selected := func() bool { a.rev.Get(); return a.sidebarView == view }
		return core.View{
			StyleFunc: func() core.Style {
				bg := core.Color(0)
				if selected() {
					bg = colors.AccentFill()
				}
				return core.Style{
					Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10,
					Padding: core.Symmetric(9, 12), CornerRadius: 10, Background: bg,
				}
			},
			OnPress: func(core.PressEvent) {
				a.setDrawerOpen(cx, false)
				a.selectCollection(cx, view)
			},
			TestID: testID,
			Children: []core.Node{
				core.Icon{SVG: icon(), StyleFunc: func() core.Style {
					c := colors.Muted()
					if selected() {
						c = colors.OnAccent()
					}
					return core.Style{FontSize: 18, Color: c}
				}},
				core.Text{Content: label, StyleFunc: func() core.Style {
					c := colors.Text()
					if selected() {
						c = colors.OnAccent()
					}
					return nsui.TypeBody.Merge(core.Style{Color: c, FlexGrow: 1})
				}},
				core.Text{ContentFunc: countText, StyleFunc: func() core.Style {
					c := colors.Muted()
					if selected() {
						c = colors.OnAccent()
					}
					return nsui.TypeCaption.Merge(core.Style{Color: c})
				}},
			},
		}
	}

	folderTree := core.Dynamic(func(cx *core.Scope) core.Node {
		_ = a.rev.Get()
		_ = cx.Env().Dark()
		visibleFolders := a.flattenVisibleFolders()
		folderNodes := make([]core.Node, 0, len(visibleFolders))
		for _, row := range visibleFolders {
			f := row.folder
			depth := row.depth
			selected := a.sidebarView == SidebarFolder && a.activeFolderID == f.ID
			hasChildren := len(f.Children) > 0
			expanded := a.expandedFolders[f.ID]
			bg := core.Color(0)
			fg := colors.Text()
			muted := colors.Muted()
			if selected {
				bg = colors.AccentFill()
				fg = colors.OnAccent()
				muted = colors.OnAccent()
			}
			var expandBtn core.Node = core.View{Style: core.Style{Width: core.Pt(0)}}
			if hasChildren {
				chev := lucide.ChevronRight
				if expanded {
					chev = lucide.ChevronDown
				}
				expandBtn = nsui.IconButton{
					Icon:     chev,
					Label:    "展开文件夹",
					Size:     28,
					IconSize: 16,
					Tint:     func() core.Color { return muted },
					OnPress: func() {
						a.expandedFolders[f.ID] = !expanded
						a.bump()
					},
				}
			}
			folderNodes = append(folderNodes, core.View{
				Style: core.Style{
					Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
					Margin:  core.Edges{Left: core.Points(depth * 14)},
					Padding: core.Symmetric(8, 10), CornerRadius: 8, Background: bg,
				},
				OnPress: func(core.PressEvent) {
					a.setDrawerOpen(cx, false)
					a.selectFolder(cx, f.ID)
				},
				Children: []core.Node{
					core.Icon{SVG: lucide.Folder(), Style: core.Style{FontSize: 16, Color: muted}},
					core.Text{Content: f.Name, MaxLines: 1, Style: nsui.TypeBody.Merge(core.Style{Color: fg, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)})},
					core.Text{Content: itoa(f.ItemCount), Style: nsui.TypeCaption.Merge(core.Style{Color: muted})},
					expandBtn,
				},
			})
		}
		return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 12, Bottom: 2, Left: 4, Right: 4}}, Children: []core.Node{
				core.Text{Content: "文件夹 (" + itoa(len(a.folders)) + ")", Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
			}},
			core.Show{When: func() bool { return len(folderNodes) == 0 }, Then: func(cx *core.Scope) core.Node {
				return core.Text{Content: "暂无文件夹", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Faint(), Padding: core.Symmetric(16, 4)})}
			}},
			core.View{Style: core.Style{Gap: 2, Padding: core.Edges{Bottom: 20}}, Children: folderNodes},
		}}
	})

	return core.View{
		TestID: "drawer.panel",
		StyleFunc: func() core.Style {
			dw := a.drawerWidth(cx)
			return core.Style{
				Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0),
				Width: core.Pt(dw), Background: themeRole(cx, "sidebar"),
				ZIndex: 10,
			}
		},
		Children: []core.Node{
			// Library switcher + Search row (matches Flutter FolderDrawer header)
			core.View{
				StyleFunc: func() core.Style {
					safe := cx.Env().SafeArea()
					bg := themeRole(cx, "sidebar")
					return core.Style{
						Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(0),
						Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6,
						Padding: core.Edges{Top: safe.Top + 10, Left: safe.Left + 12, Bottom: 8, Right: 12},
						ZIndex:  2,
						Gradients: []core.Gradient{
							core.LinearGradient(180, bg, nsui.Alpha(bg, 0)),
						},
					}
				},
				PanActivation: core.PanActivation{ActiveOffsetX: 18, FailOffsetY: 8},
				OnPan:         a.drawerPan(cx),
				Children: []core.Node{
					core.View{
						TestID:  "drawer.library",
						Style:   core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(8, 6), CornerRadius: 10},
						OnPress: func(core.PressEvent) { a.openLibraries(cx) },
						Children: []core.Node{
							core.Text{ContentFunc: func() string { a.rev.Get(); return a.libraryName() }, MaxLines: 1,
								Style: nsui.TypeTitle.Merge(core.Style{FlexShrink: 1}), StyleFunc: nsui.Colored(colors.Text)},
							core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{FontSize: 16}, StyleFunc: nsui.Colored(colors.Muted)},
						},
					},
					nsui.IconButton{Icon: lucide.Search, Label: "搜索", Size: 34, IconSize: 18, OnPress: func() {
						a.setDrawerOpen(cx, false)
						a.openSearchPage()
					}},
				},
			},
			core.Scroll{
				TestID: "drawer.scroll",
				Style:  core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
				Behavior: core.ScrollBehavior{
					NoBounces:        true,
					DecelerationRate: core.DecelerationFast,
				},
				Children: []core.Node{
					core.View{StyleFunc: func() core.Style {
						safe := cx.Env().SafeArea()
						return core.Style{Gap: 2, Padding: core.Edges{
							Top: safe.Top + 56, Bottom: safe.Bottom + 8, Left: safe.Left + 12, Right: 12,
						}}
					}, Children: []core.Node{
						collectionRow(lucide.Inbox, "全部素材", SidebarAll, func() string { a.rev.Get(); return statText(a.stats.Total) }, "drawer.all"),
						collectionRow(lucide.Tag, "未打标签", SidebarUntagged, func() string { a.rev.Get(); return statText(a.stats.UntaggedCount) }, "drawer.untagged"),
						collectionRow(lucide.FolderOpen, "未分类", SidebarUncategorized, func() string { a.rev.Get(); return statText(a.stats.UncategorizedCount) }, "drawer.uncategorized"),
						collectionRow(lucide.Clock, "使用历史", SidebarHistory, func() string { a.rev.Get(); return itoa(len(a.historyIDs)) }, "drawer.history"),
						collectionRow(lucide.Trash, "废纸篓", SidebarTrash, func() string { a.rev.Get(); return statText(a.stats.TrashCount) }, "drawer.trash"),
						folderTree,
					}},
				},
			},
		},
	}
}

func (a *App) flattenVisibleFolders() []visibleFolderRow {
	var rows []visibleFolderRow
	var visit func(f model.Folder, depth int)
	visit = func(f model.Folder, depth int) {
		rows = append(rows, visibleFolderRow{folder: f, depth: depth})
		if len(f.Children) == 0 || !a.expandedFolders[f.ID] {
			return
		}
		for _, child := range f.Children {
			visit(child, depth+1)
		}
	}
	for _, root := range a.folders {
		visit(root, 0)
	}
	return rows
}

func (a *App) findFolderName(id string) string {
	var walk func(list []model.Folder) string
	walk = func(list []model.Folder) string {
		for _, f := range list {
			if f.ID == id {
				return f.Name
			}
			if name := walk(f.Children); name != "" {
				return name
			}
		}
		return ""
	}
	return walk(a.folders)
}

func (a *App) currentViewTitle() string {
	switch a.sidebarView {
	case SidebarUntagged:
		return "未打标签"
	case SidebarUncategorized:
		return "未分类"
	case SidebarHistory:
		return "使用历史"
	case SidebarTrash:
		return "废纸篓"
	case SidebarFolder:
		if name := strings.TrimSpace(a.findFolderName(a.activeFolderID)); name != "" {
			return name
		}
		return "文件夹"
	default:
		return a.libraryName()
	}
}
