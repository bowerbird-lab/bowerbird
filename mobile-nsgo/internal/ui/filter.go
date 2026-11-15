package ui

import (
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
)

func (a *App) filterSheet(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()

	fType := a.filterDraftType
	fOrient := a.filterDraftOrient
	fSize := a.filterDraftSize
	fRating := a.filterDraftRating
	fHasURL := a.filterDraftHasURL
	fHasNote := a.filterDraftHasNote
	fTag := a.filterDraftTag
	fColor := a.filterDraftColor
	fFolder := a.filterDraftFolder

	chipRow := func(title string, options [][2]string, current func() string, set func(string)) core.Node {
		btns := make([]core.Node, 0, len(options))
		for _, opt := range options {
			label, val := opt[0], opt[1]
			btns = append(btns, core.View{
				StyleFunc: func() core.Style {
					bg, border := colors.Surface(), colors.Border()
					if current() == val {
						bg, border = colors.AccentFill(), colors.Accent()
					}
					return core.Style{
						Padding: core.Symmetric(5, 11), CornerRadius: 999,
						Background: bg, BorderWidth: 1, BorderColor: border,
					}
				},
				OnPress: func(core.PressEvent) { set(val) },
				Children: []core.Node{core.Text{Content: label, StyleFunc: func() core.Style {
					c := colors.Text()
					if current() == val {
						c = colors.OnAccent()
					}
					return core.Style{FontSize: 12, LineHeight: 16, Color: c}
				}}},
			})
		}
		return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
			core.Text{Content: title, Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, Children: btns},
		}}
	}

	ratingRow := func() core.Node {
		opts := [][2]string{{"任意", "0"}, {"1★", "1"}, {"2★", "2"}, {"3★", "3"}, {"4★", "4"}, {"5★", "5"}}
		return chipRow("评分", opts, func() string { return strconv.Itoa(fRating.Get()) }, func(v string) {
			n, _ := strconv.Atoi(v)
			fRating.Set(n)
		})
	}

	dynamicBody := core.Dynamic(func(cx *core.Scope) core.Node {
		_ = a.rev.Get()
		_ = a.filterOpen.Get()
		tagOpts := [][2]string{{"全部", ""}}
		for i, t := range a.tags {
			if i >= 24 {
				break
			}
			tagOpts = append(tagOpts, [2]string{t.Name + " (" + itoa(t.ItemCount) + ")", t.Name})
		}

		colorNodes := []core.Node{
			core.View{
				StyleFunc: func() core.Style {
					bg, border := colors.Surface(), colors.Border()
					if fColor.Get() == "" {
						bg, border = colors.AccentFill(), colors.Accent()
					}
					return core.Style{Padding: core.Symmetric(4, 10), CornerRadius: 999, Background: bg, BorderWidth: 1, BorderColor: border}
				},
				OnPress: func(core.PressEvent) { fColor.Set("") },
				Children: []core.Node{core.Text{Content: "全部", StyleFunc: func() core.Style {
					c := colors.Text()
					if fColor.Get() == "" {
						c = colors.OnAccent()
					}
					return core.Style{FontSize: 12, LineHeight: 16, Color: c}
				}}},
			},
		}
		for _, hex := range a.colors {
			h := hex
			cVal, ok := parseHexColor(h)
			if !ok {
				continue
			}
			colorNodes = append(colorNodes, core.View{
				StyleFunc: func() core.Style {
					border := colors.Border()
					bw := core.Points(1)
					if strings.EqualFold(fColor.Get(), h) {
						border = colors.Accent()
						bw = 2.5
					}
					return core.Style{Width: core.Pt(28), Height: core.Pt(28), CornerRadius: 14, Background: cVal, BorderWidth: bw, BorderColor: border}
				},
				OnPress: func(core.PressEvent) {
					if strings.EqualFold(fColor.Get(), h) {
						fColor.Set("")
					} else {
						fColor.Set(h)
					}
				},
			})
		}

		folderOpts := [][2]string{{"全部", ""}}
		for _, f := range a.flattenAllFolders() {
			folderOpts = append(folderOpts, [2]string{f.Name, f.ID})
		}

		sections := []core.Node{
			chipRow("类型", [][2]string{{"全部", ""}, {"图片", "image"}, {"视频", "video"}, {"音频", "audio"}}, fType.Get, fType.Set),
			chipRow("方向", [][2]string{{"全部", ""}, {"横图", "landscape"}, {"竖图", "portrait"}, {"方形", "square"}}, fOrient.Get, fOrient.Set),
			chipRow("大小", [][2]string{{"任意", ""}, {"小 <1MB", "small"}, {"中 1-10MB", "medium"}, {"大 >10MB", "large"}}, fSize.Get, fSize.Set),
			ratingRow(),
			chipRow("链接", [][2]string{{"任意", ""}, {"有链接", "has"}, {"无链接", "empty"}}, fHasURL.Get, fHasURL.Set),
			chipRow("备注", [][2]string{{"任意", ""}, {"有备注", "has"}, {"无备注", "empty"}}, fHasNote.Get, fHasNote.Set),
		}
		if len(a.colors) > 0 {
			sections = append(sections, core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				core.Text{Content: "主色", Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
				core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8}, Children: colorNodes},
			}})
		}
		if len(tagOpts) > 1 {
			sections = append(sections, chipRow("标签", tagOpts, fTag.Get, fTag.Set))
		}
		if a.filterTarget == "search" && len(folderOpts) > 1 {
			sections = append(sections, chipRow("文件夹", folderOpts, fFolder.Get, fFolder.Set))
		}
		return core.View{Style: core.Style{Gap: 14, Padding: core.Edges{Bottom: 16}}, Children: sections}
	})

	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.filterOpen.Get() },
		OnClose: func() { a.filterOpen.Set(false) },
		Title:   "筛选条件",
		Header: []core.Node{
			nsui.Button{Text: "重置", Variant: nsui.ButtonGhost, OnPress: func() {
				a.resetFilterDraft()
			}},
			nsui.Button{Text: "应用", OnPress: func() {
				a.applyFilterDraft(cx)
				a.filterOpen.Set(false)
			}, TestID: "filter.apply"},
		},
		Children: []core.Node{
			core.Show{When: func() bool { return !a.client.QuerySupported() }, Then: func(cx *core.Scope) core.Node {
				return core.Text{
					Content:  "旧版桌面端不支持大小/评分/链接/备注筛选，选择这些条件时将提示升级。",
					MaxLines: 2,
					Style:    nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()}),
				}
			}},
			core.Scroll{
				Style:    core.Style{MaxHeight: core.Pt(420)},
				Children: []core.Node{dynamicBody},
			},
		},
	})
}

func (a *App) openFilterSheet(target string) {
	a.filterTarget = target
	var spec query.Spec
	if target == "search" {
		spec = a.searchSpec()
	} else {
		spec = a.galleryFilter
	}
	a.filterDraftType.Set(spec.Type)
	a.filterDraftOrient.Set(spec.Orientation)
	a.filterDraftSize.Set(spec.Size)
	a.filterDraftRating.Set(spec.Rating)
	a.filterDraftHasURL.Set(spec.HasURL)
	a.filterDraftHasNote.Set(spec.HasNote)
	a.filterDraftTag.Set(spec.Tag)
	a.filterDraftColor.Set(spec.Color)
	a.filterDraftFolder.Set(spec.Folder)
	a.filterOpen.Set(true)
}

func (a *App) resetFilterDraft() {
	a.filterDraftType.Set("")
	a.filterDraftOrient.Set("")
	a.filterDraftSize.Set("")
	a.filterDraftRating.Set(0)
	a.filterDraftHasURL.Set("")
	a.filterDraftHasNote.Set("")
	a.filterDraftTag.Set("")
	a.filterDraftColor.Set("")
	a.filterDraftFolder.Set("")
}

func (a *App) applyFilterDraft(cx *core.Scope) {
	draft := query.Spec{
		Type:        a.filterDraftType.Get(),
		Orientation: a.filterDraftOrient.Get(),
		Size:        a.filterDraftSize.Get(),
		Rating:      a.filterDraftRating.Get(),
		HasURL:      a.filterDraftHasURL.Get(),
		HasNote:     a.filterDraftHasNote.Get(),
		Tag:         a.filterDraftTag.Get(),
		Color:       a.filterDraftColor.Get(),
		Folder:      a.filterDraftFolder.Get(),
	}
	if a.filterTarget == "search" {
		a.searchType.Set(draft.Type)
		a.searchOrient.Set(draft.Orientation)
		a.searchExtra = draft
		a.applySearch(cx)
		return
	}
	draft.Folder = ""
	a.galleryFilter = draft
	a.bump()
	a.refreshGalleryView(cx, true)
}

func parseHexColor(hex string) (core.Color, bool) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, false
	}
	return core.RGB(uint32(v)), true
}
