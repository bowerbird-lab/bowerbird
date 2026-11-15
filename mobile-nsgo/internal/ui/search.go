package ui

import (
	"context"
	"time"

	"github.com/nsgo-dev/nsgo/core"
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
	a.searchGallery.Reset(query.Spec{}, model.SortDesc, model.DefaultPageSize)
	a.rebuildSearchRows()
	a.bump()
}

func (a *App) searchPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		nsui.Button{Text: "返回", Variant: nsui.ButtonGhost, OnPress: func() {
			a.detailFrom = screenGallery
			a.screen.Set(screenGallery)
		}},
		nsui.Input{Value: func() string { return a.searchText.Get() }, OnChange: a.searchText.Set,
			Placeholder: "搜索名称、标签、备注…", Style: core.Style{FlexGrow: 1}, OnSubmit: func(string) { a.applySearch(cx) }, TestID: "search.text"},
		nsui.Button{Text: "搜索", OnPress: func() { a.applySearch(cx) }, TestID: "search.submit"},
	}}
	types, orientations, _ := query.Dims()
	typeRow := core.View{Style: core.Style{Direction: core.Row, Gap: 6, Wrap: core.Wrap, AlignItems: core.AlignCenter}, Children: append(
		append([]core.Node{
			nsui.Button{Text: "更多筛选", Variant: nsui.ButtonSecondary, OnPress: func() { a.openFilterSheet("search") }, TestID: "search.filter"},
		}, chips(cx, types, func() string { return a.searchType.Get() }, func(v string) {
			if a.searchType.Get() == v {
				a.searchType.Set("")
			} else {
				a.searchType.Set(v)
			}
			a.applySearch(cx)
		})...),
		chips(cx, orientations, func() string { return a.searchOrient.Get() }, func(v string) {
			if a.searchOrient.Get() == v {
				a.searchOrient.Set("")
			} else {
				a.searchOrient.Set(v)
			}
			a.applySearch(cx)
		})...),
	}
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
			Empty: core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)}, Children: []core.Node{
				core.Text{Content: "输入关键词，或选择筛选条件", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			}},
			Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		}
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
		header, typeRow,
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Text{ContentFunc: func() string {
				a.rev.Get()
				return searchCount(a.searchGallery.Snapshot())
			}, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
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
		a.filterSheet(cx),
	}}
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
