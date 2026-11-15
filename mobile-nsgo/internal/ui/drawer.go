package ui

import (
	"strings"

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

func (a *App) drawerSheet(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()

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
				a.drawerOpen.Set(false)
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
					a.drawerOpen.Set(false)
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
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 8}}, Children: []core.Node{
				core.Text{Content: "文件夹 (" + itoa(len(a.folders)) + ")", Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
			}},
			core.Scroll{
				Style: core.Style{MaxHeight: core.Pt(260)},
				Children: []core.Node{
					core.Show{When: func() bool { return len(folderNodes) == 0 }, Then: func(cx *core.Scope) core.Node {
						return core.Text{Content: "暂无文件夹", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Faint(), Padding: core.Symmetric(12, 4)})}
					}},
					core.View{Style: core.Style{Gap: 2}, Children: folderNodes},
				},
			},
		}}
	})

	return nsui.Sheet{
		Open:    func() bool { return a.drawerOpen.Get() },
		OnClose: func() { a.drawerOpen.Set(false) },
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				core.View{
					Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(6, 4)},
					OnPress: func(core.PressEvent) {
						a.drawerOpen.Set(false)
						a.openLibraries(cx)
					},
					Children: []core.Node{
						core.Text{ContentFunc: func() string { a.rev.Get(); return a.libraryName() }, MaxLines: 1,
							Style: nsui.TypeTitle.Merge(core.Style{Color: colors.Text(), FlexShrink: 1})},
						core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{FontSize: 16, Color: colors.Muted()}},
					},
				},
				nsui.Button{Text: "搜索", Variant: nsui.ButtonGhost, OnPress: func() {
					a.drawerOpen.Set(false)
					a.openSearchPage()
				}},
			}},
			collectionRow(lucide.Inbox, "全部素材", SidebarAll, func() string { a.rev.Get(); return statText(a.stats.Total) }, "drawer.all"),
			collectionRow(lucide.Tag, "未打标签", SidebarUntagged, func() string { a.rev.Get(); return statText(a.stats.UntaggedCount) }, "drawer.untagged"),
			collectionRow(lucide.FolderOpen, "未分类", SidebarUncategorized, func() string { a.rev.Get(); return statText(a.stats.UncategorizedCount) }, "drawer.uncategorized"),
			collectionRow(lucide.Clock, "使用历史", SidebarHistory, func() string { a.rev.Get(); return itoa(len(a.historyIDs)) }, "drawer.history"),
			collectionRow(lucide.Trash, "废纸篓", SidebarTrash, func() string { a.rev.Get(); return statText(a.stats.TrashCount) }, "drawer.trash"),
			folderTree,
		},
	}
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
