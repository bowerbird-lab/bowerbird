package ui

import (
	"context"
	"strconv"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	appstate "github.com/owocc/bowerbird/mobile-nsgo/internal/app"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
)

func (a *App) openSearchPage() {
	a.detailFrom = screenGallery
	a.screen.Set(screenSearch)
	a.searchText.Set("")
	a.searchType.Set("")
	a.searchOrient.Set("")
	a.searchExtra = query.Spec{}
	a.searchGallery.Clear(query.Spec{}, model.SortDesc, model.DefaultPageSize)
	a.rebuildSearchRows()
	a.bump()
}

func (a *App) searchPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()
	searchField := core.Input{
		Value: func() string { return a.searchText.Get() }, OnChange: a.searchText.Set,
		Placeholder: "搜索名称、标签、备注…", OnSubmit: func(string) { a.applySearch(cx) },
		ReturnKeyType: core.ReturnKeySearch, TestID: "search.text",
		Style: core.Style{
			FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pt(42),
			CornerRadius: 21, Padding: core.Symmetric(0, 16),
			Background: colors.Field(), BorderWidth: 1, BorderColor: colors.FieldBorder(),
			FontSize: 14, LineHeight: 18, Color: colors.Text(),
			PlaceholderColor: colors.Faint(), CaretColor: colors.Accent(),
			SelectionColor: cx.Palette().Selection,
		},
	}
	header := a.appHeader(cx, appHeaderSpec{
		Floating: true, InsetSafeArea: true, OmitLeftSafeArea: func() bool { return true },
		TestID: "search.header",
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				appCircleButton(colors, "search.back", lucide.ChevronLeft, func(core.PressEvent) {
					a.detailFrom = screenGallery
					a.screen.Set(screenGallery)
				}),
				searchField,
			}},
		},
	})
	filterRow := core.Dynamic(func(cx *core.Scope) core.Node {
		_ = a.rev.Get()
		return a.searchFilterRow(cx)
	})
	var results core.Node
	if a.env.HasHost {
		results = a.nativeGridNode(cx, true)
	} else {
		results = core.List{
			Count: func() int { a.galleryRev.Get(); return len(a.cachedSearchRows) },
			Key: func(i int) any {
				rows := a.cachedSearchRows
				if i < 0 || i >= len(rows) {
					return i
				}
				return rows[i].Key
			},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				return a.galleryListRow(cx, true, row, screenSearch)
			},
			NumColumns:    1,
			Gap:           6,
			EstimatedSize: a.cachedTileSize + 6,
			OnLoadMore:    func() { a.loadSearchMore(cx) },
			Ref:           a.searchRef,
			TestID:        "search.list",
			Empty: core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)}, Children: []core.Node{
				core.Text{Content: "输入关键词，或选择筛选条件", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			}},
			Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		}
	}
	return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Children: []core.Node{
		core.View{StyleFunc: func() core.Style {
			safe := cx.Env().SafeArea()
			pad := safeSideInset(safe)
			return core.Style{
				Width: core.Pct(100), Height: core.Pct(100), Gap: 8,
				Padding: core.Edges{Top: safe.Top + 56, Bottom: safe.Bottom + 12, Left: pad, Right: pad},
			}
		}, Children: []core.Node{
			filterRow,
			core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, AlignItems: core.AlignCenter}, Children: []core.Node{
				core.Text{ContentFunc: func() string {
					a.rev.Get()
					return searchCount(a.searchGallery.Snapshot())
				}, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
				nsui.Button{Text: "重置", Variant: nsui.ButtonGhost, OnPress: func() {
					a.searchText.Set("")
					a.searchType.Set("")
					a.searchOrient.Set("")
					a.searchExtra = query.Spec{}
					a.applySearch(cx)
				}},
			}},
			a.errorBanner(cx, func() error { a.rev.Get(); return a.searchGallery.Snapshot().Err }),
			results,
		}},
		header,
		core.Show{When: func() bool { return a.screen.Get() == screenSearch }, Then: func(cx *core.Scope) core.Node {
			return a.filterSheet(cx)
		}},
	}}
}

// searchFilterRow is the compact filter entry point: one plus button, with
// the filters the user picked shown as chips beside it. The filter sheet holds
// every condition, so the search page no longer shows a chip for each value.
func (a *App) searchFilterRow(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	children := []core.Node{core.View{
		TestID: "search.filter", Label: "添加筛选条件",
		StyleFunc: func() core.Style {
			return core.Style{
				Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 16,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
			}
		},
		OnPress:  func(core.PressEvent) { a.openFilterSheet("search") },
		Children: []core.Node{nsui.Icon(lucide.Plus, 18, colors.Text)},
	}}
	for _, label := range a.searchFilterLabels() {
		label := label
		children = append(children, drawerChip(colors, label, func() bool { return true }, func() {
			a.openFilterSheet("search")
		}))
	}
	return core.View{Style: core.Style{Direction: core.Row, Gap: 6, Wrap: core.Wrap, AlignItems: core.AlignCenter}, Children: children}
}

func (a *App) searchFilterLabels() []string {
	spec := a.searchSpec()
	labels := make([]string, 0, 8)
	switch spec.Type {
	case "image":
		labels = append(labels, "图片")
	case "video":
		labels = append(labels, "视频")
	case "audio":
		labels = append(labels, "音频")
	}
	switch spec.Orientation {
	case "landscape":
		labels = append(labels, "横图")
	case "portrait":
		labels = append(labels, "竖图")
	case "square":
		labels = append(labels, "方形")
	}
	switch spec.Size {
	case "small":
		labels = append(labels, "小文件")
	case "medium":
		labels = append(labels, "中文件")
	case "large":
		labels = append(labels, "大文件")
	}
	if spec.Rating > 0 {
		labels = append(labels, strconv.Itoa(spec.Rating)+"★")
	}
	switch spec.HasURL {
	case "has":
		labels = append(labels, "有链接")
	case "empty":
		labels = append(labels, "无链接")
	}
	switch spec.HasNote {
	case "has":
		labels = append(labels, "有备注")
	case "empty":
		labels = append(labels, "无备注")
	}
	if spec.Tag != "" {
		labels = append(labels, "标签："+spec.Tag)
	}
	if spec.Folder != "" {
		name := spec.Folder
		for _, f := range a.flattenAllFolders() {
			if f.ID == spec.Folder {
				name = f.Name
				break
			}
		}
		labels = append(labels, "文件夹："+name)
	}
	if spec.Color != "" {
		labels = append(labels, "主色 "+spec.Color)
	}
	return labels
}

func chips(cx *core.Scope, values []string, selected func() string, set func(string)) []core.Node {
	colors := nsui.ColorsOf(cx)
	out := make([]core.Node, 0, len(values))
	for _, value := range values {
		v := value
		label := v
		switch v {
		case "image":
			label = "图片"
		case "video":
			label = "视频"
		case "audio":
			label = "音频"
		case "landscape":
			label = "横图"
		case "portrait":
			label = "竖图"
		case "square":
			label = "方形"
		}
		out = append(out, core.View{
			StyleFunc: func() core.Style {
				bg, border := colors.Surface(), colors.Border()
				if selected() == v {
					bg, border = colors.AccentFill(), colors.Accent()
				}
				return core.Style{Padding: core.Symmetric(4, 10), CornerRadius: 999, Background: bg,
					BorderWidth: 1, BorderColor: border}
			},
			Children: []core.Node{core.Text{Content: label, StyleFunc: func() core.Style {
				c := colors.Text()
				if selected() == v {
					c = colors.OnAccent()
				}
				return core.Style{FontSize: 12, LineHeight: 16, Color: c}
			}}},
			OnPress: func(core.PressEvent) { set(v) },
		})
	}
	return out
}

func searchCount(snap appstate.GallerySnapshot) string {
	if snap.Loading {
		return "查询中…"
	}
	if snap.LoadedOnce {
		q := snap.Spec.String()
		if q != "" {
			return itoa(len(snap.Items)) + " / " + itoa(snap.Total) + " 个结果"
		}
		return itoa(snap.Total) + " 个结果"
	}
	return ""
}

func (a *App) applySearch(cx *core.Scope) {
	spec := a.searchSpec()
	token := a.searchGallery.Reset(spec, model.SortDesc, model.DefaultPageSize)
	a.bump()
	a.loadSearch(cx, token, 0, false, true)
}

func (a *App) loadSearch(cx *core.Scope, token uint64, offset int, appendPage bool, jumpTop bool) {
	g := a.searchGallery
	spec := g.Spec()
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 25*time.Second)
		defer cancel()
		page, err := a.client.List(ctx, spec, model.SortDesc, offset, model.DefaultPageSize)
		a.post(func() {
			applied := g.Apply(token, page, appendPage, err)
			if applied {
				a.rebuildSearchRows()
			}
			a.bump()
			if applied && jumpTop && !appendPage {
				a.scrollTop(cx, a.searchRef)
			}
		})
	}()
}

func (a *App) loadSearchMore(cx *core.Scope) {
	token, spec, _, offset, limit, ok := a.searchGallery.Request(true)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 25*time.Second)
		defer cancel()
		page, err := a.client.List(ctx, spec, model.SortDesc, offset, limit)
		a.post(func() {
			if a.searchGallery.Apply(token, page, true, err) {
				a.rebuildSearchRows()
			}
			a.bump()
		})
	}()
}

func (a *App) currentGalleryFor(from string) *appstate.Gallery {
	if from == screenSearch {
		return a.searchGallery
	}
	return a.gallery
}
