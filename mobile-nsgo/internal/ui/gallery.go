package ui

import (
	"context"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	appstate "github.com/owocc/bowerbird/mobile-nsgo/internal/app"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

const maxAnimatedGIFBytes = 8 << 20 // 8 MiB

var drawerSlideTransition = core.Transition{
	Duration:   240 * time.Millisecond,
	Easing:     core.EaseOut,
	Properties: core.TransitionTransform | core.TransitionBorder | core.TransitionOpacity,
}

func (a *App) galleryPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	a.post(func() { a.ensureLoaded(cx) })

	// Floating simplified header (matches Flutter GalleryHeader + HeaderActionPill)
	floatingHeader := core.View{
		StyleFunc: func() core.Style {
			safe := cx.Env().SafeArea()
			return core.Style{
				Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
				Padding: core.Edges{
					Top:    safe.Top + 6,
					Bottom: 10,
					Left:   safe.Left + 12,
					Right:  safe.Right + 12,
				},
				Gradients: []core.Gradient{
					core.LinearGradient(180, colors.Background(), nsui.Alpha(colors.Background(), 0)),
				},
				ZIndex: 12,
			}
		},
		Children: []core.Node{
			core.Show{
				When: func() bool { return a.multiSelect.Get() },
				Then: func(cx *core.Scope) core.Node {
					return core.View{
						StyleFunc: func() core.Style {
							return core.Style{
								Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
								Padding: core.Symmetric(4, 8), CornerRadius: 22,
								Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
							}
						},
						Children: []core.Node{
							nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.exitMultiSelect() }},
							core.Text{ContentFunc: func() string { a.rev.Get(); return "已选 " + itoa(len(a.selectedIDs)) + " 项" },
								Style: nsui.TypeBody.Merge(core.Style{FlexGrow: 1}), StyleFunc: nsui.Colored(colors.Text)},
							nsui.Button{Text: "全选", Variant: nsui.ButtonGhost, OnPress: func() { a.selectAllVisible() }},
							nsui.Button{Text: "标签", Variant: nsui.ButtonGhost,
								Disabled: func() bool { a.rev.Get(); return len(a.selectedIDs) == 0 || a.batchBusy.Get() },
								OnPress:  func() { a.batchTagOpen.Set(true) }},
							nsui.Button{Text: "目录", Variant: nsui.ButtonGhost,
								Disabled: func() bool { a.rev.Get(); return len(a.selectedIDs) == 0 || a.batchBusy.Get() },
								OnPress:  func() { a.batchFolderOpen.Set(true) }},
							nsui.Button{Text: "删除", Variant: nsui.ButtonGhost,
								Disabled: func() bool { a.rev.Get(); return len(a.selectedIDs) == 0 || a.batchBusy.Get() },
								OnPress:  func() { a.batchDelOpen.Set(true) }},
						},
					}
				},
				Else: func(cx *core.Scope) core.Node {
					return core.View{
						Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
						Children: []core.Node{
							core.View{
								TestID: "gallery.drawer",
								StyleFunc: func() core.Style {
									return core.Style{
										Width: core.Pt(42), Height: core.Pt(42), CornerRadius: 21,
										AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
										Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
									}
								},
								OnPress: func(core.PressEvent) {
									next := !a.drawerOpen.Get()
									a.drawerOpen.Set(next)
									if next {
										a.loadSidebarData(cx)
									}
								},
								Children: []core.Node{nsui.Icon(lucide.Menu, 20, colors.Text)},
							},
							core.Show{
								When: func() bool { a.rev.Get(); return a.sidebarView != SidebarAll },
								Then: func(cx *core.Scope) core.Node {
									return core.View{
										StyleFunc: func() core.Style {
											return core.Style{
												Padding: core.Symmetric(6, 12), CornerRadius: 16,
												Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
											}
										},
										OnPress: func(core.PressEvent) {
											a.drawerOpen.Set(true)
											a.loadSidebarData(cx)
										},
										Children: []core.Node{
											core.Text{ContentFunc: func() string { a.rev.Get(); return a.currentViewTitle() },
												MaxLines: 1, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Text)},
										},
									}
								},
							},
							core.View{Style: core.Style{FlexGrow: 1}},
							core.View{
								StyleFunc: func() core.Style {
									return core.Style{
										Direction: core.Row, AlignItems: core.AlignCenter,
										Height: core.Pt(42), CornerRadius: 21, Overflow: core.Hidden,
										Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
									}
								},
								Children: []core.Node{
									core.View{
										TestID: "gallery.search",
										Style: core.Style{
											Width: core.Pt(44), Height: core.Pt(42),
											AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
										},
										OnPress:  func(core.PressEvent) { a.openSearchPage() },
										Children: []core.Node{nsui.Icon(lucide.Search, 19, colors.Text)},
									},
									core.View{
										TestID: "gallery.more",
										Style: core.Style{
											Width: core.Pt(44), Height: core.Pt(42),
											AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
										},
										OnPress: func(core.PressEvent) { a.menuOpen.Set(true) },
										Children: []core.Node{
											nsui.Icon(lucide.Ellipsis, 19, colors.Text),
											core.Show{
												When: func() bool {
													a.rev.Get()
													return len(a.inboxEntries) > 0 || a.galleryFilter.String() != ""
												},
												Then: func(cx *core.Scope) core.Node {
													return core.View{
														Style: core.Style{
															Position: core.Absolute, Top: core.Pt(9), Right: core.Pt(9),
															Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 4,
															Background: colors.AccentFill(),
														},
													}
												},
											},
										},
									},
								},
							},
						},
					}
				},
			},
		},
	}

	bannerStack := core.View{
		StyleFunc: func() core.Style {
			safe := cx.Env().SafeArea()
			return core.Style{
				Position: core.Absolute, Top: core.Pt(safe.Top + 56), Left: core.Pt(12), Right: core.Pt(12),
				Gap: 6, ZIndex: 11,
			}
		},
		Children: []core.Node{
			core.Show{
				When: func() bool { a.rev.Get(); return a.sidebarView == SidebarTrash && !a.multiSelect.Get() },
				Then: func(cx *core.Scope) core.Node {
					return core.View{
						StyleFunc: func() core.Style {
							return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(6, 10),
								CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
						},
						Children: []core.Node{
							core.Text{Content: "废纸篓：点按素材可恢复", Style: nsui.TypeCaption.Merge(core.Style{FlexGrow: 1}), StyleFunc: nsui.Colored(colors.Muted)},
							nsui.Button{Text: "清空", Variant: nsui.ButtonGhost,
								Disabled: func() bool { a.rev.Get(); return len(a.gallery.Snapshot().Items) == 0 },
								OnPress:  func() { a.emptyTrashOpen.Set(true) }},
						},
					}
				},
			},
			core.Show{
				When: func() bool { return a.uploadStatus.Get() != "" },
				Then: func(cx *core.Scope) core.Node {
					return core.View{
						StyleFunc: func() core.Style {
							return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(6, 10),
								CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
						},
						Children: []core.Node{
							core.Text{ContentFunc: a.uploadStatus.Get, Style: nsui.TypeCaption.Merge(core.Style{FlexGrow: 1}), StyleFunc: nsui.Colored(colors.Text)},
							nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.cancelActiveUpload() }},
						},
					}
				},
			},
			a.errorBanner(cx, func() error { a.rev.Get(); return a.gallery.Snapshot().Err }),
		},
	}

	var gridNode core.Node
	if a.env.HasHost {
		gridNode = a.nativeGridNode(cx, false)
	} else {
		bodyList := core.List{
			Count: func() int { a.galleryRev.Get(); return len(a.cachedRows) },
			Key: func(i int) any {
				r := a.cachedRows
				if i < 0 || i >= len(r) {
					return i
				}
				return r[i].Key
			},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				return a.galleryListRow(cx, false, row, screenGallery)
			},
			NumColumns:    1,
			Gap:           6,
			EstimatedSize: a.cachedTileSize + 6,
			OnLoadMore:    func() { a.loadMoreGalleryView(cx) },
			OnRefresh:     func() { a.refreshGalleryView(cx, true) },
			Refreshing:    func() bool { a.rev.Get(); return a.gallery.Snapshot().Loading },
			Ref:           a.galleryRef,
			Footer: core.Show{When: func() bool { a.rev.Get(); return a.gallery.Snapshot().LoadingMore }, Then: func(cx *core.Scope) core.Node {
				return core.Text{Content: "加载更多…", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted(), TextAlign: core.TextAlignCenter, Padding: core.All(12)})}
			}},
			Empty: a.emptyState(cx),
			Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Padding: core.Edges{Top: 56, Left: 12, Right: 12}},
		}
		gridNode = core.View{
			Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
			Children: []core.Node{bodyList},
			OnPinch: func(ev core.GestureEvent) {
				a.handlePinchDensity(cx, ev)
			},
		}
	}

	uploadFAB := core.Show{
		When: func() bool {
			a.rev.Get()
			return !a.multiSelect.Get() && a.sidebarView != SidebarTrash
		},
		Then: func(cx *core.Scope) core.Node {
			return core.View{
				TestID: "gallery.upload",
				StyleFunc: func() core.Style {
					safe := cx.Env().SafeArea()
					return core.Style{
						Position: core.Absolute, Right: core.Pt(safe.Right + 16), Bottom: core.Pt(safe.Bottom + 18),
						Width: core.Pt(52), Height: core.Pt(52), CornerRadius: 16,
						AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
						Background: colors.AccentFill(), ZIndex: 12,
					}
				},
				OnPress:  func(core.PressEvent) { a.pickAndUpload(cx) },
				Children: []core.Node{nsui.Icon(lucide.ImagePlus, 22, colors.OnAccent)},
			}
		},
	}

	scrim := core.Show{
		When: func() bool { return a.drawerOpen.Get() },
		Then: func(cx *core.Scope) core.Node {
			return core.View{
				Style: core.Style{
					Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
					Background: core.RGBA(0x000000, 0.28), ZIndex: 20,
				},
				OnPress:       func(core.PressEvent) { a.drawerOpen.Set(false) },
				PanActivation: core.PanActivation{ActiveOffsetX: 14, FailOffsetY: 16},
				OnPan: func(ev core.PanEvent) {
					if ev.Phase == core.GestureEnded && (ev.Translation.X < -30 || ev.Velocity.X < -250) {
						a.drawerOpen.Set(false)
					}
				},
			}
		},
	}

	mainShift := core.View{
		StyleFunc: func() core.Style {
			var tx core.Points
			var rad core.Points
			if a.drawerOpen.Get() {
				tx = a.drawerWidth(cx)
				rad = 28
			}
			return core.Style{
				Width: core.Pct(100), Height: core.Pct(100),
				Background: colors.Background(), Overflow: core.Hidden,
				CornerRadii: core.Corners{TopLeft: rad, BottomLeft: rad},
				Transform:   core.Transform{TranslateX: tx},
				Transition:  drawerSlideTransition,
				ZIndex:      15,
			}
		},
		Children: []core.Node{
			gridNode,
			floatingHeader,
			bannerStack,
			uploadFAB,
			scrim,
		},
	}

	return core.View{
		StyleFunc: func() core.Style {
			return core.Style{Width: core.Pct(100), Height: core.Pct(100), Overflow: core.Hidden, Background: colors.Background()}
		},
		Children: []core.Node{
			a.leftDrawerPanel(cx),
			mainShift,
			a.filterSheet(cx),
			a.menuDialog(cx),
			a.disconnectDialog(cx),
			a.librariesDialog(cx),
			a.quickPreviewSheet(cx),
			a.restoreDialog(cx),
			a.emptyTrashDialog(cx),
			a.batchTagDialog(cx),
			a.batchFolderDialog(cx),
			a.batchDeleteDialog(cx),
		},
	}
}

func (a *App) galleryRows() []GalleryRow {
	return a.cachedRows
}

func (a *App) nativeGridNode(cx *core.Scope, isSearch bool) core.Node {
	return core.Native{
		Component: "gallery_grid",
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		PropsFunc: func() map[string]any {
			a.galleryRev.Get()
			g := a.gallery
			from := screenGallery
			scrollSeq := a.galleryScrollSeq.Get()
			if isSearch {
				g = a.searchGallery
				from = screenSearch
				scrollSeq = a.searchScrollSeq.Get()
			}
			snap := g.Snapshot()
			monthGrouped := !isSearch && a.sidebarView != SidebarTrash && a.sidebarView != SidebarHistory
			entries := a.buildNativeGridEntries(snap.Items, monthGrouped, from)
			emptyText := ""
			if len(snap.Items) == 0 {
				if isSearch {
					if snap.Loading {
						emptyText = "查询中…"
					} else if snap.LoadedOnce {
						emptyText = "没有匹配的素材"
					} else {
						emptyText = "输入关键词，或选择筛选条件"
					}
				} else {
					emptyText = a.galleryEmptyMessage(snap)
				}
			}
			safe := cx.Env().SafeArea()
			topDp := int(safe.Top) + 56
			bottomDp := int(safe.Bottom) + 84
			if isSearch {
				topDp = 4
				bottomDp = 24
			}
			return map[string]any{
				"cols":          a.gridCols.Get(),
				"isDark":        cx.Env().Dark(),
				"refreshing":    snap.Loading,
				"emptyText":     emptyText,
				"scrollTopSeq":  scrollSeq,
				"topInsetDp":    topDp,
				"bottomInsetDp": bottomDp,
				"rows":          entries,
			}
		},
		OnEvent: func(name string, data any) {
			m, _ := data.(map[string]any)
			switch name {
			case "refresh":
				if isSearch {
					a.applySearch(cx)
				} else {
					a.refreshGalleryView(cx, false)
				}
			case "loadMore":
				if isSearch {
					a.loadSearchMore(cx)
				} else {
					a.loadMoreGalleryView(cx)
				}
			case "pinchCols":
				if cols := nativeInt(m, "cols"); cols >= 2 && cols <= 5 && cols != a.gridCols.Get() {
					a.gridCols.Set(cols)
					a.rebuildGalleryRows()
					a.rebuildSearchRows()
					a.bump()
				}
			case "press":
				idx := nativeInt(m, "index")
				id := nativeStr(m, "id")
				if !isSearch && a.sidebarView == SidebarTrash {
					for _, it := range a.gallery.Snapshot().Items {
						if it.ID == id {
							a.restoreTarget = it
							a.restoreOpen.Set(true)
							a.bump()
							return
						}
					}
					return
				}
				if !isSearch && a.multiSelect.Get() {
					a.toggleSelectItem(id)
					return
				}
				from := screenGallery
				if isSearch {
					from = screenSearch
				}
				a.openDetail(cx, idx, from)
			case "longPress":
				if isSearch || a.sidebarView == SidebarTrash || a.sidebarView == SidebarHistory {
					return
				}
				idx := nativeInt(m, "index")
				id := nativeStr(m, "id")
				if a.multiSelect.Get() {
					a.toggleSelectItem(id)
					return
				}
				items := a.gallery.Snapshot().Items
				if idx >= 0 && idx < len(items) {
					a.openQuickPreview(items[idx], idx)
				}
			}
		},
	}
}

func (a *App) buildNativeGridEntries(items []model.ItemSummary, monthGrouped bool, from string) []map[string]any {
	out := make([]map[string]any, 0, len(items)+8)
	lastMonth := ""
	monthCountIdx := -1
	headerSeq := 0
	multi := from == screenGallery && a.multiSelect.Get()
	for idx, item := range items {
		if monthGrouped {
			m := monthLabel(item.BestDate())
			if m != lastMonth {
				lastMonth = m
				headerSeq++
				monthCountIdx = len(out)
				out = append(out, map[string]any{
					"kind":  "header",
					"key":   "h:" + itoa(headerSeq) + ":" + m,
					"title": m,
					"count": 0,
				})
			}
			if monthCountIdx >= 0 {
				c, _ := out[monthCountIdx]["count"].(int)
				out[monthCountIdx]["count"] = c + 1
			}
		}
		thumb := a.client.MediaURL(item.ThumbnailURL)
		if thumb == "" {
			thumb = a.client.ThumbURL(item.ID)
		}
		badge := ""
		switch {
		case item.IsVideo():
			badge = "视频"
		case item.IsGIF():
			badge = "GIF"
		case item.IsAudio():
			badge = "音频"
		}
		out = append(out, map[string]any{
			"kind":        "item",
			"key":         item.ID,
			"flatIndex":   idx,
			"id":          item.ID,
			"thumbUrl":    thumb,
			"badge":       badge,
			"shareMarked": a.isItemShareMarked(item.ID),
			"selected":    multi && a.selectedIDs[item.ID],
		})
	}
	return out
}

func (a *App) galleryEmptyMessage(snap appstate.GallerySnapshot) string {
	switch {
	case snap.Loading:
		return "正在加载…"
	case snap.Err != nil:
		return "加载失败"
	case a.sidebarView == SidebarTrash:
		return "废纸篓是空的"
	case a.sidebarView == SidebarUntagged:
		return "没有未打标签的素材"
	case a.sidebarView == SidebarUncategorized:
		return "没有未分类的素材"
	case a.sidebarView == SidebarHistory:
		return "还没有使用记录，下载、分享或复制素材后会出现在这里"
	case a.sidebarView == SidebarFolder:
		return "这个文件夹是空的"
	default:
		return "还没有素材"
	}
}

func nativeInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func nativeStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func (a *App) galleryListRow(cx *core.Scope, isSearch bool, row core.ListRow, from string) core.Node {
	colors := nsui.ColorsOf(cx)
	rowAt := func() (GalleryRow, bool) {
		a.galleryRev.Get()
		rows := a.cachedRows
		if isSearch {
			rows = a.cachedSearchRows
		}
		i := row.Index()
		if i < 0 || i >= len(rows) {
			return GalleryRow{}, false
		}
		return rows[i], true
	}

	isHeader := func() bool {
		r, ok := rowAt()
		return ok && r.Kind == RowMonthHeader
	}

	headerNode := func(cx *core.Scope) core.Node {
		return core.View{
			Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifySpaceBetween,
				Padding: core.Edges{Top: 10, Bottom: 4, Left: 2, Right: 2}},
			Children: []core.Node{
				core.Text{ContentFunc: func() string {
					r, _ := rowAt()
					return r.Title
				}, Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Text()})},
				core.Text{ContentFunc: func() string {
					r, _ := rowAt()
					return itoa(r.Count) + " 项"
				}, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			},
		}
	}

	return core.View{
		Children: []core.Node{
			core.Show{When: isHeader, Then: headerNode, Else: func(cx *core.Scope) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					cols := a.gridCols.Get()
					slots := make([]core.Node, 0, cols)
					for slotIdx := 0; slotIdx < cols; slotIdx++ {
						si := slotIdx
						slotAt := func() (TileSlot, bool) {
							r, ok := rowAt()
							if !ok || r.Kind != RowTileGroup || si >= len(r.Slots) {
								return TileSlot{}, false
							}
							return r.Slots[si], true
						}
						slots = append(slots, a.slotTile(cx, slotAt, from))
					}
					return core.View{
						Style:    core.Style{Direction: core.Row, Gap: 6},
						Children: slots,
					}
				})
			}},
		},
	}
}

func (a *App) slotTile(cx *core.Scope, slotAt func() (TileSlot, bool), from string) core.Node {
	colors := nsui.ColorsOf(cx)
	hasItem := func() bool { _, ok := slotAt(); return ok }

	source := func() core.ImageSource {
		slot, ok := slotAt()
		if !ok {
			return core.Src("")
		}
		item := slot.Item
		url := a.client.MediaURL(item.ThumbnailURL)
		if url == "" {
			url = a.client.ThumbURL(item.ID)
		}
		return core.Src(url)
	}

	badgeText := func() string {
		slot, ok := slotAt()
		if !ok {
			return ""
		}
		switch {
		case slot.Item.IsVideo():
			return "视频"
		case slot.Item.IsGIF():
			return "GIF"
		case slot.Item.IsAudio():
			return "音频"
		default:
			return ""
		}
	}

	isShareMarked := func() bool {
		slot, ok := slotAt()
		if !ok {
			return false
		}
		return a.isItemShareMarked(slot.Item.ID)
	}

	isSelected := func() bool {
		slot, ok := slotAt()
		if !ok || !a.multiSelect.Get() {
			return false
		}
		return a.selectedIDs[slot.Item.ID]
	}

	return core.View{
		StyleFunc: func() core.Style {
			a.galleryRev.Get()
			size := a.cachedTileSize
			st := core.Style{
				Width: core.Pt(size), Height: core.Pt(size),
				CornerRadius: 8, Overflow: core.Hidden,
			}
			if hasItem() {
				st.Background = colors.Inset()
				st.BorderWidth = 1
				st.BorderColor = colors.Border()
			}
			return st
		},
		Children: []core.Node{
			core.Show{When: hasItem, Then: func(cx *core.Scope) core.Node {
				return core.View{
					TestID: "gallery.tile",
					Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100)},
					Children: []core.Node{
						core.Image{SourceFunc: source, ResizeMode: core.Cover, FadeDuration: time.Millisecond,
							Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}},
						core.Show{When: func() bool { return badgeText() != "" }, Then: func(cx *core.Scope) core.Node {
							return core.View{
								Style: core.Style{Position: core.Absolute, Right: core.Pt(5), Top: core.Pt(5),
									Padding: core.Symmetric(2, 5), CornerRadius: 4, Background: core.RGBA(0x000000, 0.62)},
								Children: []core.Node{core.Text{ContentFunc: badgeText,
									Style: core.Style{FontSize: 10, LineHeight: 13, Color: core.RGBA(0xffffff, 0.95)}}},
							}
						}},
						core.Show{When: isShareMarked, Then: func(cx *core.Scope) core.Node {
							return core.View{
								Style: core.Style{Position: core.Absolute, Left: core.Pt(5), Bottom: core.Pt(5),
									Padding: core.Symmetric(2, 5), CornerRadius: 4, Background: core.RGBA(0x16a34a, 0.85)},
								Children: []core.Node{core.Text{Content: "分享",
									Style: core.Style{FontSize: 9, LineHeight: 12, Color: core.RGBA(0xffffff, 0.98)}}},
							}
						}},
						core.Show{When: isSelected, Then: func(cx *core.Scope) core.Node {
							return core.View{
								Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
									Background: core.RGBA(0x000000, 0.36), AlignItems: core.AlignEnd, JustifyContent: core.JustifyStart, Padding: core.All(5)},
								Children: []core.Node{
									core.View{
										Style: core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10,
											Background: colors.AccentFill(), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
										Children: []core.Node{nsui.Icon(lucide.Check, 13, colors.OnAccent)},
									},
								},
							}
						}},
					},
				}
			}},
		},
		OnPress: func(core.PressEvent) {
			slot, ok := slotAt()
			if !ok {
				return
			}
			if from == screenGallery && a.sidebarView == SidebarTrash {
				a.restoreTarget = slot.Item
				a.restoreOpen.Set(true)
				a.bump()
				return
			}
			if a.multiSelect.Get() && from == screenGallery {
				a.toggleSelectItem(slot.Item.ID)
				return
			}
			a.openDetail(cx, slot.FlatIndex, from)
		},
		OnLongPress: func(core.PressEvent) {
			slot, ok := slotAt()
			if !ok || from != screenGallery || a.sidebarView == SidebarTrash || a.sidebarView == SidebarHistory {
				return
			}
			if !a.multiSelect.Get() {
				a.openQuickPreview(slot.Item, slot.FlatIndex)
			} else {
				a.toggleSelectItem(slot.Item.ID)
			}
		},
	}
}

func (a *App) handlePinchDensity(cx *core.Scope, ev core.GestureEvent) {
	switch ev.Phase {
	case core.GestureBegan:
		a.pinchBaseCols = a.gridCols.Get()
		a.pinchAppliedCols = a.pinchBaseCols
	case core.GestureChanged:
		if a.pinchBaseCols == 0 {
			a.pinchBaseCols = a.gridCols.Get()
		}
		delta := 0
		if ev.Value > 1.22 {
			delta = -1
		} else if ev.Value < 0.80 {
			delta = 1
		}
		if delta != 0 {
			next := a.pinchBaseCols + delta
			if next < 2 {
				next = 2
			}
			if next > 5 {
				next = 5
			}
			if next != a.pinchAppliedCols {
				a.pinchAppliedCols = next
				a.setGridColumns(cx, next)
			}
		}
	case core.GestureEnded, core.GestureCancelled:
		a.pinchBaseCols = 0
	}
}

func (a *App) setGridColumns(cx *core.Scope, next int) {
	if next < 2 {
		next = 2
	}
	if next > 5 {
		next = 5
	}
	if next == a.gridCols.Get() {
		return
	}
	// Preserve anchor item across density change.
	var anchorID string
	var anchorOffset core.Points
	if pos, ok := a.galleryRef.ListPosition(); ok {
		anchorID = FirstItemIDAtRow(a.galleryRows(), pos.Index)
		anchorOffset = pos.Offset
	}
	a.gridCols.Set(next)
	a.rebuildGalleryRows()
	a.rebuildSearchRows()
	a.bump()
	if anchorID != "" {
		rowIdx := RowIndexForItemID(a.cachedRows, anchorID)
		a.after(16*time.Millisecond, func() {
			a.galleryRef.ScrollToIndexAt(rowIdx, 0, -anchorOffset, false)
		})
	}
}

func (a *App) quickPreviewSheet(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.quickOpen.Get() },
		OnClose: func() { a.quickOpen.Set(false) },
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.quickOpen.Get()
				_ = cx.Env().Dark()
				item := a.quickItem
				itemIdx := a.quickIndex
				thumbURL := ""
				if item.ID != "" {
					thumbURL = a.client.MediaURL(item.ThumbnailURL)
					if thumbURL == "" {
						thumbURL = a.client.ThumbURL(item.ID)
					}
				}
				actionBtn := func(label string, onPress func()) core.Node {
					return core.View{
						Style:   core.Style{Width: core.Pct(100), Padding: core.Symmetric(11, 12), CornerRadius: 8, Background: colors.Surface()},
						OnPress: func(core.PressEvent) { onPress() },
						Children: []core.Node{core.Text{Content: label, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})}},
					}
				}
				return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
						core.Image{Source: core.Src(thumbURL), ResizeMode: core.Cover,
							Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 8, Background: colors.Inset()}},
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
							core.Text{Content: item.DisplayName(), MaxLines: 1, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
							core.Text{Content: model.FormatSize(item.Size), Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
						}},
					}},
					actionBtn("查看详情", func() {
						a.quickOpen.Set(false)
						a.openDetail(cx, itemIdx, screenGallery)
					}),
					actionBtn("多选此项", func() {
						a.quickOpen.Set(false)
						a.enterMultiSelect(item.ID)
					}),
					actionBtn("复制素材内容", func() {
						a.quickOpen.Set(false)
						a.copyItemContent(cx, item)
					}),
					actionBtn("下载原文件", func() {
						a.quickOpen.Set(false)
						a.exportItem(cx, item, false)
					}),
					actionBtn("分享", func() {
						a.quickOpen.Set(false)
						a.exportItem(cx, item, true)
					}),
					actionBtn("删除（移到废纸篓）", func() {
						a.quickOpen.Set(false)
						a.quickDelTarget = item
						a.confirmDel.Set(true)
						a.bump()
					}),
				}}
			}),
		},
	})
}

func (a *App) openQuickPreview(item model.ItemSummary, flatIndex int) {
	a.quickItem = item
	a.quickIndex = flatIndex
	a.quickOpen.Set(true)
	a.bump()
}

func (a *App) enterMultiSelect(firstID string) {
	a.multiSelect.Set(true)
	if firstID != "" {
		a.selectedIDs[firstID] = true
	}
	a.bumpGallery()
	a.bump()
}

func (a *App) exitMultiSelect() {
	a.multiSelect.Set(false)
	clear(a.selectedIDs)
	a.bumpGallery()
	a.bump()
}

func (a *App) toggleSelectItem(id string) {
	if a.selectedIDs[id] {
		delete(a.selectedIDs, id)
	} else {
		a.selectedIDs[id] = true
	}
	a.bumpGallery()
	a.bump()
}

func (a *App) selectAllVisible() {
	for _, item := range a.gallery.Snapshot().Items {
		a.selectedIDs[item.ID] = true
	}
	a.bumpGallery()
	a.bump()
}

func (a *App) selectedIDSlice() []string {
	out := make([]string, 0, len(a.selectedIDs))
	for _, item := range a.gallery.Snapshot().Items {
		if a.selectedIDs[item.ID] {
			out = append(out, item.ID)
		}
	}
	return out
}

func (a *App) batchTagDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.batchTagOpen.Get() },
		OnClose: func() { a.batchTagOpen.Set(false) },
		Title:   "批量添加标签",
		Header: []core.Node{
			nsui.Button{Text: "添加", Disabled: func() bool { return a.batchBusy.Get() || strings.TrimSpace(a.batchTagInput.Get()) == "" },
				OnPress: func() { a.runBatchAddTag(cx) }},
		},
		Children: []core.Node{
			nsui.Input{Value: func() string { return a.batchTagInput.Get() }, OnChange: a.batchTagInput.Set,
				Placeholder: "输入或点选下方标签", TestID: "batch.tagInput"},
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.batchTagOpen.Get()
				_ = cx.Env().Dark()
				tagChips := make([]core.Node, 0, len(a.tags))
				for i, t := range a.tags {
					if i >= 30 {
						break
					}
					tagName := t.Name
					tagChips = append(tagChips, core.View{
						Style: core.Style{Padding: core.Symmetric(5, 11), CornerRadius: 999,
							Background: colors.Inset(), BorderWidth: 1, BorderColor: colors.Border()},
						OnPress:  func(core.PressEvent) { a.batchTagInput.Set(tagName) },
						Children: []core.Node{core.Text{Content: tagName, Style: core.Style{FontSize: 12, LineHeight: 16, Color: colors.Text()}}},
					})
				}
				if len(tagChips) == 0 {
					return core.View{}
				}
				return core.Scroll{
					Style:    core.Style{MaxHeight: core.Pt(240)},
					Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, Children: tagChips}},
				}
			}),
		},
	})
}

func (a *App) batchFolderDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.batchFolderOpen.Get() },
		OnClose: func() { a.batchFolderOpen.Set(false) },
		Title:   "批量加入文件夹",
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.batchFolderOpen.Get()
				_ = cx.Env().Dark()
				flat := a.flattenAllFolders()
				rows := make([]core.Node, 0, len(flat))
				for _, f := range flat {
					folder := f
					rows = append(rows, core.View{
						Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(10, 12),
							CornerRadius: 10, Background: colors.Inset(), BorderWidth: 1, BorderColor: colors.Border()},
						OnPress: func(core.PressEvent) {
							a.batchFolderOpen.Set(false)
							a.runBatchAddFolder(cx, folder)
						},
						Children: []core.Node{
							core.Icon{SVG: lucide.Folder(), Style: core.Style{FontSize: 16, Color: colors.Muted()}},
							core.Text{Content: folder.Name, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text(), FlexGrow: 1})},
							core.Text{Content: itoa(folder.ItemCount), Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
						},
					})
				}
				return core.Scroll{
					Style: core.Style{MaxHeight: core.Pt(320)},
					Children: []core.Node{
						core.Show{When: func() bool { return len(rows) == 0 }, Then: func(cx *core.Scope) core.Node {
							return core.Text{Content: "当前资源库还没有文件夹", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})}
						}},
						core.View{Style: core.Style{Gap: 6}, Children: rows},
					},
				}
			}),
		},
	})
}

func (a *App) batchDeleteDialog(cx *core.Scope) core.Node {
	return nsui.Dialog{
		Open:         func() bool { return a.batchDelOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.batchDelOpen.Set(open) },
		Title:        "删除已选素材？",
		Description:  "将把已选的 " + itoa(len(a.selectedIDs)) + " 项移到桌面端废纸篓。",
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.batchDelOpen.Set(false) }},
			nsui.Button{Text: "移到废纸篓", OnPress: func() {
				a.batchDelOpen.Set(false)
				a.runBatchDelete(cx)
			}},
		},
	}
}

func (a *App) restoreDialog(cx *core.Scope) core.Node {
	return nsui.Dialog{
		Open:         func() bool { return a.restoreOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.restoreOpen.Set(open) },
		Title:        "恢复素材？",
		Description:  "将「" + a.restoreTarget.DisplayName() + "」从废纸篓恢复回原位置。",
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.restoreOpen.Set(false) }},
			nsui.Button{Text: "恢复", OnPress: func() {
				a.restoreOpen.Set(false)
				a.restoreItem(cx, a.restoreTarget)
			}},
		},
	}
}

func (a *App) emptyTrashDialog(cx *core.Scope) core.Node {
	return nsui.Dialog{
		Open:         func() bool { return a.emptyTrashOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.emptyTrashOpen.Set(open) },
		Title:        "清空废纸篓？",
		Description:  "废纸篓中的素材将被永久删除，无法恢复！",
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.emptyTrashOpen.Set(false) }},
			nsui.Button{Text: "永久清空", OnPress: func() {
				a.emptyTrashOpen.Set(false)
				a.emptyTrash(cx)
			}},
		},
	}
}

func (a *App) emptyState(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		_ = a.rev.Get()
		snap := a.gallery.Snapshot()
		text := "还没有素材"
		switch {
		case snap.Loading:
			text = "正在加载…"
		case snap.Err != nil:
			text = "加载失败"
		case a.sidebarView == SidebarTrash:
			text = "废纸篓是空的"
		case a.sidebarView == SidebarUntagged:
			text = "没有未打标签的素材"
		case a.sidebarView == SidebarUncategorized:
			text = "没有未分类的素材"
		case a.sidebarView == SidebarHistory:
			text = "还没有使用记录，下载、分享或复制素材后会出现在这里"
		case a.sidebarView == SidebarFolder:
			text = "这个文件夹是空的"
		}
		return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 8, Padding: core.All(24)}, Children: []core.Node{
			core.Text{Content: text, MaxLines: 3, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Muted(), TextAlign: core.TextAlignCenter})},
			core.Show{When: func() bool { return snap.Err != nil }, Then: func(cx *core.Scope) core.Node {
				return nsui.Button{Text: "重试", Variant: nsui.ButtonSecondary, OnPress: func() { a.refreshGalleryView(cx, true) }}
			}},
		}}
	})
}

func (a *App) errorBanner(cx *core.Scope, errFn func() error) core.Node {
	colors := nsui.ColorsOf(cx)
	return core.Show{When: func() bool { return errFn() != nil }, Then: func(cx *core.Scope) core.Node {
		return core.View{
			StyleFunc: func() core.Style {
				return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
					Padding: core.All(8), CornerRadius: 8, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Danger()}
			},
			Children: []core.Node{
				core.Text{ContentFunc: func() string {
					if e := errFn(); e != nil {
						return friendlyError(e)
					}
					return ""
				}, MaxLines: 3, Style: nsui.TypeCaption.Merge(core.Style{FlexGrow: 1}), StyleFunc: nsui.Colored(colors.Danger)},
				nsui.Button{Text: "重试", Variant: nsui.ButtonGhost, OnPress: func() { a.refreshGalleryView(cx, true) }},
			},
		}
	}}
}

func countLabel(snap appstate.GallerySnapshot) string {
	if snap.Total > 0 && snap.Total != len(snap.Items) {
		return itoa(len(snap.Items)) + " / " + itoa(snap.Total) + " 项"
	}
	if snap.Total > 0 {
		return itoa(snap.Total) + " 项"
	}
	return "资源库"
}

func friendlyError(err error) string {
	return transport.DescribeFailure(err)
}

func (a *App) runBatchAddTag(cx *core.Scope) {
	tag := strings.TrimSpace(a.batchTagInput.Get())
	ids := a.selectedIDSlice()
	if tag == "" || len(ids) == 0 {
		return
	}
	a.batchTagOpen.Set(false)
	a.batchBusy.Set(true)
	a.showToast("正在添加标签…")
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 2*time.Minute)
		defer cancel()
		res := a.client.BatchAddTag(ctx, ids, tag, nil)
		a.post(func() {
			a.batchBusy.Set(false)
			a.batchTagInput.Set("")
			if res.Succeeded > 0 {
				a.exitMultiSelect()
				a.loadSidebarData(cx)
				a.scheduleCalibratedRefresh(cx, 250*time.Millisecond)
			}
			a.showToast(res.Summary("添加标签「" + tag + "」"))
		})
	}()
}

func (a *App) runBatchAddFolder(cx *core.Scope, folder model.Folder) {
	ids := a.selectedIDSlice()
	if len(ids) == 0 {
		return
	}
	a.batchBusy.Set(true)
	a.showToast("正在加入文件夹…")
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 2*time.Minute)
		defer cancel()
		res := a.client.BatchAddFolder(ctx, ids, folder.ID, nil)
		a.post(func() {
			a.batchBusy.Set(false)
			if res.Succeeded > 0 {
				a.exitMultiSelect()
				a.loadSidebarData(cx)
				a.scheduleCalibratedRefresh(cx, 250*time.Millisecond)
			}
			a.showToast(res.Summary("加入「" + folder.Name + "」"))
		})
	}()
}

func (a *App) runBatchDelete(cx *core.Scope) {
	ids := a.selectedIDSlice()
	if len(ids) == 0 {
		return
	}
	a.batchBusy.Set(true)
	a.showToast("正在删除…")
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 2*time.Minute)
		defer cancel()
		res := a.client.BatchDelete(ctx, ids, nil)
		a.post(func() {
			a.batchBusy.Set(false)
			gen := a.gallery.Generation()
			failed := map[string]bool{}
			for _, f := range res.Failures {
				failed[f.ID] = true
			}
			for _, id := range ids {
				if !failed[id] {
					a.gallery.Remove(gen, id)
					_ = a.local.RemoveShareTag(a.session.ScopeKey(), id)
					_ = a.local.RemoveShareTag(a.session.LegacyNameScope(), id)
				}
			}
			a.reloadShareTags()
			if res.Succeeded > 0 {
				a.exitMultiSelect()
				a.loadSidebarData(cx)
				a.scheduleCalibratedRefresh(cx, 350*time.Millisecond)
			}
			a.rebuildGalleryRows()
			a.showToast(res.Summary("移到废纸篓"))
			a.bump()
		})
	}()
}

func (a *App) restoreItem(cx *core.Scope, item model.ItemSummary) {
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 20*time.Second)
		defer cancel()
		err := a.client.Restore(ctx, item.ID)
		a.post(func() {
			if err != nil {
				a.showToast("恢复失败: " + transport.DescribeFailure(err))
				return
			}
			a.gallery.Remove(a.gallery.Generation(), item.ID)
			a.rebuildGalleryRows()
			a.loadSidebarData(cx)
			a.scheduleCalibratedRefresh(cx, 300*time.Millisecond)
			a.showToast("已从废纸篓恢复")
			a.bump()
		})
	}()
}

func (a *App) emptyTrash(cx *core.Scope) {
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 30*time.Second)
		defer cancel()
		err := a.client.EmptyTrash(ctx)
		a.post(func() {
			if err != nil {
				a.showToast("清空失败: " + transport.DescribeFailure(err))
				return
			}
			a.refreshGalleryView(cx, true)
			a.loadSidebarData(cx)
			a.showToast("废纸篓已清空")
		})
	}()
}

func (a *App) searchSpec() query.Spec {
	s := a.searchExtra
	s.Text = strings.TrimSpace(a.searchText.Get())
	s.Type = a.searchType.Get()
	s.Orientation = a.searchOrient.Get()
	return s
}
