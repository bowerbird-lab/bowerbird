package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsui "github.com/nsgo-dev/nsgo/ui"
)

func (a *App) menuDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	item := func(label, hint string, press func()) core.Node {
		return core.View{
			Style:   core.Style{Width: core.Pct(100), Padding: core.All(10), CornerRadius: 8, Background: colors.Surface(), Gap: 2},
			OnPress: func(core.PressEvent) { press() },
			Children: []core.Node{
				core.Text{Content: label, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
				core.Text{Content: hint, MaxLines: 2, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			},
		}
	}
	colRow := core.View{
		Style: core.Style{Width: core.Pct(100), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
			Padding: core.All(10), CornerRadius: 8, Background: colors.Surface()},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, Gap: 2}, Children: []core.Node{
				core.Text{Content: "网格列数", Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
				core.Text{ContentFunc: func() string { return itoa(a.gridCols.Get()) + " 列（支持双指捏合调整）" },
					Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			}},
			nsui.Button{Text: "-", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.gridCols.Get() <= 2 },
				OnPress: func() { a.setGridColumns(cx, a.gridCols.Get()-1) }},
			nsui.Button{Text: "+", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.gridCols.Get() >= 5 },
				OnPress: func() { a.setGridColumns(cx, a.gridCols.Get()+1) }},
		},
	}
	return nsui.Dialog{
		Open:         func() bool { return a.menuOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.menuOpen.Set(open) },
		Title:        "更多",
		Children: []core.Node{
			item("上传素材", "从手机选择图片或视频上传到当前资源库", func() {
				a.menuOpen.Set(false)
				a.pickAndUpload(cx)
			}),
			item("切换资源库", "切换会改变电脑当前活动库，并影响其他已连接设备", func() {
				a.menuOpen.Set(false)
				a.openLibraries(cx)
			}),
			item("收件箱", "查看从其他应用分享进来的待上传内容", func() {
				a.menuOpen.Set(false)
				a.inboxBackScreen = screenGallery
				a.screen.Set(screenInbox)
				a.consumeShares(cx)
			}),
			colRow,
			item("断开连接", "保留历史连接，下次可直接重连", func() {
				a.menuOpen.Set(false)
				a.confirmDisc.Set(true)
			}),
		},
		Footer: []core.Node{
			nsui.Button{Text: "关闭", Variant: nsui.ButtonGhost, OnPress: func() { a.menuOpen.Set(false) }},
		},
	}
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
	list := core.List{
		Count: func() int { a.rev.Get(); return len(a.libs) },
		Key:   func(i int) any { a.rev.Get(); return a.libs[i].ID + a.libs[i].Path },
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			a.rev.Get()
			i := row.Index()
			if i < 0 || i >= len(a.libs) {
				return core.View{}
			}
			lib := a.libs[i]
			subtitle := lib.Path
			if lib.IsActive {
				subtitle = "当前活动库 · " + subtitle
			}
			return core.View{
				Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.All(10),
					CornerRadius: 8, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()},
				Children: []core.Node{
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
						core.Text{Content: lib.Name + "  ·  " + itoa(int(lib.ItemCount)) + " 项", MaxLines: 1,
							Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
						core.Text{Content: subtitle, MaxLines: 1, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
					}},
					core.Show{When: func() bool { return !lib.IsActive }, Then: func(cx *core.Scope) core.Node {
						return nsui.Button{Text: "切换", Variant: nsui.ButtonSecondary,
							Disabled: func() bool { return a.libsBusy.Get() },
							OnPress:  func() { a.switchLibrary(cx, lib) }}
					}},
				},
			}
		},
		EstimatedSize: 64,
		Gap:           6,
		Style:         core.Style{MaxHeight: core.Pt(360)},
		Empty:         core.Text{Content: "没有可切换的资源库", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
	}
	children := []core.Node{list}
	if len(a.libs) == 0 && a.libsBusy.Get() {
		children = []core.Node{core.Text{Content: "正在读取资源库…", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})}}
	}
	return nsui.Dialog{
		Open:         func() bool { return a.libsOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.libsOpen.Set(open) },
		Title:        "切换资源库",
		Description:  "电脑端会立即切换活动库；当前列表、筛选和详情会失效。",
		Children:     children,
		Footer: []core.Node{
			nsui.Button{Text: "刷新", Variant: nsui.ButtonGhost, OnPress: func() { a.openLibraries(cx) }},
			nsui.Button{Text: "关闭", Variant: nsui.ButtonGhost, OnPress: func() { a.libsOpen.Set(false) }},
		},
	}
}
