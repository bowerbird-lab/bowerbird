package ui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func (a *App) openDetail(cx *core.Scope, flatIndex int, from string) {
	a.detailIndex.Set(flatIndex)
	a.detailFrom = from
	a.detailChrome.Set(false) // Immersive fullscreen by default; tap to toggle header
	a.videoError.Set("")
	a.videoPaused.Set(false)
	a.screen.Set(screenDetail)
	items := a.currentGalleryFor(from).Snapshot().Items
	if flatIndex >= 0 && flatIndex < len(items) {
		a.ensureItemDetail(cx, items[flatIndex].ID)
	}
	a.bump()
}

func (a *App) detailPage(cx *core.Scope) core.Node {
	preview := func() core.Color { return themeRole(cx, "preview") }
	onPreview := func() core.Color { return themeRole(cx, "onBadge") }
	onPreviewMuted := func() core.Color { return themeRole(cx, "onBadge").Faded(0.72) }
	overlay := func() core.Color { return themeRole(cx, "badge") }

	currentItem := func() (model.ItemSummary, int, int, bool) {
		items := a.currentGalleryFor(a.detailFrom).Snapshot().Items
		if len(items) == 0 {
			return model.ItemSummary{}, 0, 0, false
		}
		index := a.detailIndex.Get()
		if index < 0 {
			index = 0
		}
		if index >= len(items) {
			index = len(items) - 1
		}
		return items[index], index, len(items), true
	}

	return core.Dynamic(func(cx *core.Scope) core.Node {
		_ = a.detailIndex.Get()
		_ = a.videoError.Get()
		item, index, total, ok := currentItem()
		if !ok {
			return core.View{
				Style: core.Style{
					Width: core.Pct(100), Height: core.Pct(100),
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 10,
					Background: preview(),
				},
				Children: []core.Node{
					core.Text{Content: "素材已不在当前列表", Style: nsui.TypeBody.Merge(core.Style{Color: onPreviewMuted()})},
					nsui.Button{Text: "返回", Variant: nsui.ButtonSecondary, OnPress: func() { a.backFromDetail() }},
				},
			}
		}
		a.post(func() { a.ensureItemDetail(cx, item.ID) })

		zoom := core.NewSignal(cx, float32(1))
		panX := core.NewSignal(cx, core.Points(0))
		panY := core.NewSignal(cx, core.Points(0))
		swipeX := core.NewSignal(cx, core.Points(0))

		maxPan := func(z float32, dim core.Points) core.Points {
			if z <= 1.01 || dim <= 0 {
				return 0
			}
			return dim * core.Points(z-1) / 2
		}
		clamp := func(v, lo, hi core.Points) core.Points {
			if v < lo {
				return lo
			}
			if v > hi {
				return hi
			}
			return v
		}

		goTo := func(next int) {
			if next < 0 || next >= total {
				return
			}
			a.videoError.Set("")
			a.videoPaused.Set(false)
			a.detailIndex.Set(next)
			items := a.currentGalleryFor(a.detailFrom).Snapshot().Items
			if next < len(items) {
				a.ensureItemDetail(cx, items[next].ID)
			}
		}

		var pinchStartZoom float32 = 1
		var startZoom float32 = 1
		var startPanX, startPanY core.Points
		var atLeftEdge, atRightEdge, switchingMode bool
		var lastTapAt time.Time

		onMediaTap := func() {
			now := time.Now()
			if item.IsImage() && !lastTapAt.IsZero() && now.Sub(lastTapAt) < 280*time.Millisecond {
				lastTapAt = time.Time{}
				// Double-tap toggles zoom and keeps chrome consistent.
				if zoom.Get() > 1.05 {
					zoom.Set(1)
					panX.Set(0)
					panY.Set(0)
				} else {
					zoom.Set(2.4)
					panX.Set(0)
					panY.Set(0)
				}
				a.detailChrome.Update(func(v bool) bool { return !v })
				return
			}
			lastTapAt = now
			a.detailChrome.Update(func(v bool) bool { return !v })
		}

		var mediaContent core.Node
		if item.IsImage() {
			mediaContent = core.View{
				TestID:  "detail.media",
				Style:   core.Style{Width: core.Pct(100), Height: core.Pct(100), Overflow: core.Hidden},
				OnPress: func(core.PressEvent) { onMediaTap() },
				OnPinch: func(ev core.GestureEvent) {
					switch ev.Phase {
					case core.GestureBegan:
						pinchStartZoom = zoom.Get()
					case core.GestureChanged:
						nextZ := pinchStartZoom * ev.Value
						if nextZ < 1 {
							nextZ = 1
						}
						if nextZ > 4 {
							nextZ = 4
						}
						zoom.Set(nextZ)
						win := cx.Env().Window()
						mx, my := maxPan(nextZ, win.Width), maxPan(nextZ, win.Height)
						panX.Set(clamp(panX.Get(), -mx, mx))
						panY.Set(clamp(panY.Get(), -my, my))
					case core.GestureEnded, core.GestureCancelled:
						if zoom.Get() < 1.05 {
							zoom.Set(1)
							panX.Set(0)
							panY.Set(0)
						}
					}
				},
				PanActivation: core.PanActivation{ActiveOffsetX: 12, ActiveOffsetY: 12},
				OnPan: func(ev core.PanEvent) {
					win := cx.Env().Window()
					switch ev.Phase {
					case core.GestureBegan:
						startZoom = zoom.Get()
						startPanX = panX.Get()
						startPanY = panY.Get()
						mx := maxPan(startZoom, win.Width)
						atLeftEdge = startPanX >= mx-3
						atRightEdge = startPanX <= -mx+3
						switchingMode = false
					case core.GestureChanged:
						if startZoom <= 1.02 {
							dx := ev.Translation.X
							if (index == 0 && dx > 0) || (index == total-1 && dx < 0) {
								dx *= 0.25
							}
							swipeX.Set(dx)
							return
						}
						dx := ev.Translation.X
						dy := ev.Translation.Y
						if !switchingMode && math.Abs(float64(dx)) > math.Abs(float64(dy))*1.4 {
							if atLeftEdge && dx > 20 && index > 0 {
								switchingMode = true
							} else if atRightEdge && dx < -20 && index < total-1 {
								switchingMode = true
							}
						}
						if switchingMode {
							swipeX.Set(dx)
							return
						}
						mx, my := maxPan(startZoom, win.Width), maxPan(startZoom, win.Height)
						panX.Set(clamp(startPanX+dx, -mx, mx))
						panY.Set(clamp(startPanY+dy, -my, my))
					case core.GestureEnded, core.GestureCancelled:
						if startZoom <= 1.02 || switchingMode {
							dx := swipeX.Get()
							swipeX.Set(0)
							if ev.Phase == core.GestureEnded {
								if (dx < -64 || ev.Velocity.X < -420) && index < total-1 {
									goTo(index + 1)
								} else if (dx > 64 || ev.Velocity.X > 420) && index > 0 {
									goTo(index - 1)
								}
							}
						}
					}
				},
				Children: []core.Node{
					core.View{
						StyleFunc: func() core.Style {
							return core.Style{
								Width: core.Pct(100), Height: core.Pct(100),
								AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
								Transform: core.Transform{
									Scale:      zoom.Get(),
									TranslateX: panX.Get() + swipeX.Get(),
									TranslateY: panY.Get(),
								},
							}
						},
						Children: []core.Node{
							core.Image{
								Source:      core.Src(a.client.OriginalURL(item.ID)),
								Placeholder: core.Src(a.client.ThumbURL(item.ID)),
								ResizeMode:  core.Contain,
								Style:       core.Style{Width: core.Pct(100), Height: core.Pct(100)},
								TestID:      "detail.image",
							},
						},
					},
				},
			}
		} else {
			mediaContent = core.View{
				TestID: "detail.media",
				Style: core.Style{
					Width: core.Pct(100), Height: core.Pct(100),
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				},
				OnPress:       func(core.PressEvent) { a.detailChrome.Update(func(v bool) bool { return !v }) },
				PanActivation: core.PanActivation{ActiveOffsetX: 36, FailOffsetY: 20},
				OnPan: func(ev core.PanEvent) {
					if ev.Phase != core.GestureEnded {
						return
					}
					if ev.Translation.X < -56 && index < total-1 {
						goTo(index + 1)
					} else if ev.Translation.X > 56 && index > 0 {
						goTo(index - 1)
					}
				},
				Children: []core.Node{a.nonImageMediaNode(cx, item)},
			}
		}

		// Floating translucent overlay header — hidden until user single-taps.
		floatingHeader := core.Show{
			When: func() bool { return a.detailChrome.Get() },
			Then: func(cx *core.Scope) core.Node {
				return core.View{
					StyleFunc: func() core.Style {
						safe := cx.Env().SafeArea()
						return core.Style{
							Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
							Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
							Padding: core.Edges{
								Top:    safe.Top + 6,
								Bottom: 10,
								Left:   safe.Left + 8,
								Right:  safe.Right + 8,
							},
							Background: overlay(),
							ZIndex:     20,
						}
					},
					Children: []core.Node{
						nsui.IconButton{Icon: lucide.ArrowLeft, Label: "返回", Size: 36, IconSize: 20, Tint: onPreview,
							OnPress: func() { a.backFromDetail() }, TestID: "detail.back"},
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
							core.Text{Content: item.DisplayName(), MaxLines: 1,
								Style: nsui.TypeBody.Merge(core.Style{Color: onPreview()})},
							core.Text{Content: itoa(index+1) + " / " + itoa(total),
								Style: nsui.TypeCaption.Merge(core.Style{Color: onPreviewMuted()})},
						}},
						nsui.IconButton{Icon: lucide.Info, Label: "信息", Size: 36, IconSize: 19, Tint: onPreview,
							OnPress: func() {
								a.ensureItemDetail(cx, item.ID)
								a.metaOpen.Set(true)
							}, TestID: "detail.meta"},
						nsui.IconButton{Icon: lucide.Copy, Label: "复制", Size: 36, IconSize: 19, Tint: onPreview,
							OnPress: func() { a.copyItemContent(cx, item) }, TestID: "detail.copy"},
						nsui.IconButton{Icon: lucide.Download, Label: "下载", Size: 36, IconSize: 19, Tint: onPreview,
							OnPress: func() { a.exportItem(cx, item, false) }, TestID: "detail.download"},
						nsui.IconButton{Icon: lucide.Share2, Label: "分享", Size: 36, IconSize: 19, Tint: onPreview,
							OnPress: func() { a.exportItem(cx, item, true) }, TestID: "detail.share"},
						nsui.IconButton{Icon: lucide.Trash, Label: "删除", Size: 36, IconSize: 19, Tint: onPreview,
							OnPress: func() {
								a.quickDelTarget = item
								a.confirmDel.Set(true)
							}, TestID: "detail.delete"},
					},
				}
			},
		}

		return core.View{
			Style: core.Style{
				Width: core.Pct(100), Height: core.Pct(100),
				Background: preview(),
				Overflow:   core.Hidden,
			},
			Children: []core.Node{
				mediaContent,
				floatingHeader,
				a.metadataSheet(cx, item),
				a.deleteDialog(cx),
			},
		}
	})
}

func (a *App) nonImageMediaNode(cx *core.Scope, item model.ItemSummary) core.Node {
	white := themeRole(cx, "onBadge")
	muted := themeRole(cx, "onBadge").Faded(0.72)
	original := a.client.OriginalURL(item.ID)
	if item.IsVideo() {
		if msg := a.videoError.Get(); msg != "" {
			return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.All(24)}, Children: []core.Node{
				core.Text{Content: "视频无法在应用内播放", Style: nsui.TypeTitle.Merge(core.Style{Color: white})},
				core.Text{Content: msg, MaxLines: 3, Style: nsui.TypeCaption.Merge(core.Style{Color: muted, TextAlign: core.TextAlignCenter})},
				core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
					nsui.Button{Text: "下载原视频", Variant: nsui.ButtonSecondary, OnPress: func() { a.exportItem(cx, item, false) }},
					nsui.Button{Text: "用其他应用打开", Variant: nsui.ButtonSecondary, OnPress: func() { a.exportItem(cx, item, true) }},
				}},
			}}
		}
		return core.Video{
			Source:     original,
			Controls:   true,
			ResizeMode: core.Contain,
			PausedFunc: func() bool {
				return a.screen.Get() != screenDetail ||
					cx.Env().Lifecycle() != core.LifecycleActive ||
					a.videoPaused.Get() ||
					a.metaOpen.Get()
			},
			OnError: func(err error) {
				a.videoError.Set("不支持的编码格式或播放失败: " + err.Error())
			},
			Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			TestID: "detail.video",
		}
	}
	if item.IsAudio() {
		return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.All(24)}, Children: []core.Node{
			core.Text{Content: "音频文件", Style: nsui.TypeTitle.Merge(core.Style{Color: white})},
			core.Text{Content: item.Filename, MaxLines: 2, Style: nsui.TypeCaption.Merge(core.Style{Color: white, TextAlign: core.TextAlignCenter})},
			core.Text{Content: "当前版本没有内置音频播放器，可下载或分享后用其他应用播放", MaxLines: 3,
				Style: nsui.TypeCaption.Merge(core.Style{Color: muted, TextAlign: core.TextAlignCenter})},
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				nsui.Button{Text: "下载", Variant: nsui.ButtonSecondary, OnPress: func() { a.exportItem(cx, item, false) }},
				nsui.Button{Text: "分享", Variant: nsui.ButtonSecondary, OnPress: func() { a.exportItem(cx, item, true) }},
			}},
		}}
	}
	return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.All(24)}, Children: []core.Node{
		core.Text{Content: "暂无预览", Style: nsui.TypeTitle.Merge(core.Style{Color: white})},
		core.Text{Content: item.Filename, MaxLines: 2, Style: nsui.TypeCaption.Merge(core.Style{Color: muted, TextAlign: core.TextAlignCenter})},
		core.Text{Content: "可点击屏幕显示顶部菜单进行下载或分享", Style: nsui.TypeCaption.Merge(core.Style{Color: muted})},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			nsui.Button{Text: "下载", Variant: nsui.ButtonSecondary, OnPress: func() { a.exportItem(cx, item, false) }},
			nsui.Button{Text: "分享", Variant: nsui.ButtonSecondary, OnPress: func() { a.exportItem(cx, item, true) }},
		}},
	}}
}

func (a *App) metadataSheet(cx *core.Scope, item model.ItemSummary) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.metaOpen.Get() },
		OnClose: func() { a.metaOpen.Set(false) },
		Title:   "完整元数据",
		Header: []core.Node{
			nsui.Button{Text: "复制汇总", Variant: nsui.ButtonSecondary, OnPress: func() {
				detail := a.detailCache[item.ID]
				cx.SetClipboard(itemInfoText(item, detail))
				a.showToast("已复制 汇总信息")
			}},
		},
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.metaOpen.Get()
				_ = cx.Env().Dark()
				detail, hasDetail := a.detailCache[item.ID]

				copyField := func(label, val string) {
					cx.SetClipboard(val)
					a.showToast("已复制 " + label)
				}
				metaRow := func(label, value string, copyable bool) core.Node {
					if strings.TrimSpace(value) == "" {
						return core.View{}
					}
					var copyBtn core.Node = core.View{}
					if copyable {
						copyBtn = nsui.IconButton{
							Icon:     lucide.Copy,
							Label:    "复制" + label,
							Size:     26,
							IconSize: 14,
							OnPress:  func() { copyField(label, value) },
						}
					}
					return core.View{
						Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(4, 0)},
						Children: []core.Node{
							core.Text{Content: label, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted(), Width: core.Pt(76)})},
							core.Text{Content: value, MaxLines: 4, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Text(), FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)})},
							copyBtn,
						},
					}
				}

				rows := []core.Node{
					metaRow("标题", item.DisplayName(), true),
				}
				if a.isItemShareMarked(item.ID) {
					rows = append(rows, metaRow("来源", "手机分享", false))
				}
				rows = append(rows,
					metaRow("文件名", item.Filename, true),
					metaRow("日期", formatTime(item.BestDate()), false),
				)
				if item.Width > 0 && item.Height > 0 {
					dim := fmt.Sprintf("%d × %d", item.Width, item.Height)
					if item.Orientation != "" {
						dim += " (" + item.Orientation + ")"
					}
					rows = append(rows, metaRow("尺寸", dim, true))
				}
				rows = append(rows,
					metaRow("大小", model.FormatSize(item.Size), false),
					metaRow("格式", strings.ToUpper(item.Extension), false),
					metaRow("MIME", item.MimeType, false),
				)
				if hasDetail {
					rows = append(rows,
						metaRow("路径", detail.FilePath, true),
						metaRow("备注", detail.Note, true),
						metaRow("来源 URL", detail.URL, true),
						metaRow("Prompt", detail.Prompt, true),
					)
					if detail.Rating > 0 {
						rows = append(rows, metaRow("评分", itoa(detail.Rating)+" / 5 ★", false))
					}
					if detail.Meta != nil {
						m := detail.Meta
						cam := strings.TrimSpace(m.ExifMake + " " + m.ExifModel)
						rows = append(rows,
							metaRow("相机", cam, true),
							metaRow("拍摄时间", m.ExifDate, false),
							metaRow("色彩空间", strings.TrimSpace(m.ColorModel+" "+m.ColorSpace), false),
							metaRow("像素", m.Megapixels, false),
						)
					}
					if detail.DominantColor != "" {
						dc := strings.ToUpper(detail.DominantColor)
						colorVal, ok := parseHexColor(dc)
						var swatch core.Node = core.View{}
						if ok {
							swatch = core.View{Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), CornerRadius: 8,
								Background: colorVal, BorderWidth: 1, BorderColor: colors.Border()}}
						}
						rows = append(rows, core.View{
							Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(4, 0)},
							Children: []core.Node{
								core.Text{Content: "主色", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted(), Width: core.Pt(76)})},
								swatch,
								core.Text{Content: dc, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Text(), FlexGrow: 1})},
								nsui.IconButton{Icon: lucide.Copy, Label: "复制主色", Size: 26, IconSize: 14, OnPress: func() { copyField("主色", dc) }},
							},
						})
					}
					if len(detail.Palette) > 0 {
						swatches := make([]core.Node, 0, len(detail.Palette))
						for _, hex := range detail.Palette {
							if cv, ok := parseHexColor(hex); ok {
								h := strings.ToUpper(hex)
								swatches = append(swatches, core.View{
									Style:   core.Style{Width: core.Pt(28), Height: core.Pt(20), CornerRadius: 5, Background: cv, BorderWidth: 1, BorderColor: colors.Border()},
									OnPress: func(core.PressEvent) { copyField("色板颜色", h) },
								})
							}
						}
						rows = append(rows, core.View{
							Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(4, 0)},
							Children: []core.Node{
								core.Text{Content: "调色板", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted(), Width: core.Pt(76)})},
								core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6, FlexGrow: 1}, Children: swatches},
								nsui.IconButton{Icon: lucide.Copy, Label: "复制调色板", Size: 26, IconSize: 14,
									OnPress: func() { copyField("调色板", strings.Join(detail.Palette, ", ")) }},
							},
						})
					}
				}
				if len(item.Folders) > 0 {
					rows = append(rows, metaRow("文件夹", strings.Join(item.Folders, "、"), true))
				}
				if len(item.Tags) > 0 {
					rows = append(rows, metaRow("标签", strings.Join(item.Tags, "、"), true))
				}
				rows = append(rows, metaRow("ID", item.ID, true))

				return core.Scroll{
					Style:    core.Style{MaxHeight: core.Pt(440)},
					Children: []core.Node{core.View{Style: core.Style{Gap: 4, Padding: core.Edges{Bottom: 16}}, Children: rows}},
				}
			}),
		},
	})
}

func (a *App) deleteDialog(cx *core.Scope) core.Node {
	return nsui.Dialog{
		Open:         func() bool { return a.confirmDel.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.confirmDel.Set(open) },
		Title:        "删除素材？",
		Description:  "「" + a.quickDelTarget.DisplayName() + "」将移到电脑的废纸篓，可在废纸篓中恢复。",
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.confirmDel.Set(false) }},
			nsui.Button{Text: "移到废纸篓", OnPress: func() {
				a.confirmDel.Set(false)
				a.deleteItem(cx, a.quickDelTarget)
			}},
		},
	}
}

func (a *App) backFromDetail() {
	a.videoPaused.Set(true)
	if a.detailFrom == screenSearch {
		a.screen.Set(screenSearch)
	} else {
		a.screen.Set(screenGallery)
	}
}

func (a *App) ensureItemDetail(cx *core.Scope, id string) {
	if id == "" {
		return
	}
	if _, ok := a.detailCache[id]; ok {
		return
	}
	if a.detailLoading[id] {
		return
	}
	a.detailLoading[id] = true
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 15*time.Second)
		defer cancel()
		detail, err := a.client.Item(ctx, id)
		a.post(func() {
			delete(a.detailLoading, id)
			if err != nil {
				return
			}
			a.detailCache[id] = detail
			a.bump()
		})
	}()
}

func (a *App) deleteItem(cx *core.Scope, item model.ItemSummary) {
	from := a.detailFrom
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 20*time.Second)
		defer cancel()
		err := a.client.Delete(ctx, item.ID)
		a.post(func() {
			if err != nil {
				a.showToast(transport.DescribeFailure(err))
				return
			}
			g := a.currentGalleryFor(from)
			g.Remove(g.Generation(), item.ID)
			if from == screenSearch {
				a.rebuildSearchRows()
			} else {
				a.rebuildGalleryRows()
			}
			_ = a.local.RemoveShareTag(a.session.ScopeKey(), item.ID)
			_ = a.local.RemoveShareTag(a.session.LegacyNameScope(), item.ID)
			a.reloadShareTags()
			a.loadSidebarData(cx)
			a.scheduleCalibratedRefresh(cx, 350*time.Millisecond)
			a.showToast("已移到废纸篓")
			if a.screen.Get() == screenDetail {
				a.backFromDetail()
			}
			a.bump()
		})
	}()
}

func itemInfoText(item model.ItemSummary, detail model.ItemDetail) string {
	var b strings.Builder
	b.WriteString(item.DisplayName())
	b.WriteString("\n")
	if item.Filename != "" {
		b.WriteString("文件: " + item.Filename + "\n")
	}
	if item.MimeType != "" {
		b.WriteString("类型: " + item.MimeType + "\n")
	}
	if item.Width > 0 && item.Height > 0 {
		b.WriteString(fmt.Sprintf("尺寸: %d × %d\n", item.Width, item.Height))
	}
	if item.Size > 0 {
		b.WriteString("大小: " + model.FormatSize(item.Size) + "\n")
	}
	if d := item.BestDate(); d > 0 {
		b.WriteString("日期: " + formatTime(d) + "\n")
	}
	if detail.Note != "" {
		b.WriteString("备注: " + detail.Note + "\n")
	}
	if detail.URL != "" {
		b.WriteString("链接: " + detail.URL + "\n")
	}
	if detail.Prompt != "" {
		b.WriteString("Prompt: " + detail.Prompt + "\n")
	}
	if len(item.Tags) > 0 {
		b.WriteString("标签: " + strings.Join(item.Tags, "、") + "\n")
	}
	if len(item.Folders) > 0 {
		b.WriteString("文件夹: " + strings.Join(item.Folders, "、") + "\n")
	}
	if item.ID != "" {
		b.WriteString("ID: " + item.ID)
	}
	return strings.TrimSpace(b.String())
}

func formatTime(ms int64) string {
	if ms < 946684800000 {
		return ""
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04")
}
