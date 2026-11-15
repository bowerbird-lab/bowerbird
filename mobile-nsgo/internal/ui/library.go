package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"
)

func (a *App) menuDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	row := func(icon func() *core.SVG, label, badge string, destructive bool, press func()) core.Node {
		fg := colors.Text()
		ic := colors.Muted()
		if destructive {
			fg = colors.Danger()
			ic = colors.Danger()
		}
		var badgeNode core.Node = core.View{}
		if badge != "" {
			badgeNode = core.Text{Content: badge, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})}
		}
		return core.View{
			Style:   core.Style{Width: core.Pct(100), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 12), CornerRadius: 10, Background: colors.Inset()},
			OnPress: func(core.PressEvent) { press() },
			Children: []core.Node{
				core.Icon{SVG: icon(), Style: core.Style{FontSize: 18, Color: ic}},
				core.Text{Content: label, Style: nsui.TypeBody.Merge(core.Style{Color: fg, FlexGrow: 1})},
				badgeNode,
			},
		}
	}

	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.menuOpen.Get() },
		OnClose: func() { a.menuOpen.Set(false) },
		Title:   "更多",
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.menuOpen.Get()
				filterBadge := ""
				if a.galleryFilter.String() != "" {
					filterBadge = "已筛选"
				}
				inboxBadge := ""
				if len(a.inboxEntries) > 0 {
					inboxBadge = itoa(len(a.inboxEntries))
				}
				colRow := core.View{
					Style: core.Style{Width: core.Pct(100), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
						Padding: core.Symmetric(10, 12), CornerRadius: 10, Background: colors.Inset()},
					Children: []core.Node{
						core.View{Style: core.Style{FlexGrow: 1, Gap: 1}, Children: []core.Node{
							core.Text{Content: "网格列数", Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
							core.Text{ContentFunc: func() string { return itoa(a.gridCols.Get()) + " 列（支持双指捏合）" },
								Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
						}},
						nsui.Button{Text: "-", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.gridCols.Get() <= 2 },
							OnPress: func() { a.setGridColumns(cx, a.gridCols.Get()-1) }},
						nsui.Button{Text: "+", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.gridCols.Get() >= 5 },
							OnPress: func() { a.setGridColumns(cx, a.gridCols.Get()+1) }},
					},
				}

				items := []core.Node{
					row(lucide.RefreshCw, "刷新", "", false, func() {
						a.menuOpen.Set(false)
						a.refreshGalleryView(cx, true)
					}),
				}
				if a.sidebarView != SidebarTrash && a.sidebarView != SidebarHistory {
					items = append(items, row(lucide.SlidersHorizontal, "筛选", filterBadge, false, func() {
						a.menuOpen.Set(false)
						a.openFilterSheet("gallery")
					}))
				}
				items = append(items,
					row(lucide.Inbox, "收件箱", inboxBadge, false, func() {
						a.menuOpen.Set(false)
						a.inboxBackScreen = screenGallery
						a.screen.Set(screenInbox)
						a.consumeShares(cx)
					}),
					row(lucide.ArrowLeftRight, "切换资源库", a.libraryName(), false, func() {
						a.menuOpen.Set(false)
						a.openLibraries(cx)
					}),
					colRow,
				)
				if a.sidebarView == SidebarTrash && len(a.gallery.Snapshot().Items) > 0 {
					items = append(items, row(lucide.Trash, "清空废纸篓", "", true, func() {
						a.menuOpen.Set(false)
						a.emptyTrashOpen.Set(true)
					}))
				}
				items = append(items, row(lucide.LogOut, "断开连接", "", true, func() {
					a.menuOpen.Set(false)
					a.confirmDisc.Set(true)
				}))
				return core.Scroll{
					Style:    core.Style{FlexShrink: 1, MinHeight: core.Pt(0)},
					Children: []core.Node{core.View{Style: core.Style{Gap: 8}, Children: items}},
				}
			}),
		},
	})
}

func (a *App) disconnectDialog(cx *core.Scope) core.Node {
	return nsui.Dialog{
		Open:         func() bool { return a.confirmDisc.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.confirmDisc.Set(open) },
		Title:        "断开连接？",
		Description:  "将返回连接页，保留历史记录与收件箱中的待上传内容。",
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.confirmDisc.Set(false) }},
			nsui.Button{Text: "断开", OnPress: func() {
				a.confirmDisc.Set(false)
				a.disconnect(cx)
			}},
		},
	}
}

func (a *App) librariesDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.libsOpen.Get() },
		OnClose: func() { a.libsOpen.Set(false) },
		Title:   "切换资源库",
		Header: []core.Node{
			nsui.Button{Text: "刷新", Variant: nsui.ButtonGhost, OnPress: func() { a.openLibraries(cx) }},
		},
		Children: []core.Node{
			core.Text{
				Content: "切换会改变电脑当前活动库，当前列表、筛选和详情会重新加载。",
				MaxLines: 2,
				Style:   nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()}),
			},
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.libsOpen.Get()
				if len(a.libs) == 0 {
					msg := "没有可切换的资源库"
					if a.libsBusy.Get() {
						msg = "正在读取资源库…"
					}
					return core.Text{Content: msg, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted(), Padding: core.Symmetric(16, 4)})}
				}
				rows := make([]core.Node, 0, len(a.libs))
				for _, l := range a.libs {
					lib := l
					subtitle := lib.Path
					if lib.IsActive {
						subtitle = "当前活动库 · " + subtitle
					}
					rows = append(rows, core.View{
						Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.All(10),
							CornerRadius: 10, Background: colors.Inset(), BorderWidth: 1, BorderColor: colors.Border()},
						OnPress: func(core.PressEvent) {
							if !a.libsBusy.Get() {
								a.switchLibrary(cx, lib)
							}
						},
						Children: []core.Node{
							core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
								core.Text{Content: lib.Name + " · " + itoa(int(lib.ItemCount)) + " 项", MaxLines: 1,
									Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
								core.Text{Content: subtitle, MaxLines: 1, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
							}},
							core.Show{When: func() bool { return !lib.IsActive }, Then: func(cx *core.Scope) core.Node {
								return nsui.Button{Text: "切换", Variant: nsui.ButtonSecondary,
									Disabled: func() bool { return a.libsBusy.Get() },
									OnPress:  func() { a.switchLibrary(cx, lib) }}
							}},
						},
					})
				}
				return core.Scroll{
					Style:    core.Style{MaxHeight: core.Pt(360)},
					Children: []core.Node{core.View{Style: core.Style{Gap: 6}, Children: rows}},
				}
			}),
		},
	})
}
