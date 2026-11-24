package ui

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/plugin"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func (a *App) openDetail(cx *core.Scope, flatIndex int, from string, src core.Bounds) {
	a.detailIndex.Set(flatIndex)
	a.detailPageIndex.Set(0)
	a.detailFrom = from
	a.detailChrome.Set(false) // Immersive fullscreen by default; tap to toggle header
	a.videoError.Set("")
	a.videoPaused.Set(false)
	items := a.currentGalleryFor(from).Snapshot().Items
	if flatIndex >= 0 && flatIndex < len(items) {
		item := items[flatIndex]
		a.ensureItemDetail(cx, item.ID)
		if item.VariantOf != "" {
			a.ensureItemDetail(cx, item.VariantOf)
		}
		for _, vid := range item.Variants {
			a.ensureItemDetail(cx, vid)
		}
		thumb := a.client.MediaURL(item.ThumbnailURL)
		if thumb == "" {
			thumb = a.client.ThumbURL(item.ID)
		}
		win := cx.Env().Window()
		dst := detailContainBounds(win, item.Width, item.Height)
		if item.IsImage() && src.Size.Width >= 24 && src.Size.Height >= 24 && dst.Size.Width > 0 && dst.Size.Height > 0 {
			a.matchFrom = src
			a.matchTo = dst
			a.matchThumb = thumb
			a.matchOriginal = a.client.OriginalURL(item.ID)
			a.matchOpenedIndex = flatIndex
			a.startPreviewMatch(true, func() {
				a.screen.Set(screenDetail)
				a.bump()
			})
			return
		}
	}
	a.cancelPreviewMatch()
	a.screen.Set(screenDetail)
	a.bump()
}

func detailContainBounds(win core.Size, itemW, itemH int) core.Bounds {
	if win.Width <= 0 || win.Height <= 0 {
		return core.Bounds{}
	}
	if itemW <= 0 || itemH <= 0 {
		s := min(win.Width, win.Height)
		return core.Bounds{
			Origin: core.Point{X: (win.Width - s) / 2, Y: (win.Height - s) / 2},
			Size:   core.Size{Width: s, Height: s},
		}
	}
	w, h := float64(itemW), float64(itemH)
	scale := min(float64(win.Width)/w, float64(win.Height)/h)
	dw := core.Points(w * scale)
	dh := core.Points(h * scale)
	return core.Bounds{
		Origin: core.Point{X: (win.Width - dw) / 2, Y: (win.Height - dh) / 2},
		Size:   core.Size{Width: dw, Height: dh},
	}
}

const (
	previewMatchOpenDuration  = 240 * time.Millisecond
	previewMatchCloseDuration = 220 * time.Millisecond
	previewMatchDuration      = previewMatchOpenDuration
)

func (a *App) cancelPreviewMatch() {
	a.matchGen++
	a.matchAnimated = false
	a.matchInteractive = false
	a.matchInteractiveReady = false
	a.matchPendingAction = 0
	a.matchProgressVal = nil
	a.matchDynamicScope = nil
	a.matchCornerDur.Set(0)
	a.matchCornerRadius.Set(0)
	a.matchStage.Set(0)
	a.matchDetail.Set(false)
}

func (a *App) canRunHeroBack() bool {
	return a.detailIndex.Get() == a.matchOpenedIndex &&
		a.detailPageIndex.Get() == 0 &&
		a.matchThumb != "" &&
		a.matchFrom.Size.Width >= 24 &&
		a.matchFrom.Size.Height >= 24 &&
		a.matchTo.Size.Width > 0 &&
		a.matchTo.Size.Height > 0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func interactiveHeroProgress(gestureProgress float64) float64 {
	return 1.0 - 0.50*clamp01(gestureProgress)
}

func (a *App) startInteractiveBack(progress float64) {
	if !a.canRunHeroBack() || a.matchStage.Get() != 0 {
		return
	}
	if (!a.env.HasHost && !a.forceMatchAnim) || a.scope.Env().ReduceMotion() {
		return
	}
	a.videoPaused.Set(true)
	a.matchGen++
	a.matchAnimated = false
	a.matchInteractive = true
	a.matchInteractiveReady = false
	a.matchPendingAction = 0
	a.matchPendingProgress = clamp01(progress)
	a.matchProgressVal = nil
	a.matchDynamicScope = nil
	a.matchCornerDur.Set(0)
	a.matchCornerRadius.Set(0)
	a.matchStage.Set(2)
	a.matchRev.Update(func(v int) int { return v + 1 })
}

func (a *App) updateInteractiveBack(raw float64) {
	if !a.matchInteractive {
		return
	}
	p := clamp01(raw)
	a.matchPendingProgress = p
	if a.matchInteractiveReady && a.matchProgressVal != nil {
		hp := interactiveHeroProgress(p)
		a.matchProgressVal.Set(hp)
		a.matchCornerDur.Set(0)
		a.matchCornerRadius.Set(a.matchTileRadius * core.Points(1.0-hp))
	}
}

func (a *App) finishInteractiveBack(commit bool) {
	if !a.matchInteractive {
		if commit {
			a.backFromDetail()
		}
		return
	}
	if !a.matchInteractiveReady {
		if commit {
			a.matchPendingAction = 1
		} else {
			a.matchPendingAction = -1
		}
		return
	}
	gen := a.matchGen
	a.matchInteractive = false
	a.matchInteractiveReady = false
	a.matchPendingAction = 0
	p := a.matchProgressVal
	dynCx := a.matchDynamicScope
	if p == nil || dynCx == nil || dynCx.Disposed() {
		if commit {
			a.screen.Set(a.detailReturnScreen())
		} else {
			a.screen.Set(screenDetail)
			a.bump()
		}
		a.matchStage.Set(0)
		return
	}
	easing := core.BezierEasing(core.EaseOut)
	if commit {
		a.screen.Set(a.detailReturnScreen())
		a.matchCornerDur.Set(200 * time.Millisecond)
		a.matchCornerRadius.Set(a.matchTileRadius)
		dynCx.StartAnimation(core.Timing(p, 0, core.TimingConfig{
			Duration: 200 * time.Millisecond,
			Easing:   easing,
		}), func(bool) {
			if a.matchGen == gen {
				a.matchStage.Set(0)
			}
		})
		a.after(320*time.Millisecond, func() {
			if a.matchGen == gen && a.matchStage.Get() != 0 {
				a.matchStage.Set(0)
			}
		})
		return
	}
	a.matchCornerDur.Set(180 * time.Millisecond)
	a.matchCornerRadius.Set(0)
	dynCx.StartAnimation(core.Timing(p, 1, core.TimingConfig{
		Duration: 180 * time.Millisecond,
		Easing:   easing,
	}), func(bool) {
		if a.matchGen != gen {
			return
		}
		a.screen.Set(screenDetail)
		a.bump()
		a.after(140*time.Millisecond, func() {
			if a.matchGen == gen {
				a.matchStage.Set(0)
			}
		})
	})
	a.after(380*time.Millisecond, func() {
		if a.matchGen == gen {
			if a.screen.Get() != screenDetail {
				a.screen.Set(screenDetail)
				a.bump()
			}
			if a.matchStage.Get() != 0 {
				a.matchStage.Set(0)
			}
		}
	})
}

// startPreviewMatch runs a GPU-driven hero + backdrop fade between the tapped
// grid tile and the detail page's contain frame. Opening keeps the gallery
// visible underneath while the backdrop dims to black, then swaps in screenDetail
// after the flight finishes. Closing covers screenDetail, immediately reveals
// the gallery underneath, and shrinks the hero back to its tile.
func (a *App) startPreviewMatch(open bool, apply func()) {
	if a.matchThumb == "" || a.matchFrom.Size.Width < 24 || a.matchFrom.Size.Height < 24 ||
		a.matchTo.Size.Width <= 0 || a.matchTo.Size.Height <= 0 {
		a.cancelPreviewMatch()
		apply()
		return
	}
	if (!a.env.HasHost && !a.forceMatchAnim) || a.scope.Env().ReduceMotion() {
		a.cancelPreviewMatch()
		apply()
		return
	}
	a.matchGen++
	gen := a.matchGen
	a.matchAnimated = false
	a.matchApply = apply
	a.matchApplied = false
	if open {
		a.matchStage.Set(1)
	} else {
		a.matchStage.Set(2)
	}
	a.matchRev.Update(func(v int) int { return v + 1 })

	// Unconditional safety net so the destination screen always mounts and
	// the overlay never lingers even if an animation callback is skipped.
	a.after(previewMatchOpenDuration+320*time.Millisecond, func() {
		if a.matchGen != gen {
			return
		}
		if !a.matchApplied && a.matchApply != nil {
			a.matchApplied = true
			a.matchApply()
		}
		if a.matchStage.Get() != 0 {
			a.matchStage.Set(0)
		}
	})
}

// previewMatchOverlay renders a single hero thumbnail moving on GPU
// (TranslateX, TranslateY, Scale) over a fading dark backdrop.
func (a *App) previewMatchOverlay(cx *core.Scope) core.Node {
	return core.Show{When: func() bool { return a.matchStage.Get() != 0 }, Then: func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			_ = a.matchRev.Get()
			stage := core.Untracked(cx, a.matchStage.Get)
			if stage == 0 || a.matchThumb == "" || a.matchTo.Size.Width <= 0 || a.matchTo.Size.Height <= 0 {
				return core.View{}
			}
			gen := a.matchGen
			open := stage == 1
			thumb := a.matchThumb
			src := a.matchFrom
			dst := a.matchTo

			sx := float64(src.Origin.X + src.Size.Width/2)
			sy := float64(src.Origin.Y + src.Size.Height/2)
			dx := float64(dst.Origin.X + dst.Size.Width/2)
			dy := float64(dst.Origin.Y + dst.Size.Height/2)
			fromTX := sx - dx
			fromTY := sy - dy
			sx0 := float64(src.Size.Width) / float64(dst.Size.Width)
			sy0 := float64(src.Size.Height) / float64(dst.Size.Height)
			if sx0 < 0.05 {
				sx0 = 0.05
			}
			if sy0 < 0.05 {
				sy0 = 0.05
			}
			u0 := max(sx0, sy0)
			tileRadius := core.Points(8 / ((sx0 + sy0) / 2))
			if tileRadius > 48 {
				tileRadius = 48
			}
			a.matchTileRadius = tileRadius

			startVal := 0.0
			if !open {
				startVal = 1.0
			}
			a.matchCornerDur.Set(0)
			if open {
				a.matchCornerRadius.Set(tileRadius)
			} else {
				a.matchCornerRadius.Set(0)
			}
			progress := core.NewAnimatedValue(cx, startVal)
			easing := core.BezierEasing(core.EaseOut)
			interactive := !open && a.matchInteractive
			if interactive {
				a.matchProgressVal = progress
				a.matchDynamicScope = cx
			}

			if open {
				cx.StartAnimation(core.Timing(progress, 1, core.TimingConfig{
					Duration: previewMatchOpenDuration,
					Easing:   easing,
				}), func(bool) {
					if a.matchGen != gen {
						return
					}
					if !a.matchApplied && a.matchApply != nil {
						a.matchApplied = true
						a.matchApply()
					}
					a.after(140*time.Millisecond, func() {
						if a.matchGen == gen {
							a.matchStage.Set(0)
						}
					})
				})
				a.scope.RequestAnimationFrame(func(time.Time) {
					if a.matchGen != gen || cx.Disposed() {
						return
					}
					a.matchCornerDur.Set(previewMatchOpenDuration)
					a.matchCornerRadius.Set(0)
				})
			} else if interactive {
				a.scope.RequestAnimationFrame(func(time.Time) {
					if a.matchGen != gen || !a.matchInteractive || cx.Disposed() {
						return
					}
					a.matchInteractiveReady = true
					a.screen.Set(a.detailReturnScreen())
					if a.matchPendingAction != 0 {
						a.finishInteractiveBack(a.matchPendingAction > 0)
						return
					}
					if a.matchPendingProgress > 0 {
						hp := interactiveHeroProgress(a.matchPendingProgress)
						progress.Set(hp)
						a.matchCornerDur.Set(0)
						a.matchCornerRadius.Set(tileRadius * core.Points(1.0-hp))
					}
				})
			} else {
				a.scope.RequestAnimationFrame(func(time.Time) {
					if a.matchGen != gen || cx.Disposed() {
						return
					}
					if !a.matchApplied && a.matchApply != nil {
						a.matchApplied = true
						a.matchApply()
					}
					a.matchCornerDur.Set(previewMatchCloseDuration)
					a.matchCornerRadius.Set(tileRadius)
					cx.StartAnimation(core.Timing(progress, 0, core.TimingConfig{
						Duration: previewMatchCloseDuration,
						Easing:   easing,
					}), func(bool) {
						if a.matchGen == gen {
							a.matchStage.Set(0)
						}
					})
				})
			}

			backdropAnim := &core.AnimatedStyle{
				Opacity: core.Interpolate(progress, core.Interpolation{
					Input:  []float64{0, 1},
					Output: []float64{0, 1},
				}),
			}
			clipScaleX := core.Interpolate(progress, core.Interpolation{
				Input:  []float64{0, 1},
				Output: []float64{sx0, 1},
			})
			clipScaleY := core.Interpolate(progress, core.Interpolation{
				Input:  []float64{0, 1},
				Output: []float64{sy0, 1},
			})
			imageUniform := core.Interpolate(progress, core.Interpolation{
				Input:  []float64{0, 1},
				Output: []float64{u0, 1},
			})
			clipAnim := &core.AnimatedStyle{
				TranslateX: core.Interpolate(progress, core.Interpolation{
					Input:  []float64{0, 1},
					Output: []float64{fromTX, 0},
				}),
				TranslateY: core.Interpolate(progress, core.Interpolation{
					Input:  []float64{0, 1},
					Output: []float64{fromTY, 0},
				}),
				ScaleX: clipScaleX,
				ScaleY: clipScaleY,
			}
			// Compensate non-uniform clip scaling so the photo keeps its true aspect ratio
			// while morphing cleanly between the square Cover crop and full Contain frame.
			innerAnim := &core.AnimatedStyle{
				ScaleX: core.Div(imageUniform, clipScaleX),
				ScaleY: core.Div(imageUniform, clipScaleY),
			}

			return core.View{
				TestID: "preview.match.root",
				Style: core.Style{
					Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
					PointerEvents: core.PointerEventsNone, ZIndex: 100,
				},
				Children: []core.Node{
					core.View{
						TestID: "preview.match.backdrop",
						StyleFunc: func() core.Style {
							return core.Style{
								Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
								Background: nsui.ColorsOf(cx).Background(),
								Animated:   backdropAnim,
							}
						},
					},
					core.View{
						TestID: "preview.match",
						StyleFunc: func() core.Style {
							dur := a.matchCornerDur.Get()
							var tr core.Transition
							if dur > 0 {
								tr = core.Transition{
									Duration:   dur,
									Easing:     core.EaseOut,
									Properties: core.TransitionBorder,
								}
							}
							return core.Style{
								Position: core.Absolute, Left: core.Pt(dst.Origin.X), Top: core.Pt(dst.Origin.Y),
								Width: core.Pt(dst.Size.Width), Height: core.Pt(dst.Size.Height),
								CornerRadius: a.matchCornerRadius.Get(),
								Overflow:     core.Hidden,
								Animated:     clipAnim,
								Transition:   tr,
							}
						},
						Children: []core.Node{
							core.View{
								Style: core.Style{
									Width: core.Pct(100), Height: core.Pct(100),
									Animated: innerAnim,
								},
								Children: []core.Node{
									core.Image{
										Source:       core.Src(thumb),
										ResizeMode:   core.Contain,
										FadeDuration: time.Millisecond,
										Style:        core.Style{Width: core.Pct(100), Height: core.Pct(100)},
									},
								},
							},
						},
					},
				},
			}
		})
	}}
}

func (a *App) resolveDetailPages(cx *core.Scope, galleryItems []model.ItemSummary, galleryIdx int) ([]model.ItemSummary, int) {
	if galleryIdx < 0 || galleryIdx >= len(galleryItems) {
		return nil, 0
	}
	base := galleryItems[galleryIdx]
	byID := make(map[string]model.ItemSummary, len(galleryItems)+len(a.detailCache))
	for _, it := range galleryItems {
		byID[it.ID] = it
	}
	for id, d := range a.detailCache {
		byID[id] = d.Summary()
	}
	if d, ok := byID[base.ID]; ok && len(d.Variants) > len(base.Variants) {
		base = d
	}

	root := base
	if base.VariantOf != "" {
		if parent, ok := byID[base.VariantOf]; ok {
			root = parent
		} else {
			parentID := base.VariantOf
			a.post(func() { a.ensureItemDetail(cx, parentID) })
			return []model.ItemSummary{base}, 0
		}
	}

	if len(root.Variants) == 0 {
		return []model.ItemSummary{base}, 0
	}

	orderedIDs := make([]string, 0, 1+len(root.Variants))
	orderedIDs = append(orderedIDs, root.ID)
	seen := map[string]bool{root.ID: true}
	for _, vid := range root.Variants {
		if vid == "" || seen[vid] {
			continue
		}
		seen[vid] = true
		orderedIDs = append(orderedIDs, vid)
	}

	pages := make([]model.ItemSummary, 0, len(orderedIDs))
	var missing []string
	for _, pid := range orderedIDs {
		if it, ok := byID[pid]; ok {
			pages = append(pages, it)
			if _, cached := a.detailCache[pid]; !cached {
				missing = append(missing, pid)
			}
		} else {
			pages = append(pages, model.ItemSummary{
				ID:           pid,
				Name:         pid,
				ThumbnailURL: "/api/v1/thumbs/" + pid,
				OriginalURL:  "/api/v1/originals/" + pid,
			})
			missing = append(missing, pid)
		}
	}
	if len(missing) > 0 {
		a.post(func() {
			for _, pid := range missing {
				a.ensureItemDetail(cx, pid)
			}
		})
	}

	pageIdx := a.detailPageIndex.Get()
	if pageIdx < 0 {
		pageIdx = 0
	}
	if pageIdx >= len(pages) {
		pageIdx = len(pages) - 1
	}
	return pages, pageIdx
}

func visibleGalleryPosition(items []model.ItemSummary, flatIdx int) (pos, total int, prevIdx, nextIdx int) {
	slots := visibleTileSlots(items)
	total = len(slots)
	prevIdx, nextIdx = -1, -1
	for i, s := range slots {
		if s.FlatIndex == flatIdx {
			pos = i
			if i > 0 {
				prevIdx = slots[i-1].FlatIndex
			}
			if i+1 < len(slots) {
				nextIdx = slots[i+1].FlatIndex
			}
			return pos, total, prevIdx, nextIdx
		}
	}
	return flatIdx, len(items), flatIdx - 1, flatIdx + 1
}

func (a *App) detailPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	onPreview := func() core.Color { return themeRole(cx, "onBadge") }
	onPreviewMuted := func() core.Color { return themeRole(cx, "onBadge").Faded(0.72) }
	overlay := func() core.Color { return themeRole(cx, "badge") }

	currentItem := func() ([]model.ItemSummary, int, bool) {
		items := a.currentGalleryFor(a.detailFrom).Snapshot().Items
		if len(items) == 0 {
			return nil, 0, false
		}
		index := a.detailIndex.Get()
		if index < 0 {
			index = 0
		}
		if index >= len(items) {
			index = len(items) - 1
		}
		return items, index, true
	}

	return core.Dynamic(func(cx *core.Scope) core.Node {
		_ = a.detailIndex.Get()
		_ = a.detailPageIndex.Get()
		_ = a.rev.Get()
		_ = a.videoError.Get()
		items, index, ok := currentItem()
		if !ok {
			return core.View{
				StyleFunc: func() core.Style {
					return core.Style{
						Width: core.Pct(100), Height: core.Pct(100),
						AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 10,
						Background: colors.Background(),
					}
				},
				Children: []core.Node{
					core.Text{Content: "素材已不在当前列表", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Muted)},
					mobileActionButton(cx, colors, mobileBtnSpec{
						Text: "返回", Variant: nsui.ButtonSecondary,
						OnPress: func() { a.backFromDetail() },
					}),
				},
			}
		}
		pages, pageIdx := a.resolveDetailPages(cx, items, index)
		item := pages[pageIdx]
		visPos, visTotal, prevGalleryIdx, nextGalleryIdx := visibleGalleryPosition(items, index)
		a.post(func() { a.ensureItemDetail(cx, item.ID) })

		zoom := core.NewSignal(cx, float32(1))
		panX := core.NewSignal(cx, core.Points(0))
		panY := core.NewSignal(cx, core.Points(0))
		swipeX := core.NewSignal(cx, core.Points(0))
		smooth := core.NewSignal(cx, true)

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

		goToGallery := func(next int) {
			if next < 0 || next >= len(items) {
				return
			}
			a.videoError.Set("")
			a.videoPaused.Set(false)
			a.detailPageIndex.Set(0)
			a.detailIndex.Set(next)
			a.ensureItemDetail(cx, items[next].ID)
		}

		goToPage := func(nextPage int) {
			if nextPage < 0 || nextPage >= len(pages) {
				return
			}
			a.videoError.Set("")
			a.videoPaused.Set(false)
			a.detailPageIndex.Set(nextPage)
			a.ensureItemDetail(cx, pages[nextPage].ID)
		}

		canStepPrev := pageIdx > 0 || prevGalleryIdx >= 0
		canStepNext := pageIdx < len(pages)-1 || nextGalleryIdx >= 0
		stepPrev := func() {
			if pageIdx > 0 {
				goToPage(pageIdx - 1)
			} else if prevGalleryIdx >= 0 {
				goToGallery(prevGalleryIdx)
			}
		}
		stepNext := func() {
			if pageIdx < len(pages)-1 {
				goToPage(pageIdx + 1)
			} else if nextGalleryIdx >= 0 {
				goToGallery(nextGalleryIdx)
			}
		}

		var pinchStartZoom float32 = 1
		var startZoom float32 = 1
		var startPanX, startPanY core.Points
		var atLeftEdge, atRightEdge, switchingMode bool
		var axisLocked, horizontalSwipe bool
		var lastTapAt time.Time

		onMediaTap := func() {
			now := time.Now()
			if item.IsImage() && !lastTapAt.IsZero() && now.Sub(lastTapAt) < 280*time.Millisecond {
				lastTapAt = time.Time{}
				// Double-tap toggles zoom with a smooth transition.
				smooth.Set(true)
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
			origReady := core.NewSignal(cx, false)
			thumbURL := a.itemThumbURL(item)
			mediaContent = core.View{
				TestID:  "detail.media",
				Style:   core.Style{Width: core.Pct(100), Height: core.Pct(100), Overflow: core.Hidden},
				OnPress: func(core.PressEvent) { onMediaTap() },
				OnPinch: func(ev core.GestureEvent) {
					switch ev.Phase {
					case core.GestureBegan:
						smooth.Set(false)
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
						smooth.Set(true)
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
						smooth.Set(false)
						startZoom = zoom.Get()
						startPanX = panX.Get()
						startPanY = panY.Get()
						mx := maxPan(startZoom, win.Width)
						atLeftEdge = startPanX >= mx-3
						atRightEdge = startPanX <= -mx+3
						switchingMode = false
						axisLocked = false
						horizontalSwipe = false
					case core.GestureChanged:
						dx := ev.Translation.X
						dy := ev.Translation.Y
						if startZoom <= 1.02 {
							// Lock the gesture to an axis first: a mostly
							// vertical swipe must not page sideways because
							// of the small sideways wobble of a finger.
							if !axisLocked {
								if math.Abs(float64(dx)) > 8 || math.Abs(float64(dy)) > 8 {
									axisLocked = true
									horizontalSwipe = math.Abs(float64(dx)) > math.Abs(float64(dy))*1.35
								}
							}
							if !horizontalSwipe {
								swipeX.Set(0)
								return
							}
							if (!canStepPrev && dx > 0) || (!canStepNext && dx < 0) {
								dx *= 0.25
							}
							swipeX.Set(dx)
							return
						}
						if !switchingMode && math.Abs(float64(dx)) > math.Abs(float64(dy))*1.6 {
							if atLeftEdge && dx > 20 && canStepPrev {
								switchingMode = true
							} else if atRightEdge && dx < -20 && canStepNext {
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
						smooth.Set(true)
						if startZoom <= 1.02 || switchingMode {
							dx := swipeX.Get()
							swipeX.Set(0)
							if ev.Phase != core.GestureEnded {
								return
							}
							if startZoom <= 1.02 && !horizontalSwipe {
								return
							}
							// A page turn needs both a clear sideways intent
							// and a distance or a fast sideways flick.
							dy := ev.Translation.Y
							vx, vy := ev.Velocity.X, ev.Velocity.Y
							sideways := math.Abs(float64(dx)) > math.Abs(float64(dy))*1.35
							flick := math.Abs(float64(vx)) > 700 && math.Abs(float64(vx)) > math.Abs(float64(vy))*1.6
							if !sideways && !flick {
								return
							}
							threshold := win.Width * 0.2
							if threshold < 84 {
								threshold = 84
							}
							if (dx < -threshold || (flick && vx < 0)) && canStepNext {
								stepNext()
							} else if (dx > threshold || (flick && vx > 0)) && canStepPrev {
								stepPrev()
							}
						}
					}
				},
				Children: []core.Node{
					core.View{
						StyleFunc: func() core.Style {
							tr := core.Transition{Duration: 220 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionTransform}
							if !smooth.Get() {
								tr = core.Transition{}
							}
							return core.Style{
								Width: core.Pct(100), Height: core.Pct(100),
								AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
								Transform: core.Transform{
									Scale:      zoom.Get(),
									TranslateX: panX.Get() + swipeX.Get(),
									TranslateY: panY.Get(),
								},
								Transition: tr,
							}
						},
						Children: []core.Node{
							core.Show{
								When: func() bool { return !origReady.Get() },
								Then: func(cx *core.Scope) core.Node {
									return core.Image{
										Source:       core.Src(thumbURL),
										ResizeMode:   core.Contain,
										FadeDuration: time.Millisecond,
										Style: core.Style{
											Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0),
										},
									}
								},
							},
							core.Image{
								Source:       core.Src(a.client.OriginalURL(item.ID)),
								ResizeMode:   core.Contain,
								FadeDuration: time.Millisecond,
								OnLoad: func(core.Size) {
									cx.AfterFunc(40*time.Millisecond, func() { origReady.Set(true) })
								},
								Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100)},
								TestID: "detail.image",
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
				PanActivation: core.PanActivation{ActiveOffsetX: 42, FailOffsetY: 24},
				OnPan: func(ev core.PanEvent) {
					if ev.Phase != core.GestureEnded {
						return
					}
					dx, dy := ev.Translation.X, ev.Translation.Y
					if math.Abs(float64(dx)) <= math.Abs(float64(dy))*1.5 {
						return
					}
					threshold := cx.Env().Window().Width * 0.2
					if threshold < 84 {
						threshold = 84
					}
					if dx < -threshold && canStepNext {
						stepNext()
					} else if dx > threshold && canStepPrev {
						stepPrev()
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
						mobileIconButton(cx, mobileIconBtnSpec{
							TestID: "detail.back", Label: "返回", Icon: lucide.ArrowLeft,
							Variant: mobileIconGhost, Tint: onPreview,
							OnPress: func() { a.backFromDetail() },
						}),
						core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
							core.Text{Content: item.DisplayName(), MaxLines: 1,
								Style: nsui.TypeBody.Merge(core.Style{Color: onPreview()})},
							core.Text{Content: func() string {
								s := itoa(visPos+1) + " / " + itoa(visTotal)
								if len(pages) > 1 {
									s += " · 第 " + itoa(pageIdx+1) + "/" + itoa(len(pages)) + " 页"
								}
								return s
							}(),
								Style: nsui.TypeCaption.Merge(core.Style{Color: onPreviewMuted()})},
						}},
						mobileIconButton(cx, mobileIconBtnSpec{
							TestID: "detail.meta", Label: "信息", Icon: lucide.Info, IconSize: 19,
							Variant: mobileIconGhost, Tint: onPreview,
							OnPress: func() {
								a.ensureItemDetail(cx, item.ID)
								a.metaOpen.Set(true)
							},
						}),
						mobileIconButton(cx, mobileIconBtnSpec{
							TestID: "detail.copy", Label: "复制", Icon: lucide.Copy, IconSize: 19,
							Variant: mobileIconGhost, Tint: onPreview,
							OnPress: func() { a.copyItemContent(cx, item) },
						}),
						mobileIconButton(cx, mobileIconBtnSpec{
							TestID: "detail.download", Label: "下载", Icon: lucide.Download, IconSize: 19,
							Variant: mobileIconGhost, Tint: onPreview,
							OnPress: func() { a.exportItem(cx, item, false) },
						}),
						mobileIconButton(cx, mobileIconBtnSpec{
							TestID: "detail.share", Label: "分享", Icon: lucide.Share2, IconSize: 19,
							Variant: mobileIconGhost, Tint: onPreview,
							OnPress: func() { a.exportItem(cx, item, true) },
						}),
						mobileIconButton(cx, mobileIconBtnSpec{
							TestID: "detail.delete", Label: "删除", Icon: lucide.Trash, IconSize: 19,
							Variant: mobileIconGhost, Tint: onPreview,
							OnPress: func() {
								a.quickDelTarget = item
								a.confirmDel.Set(true)
							},
						}),
					},
				}
			},
		}

		pageBar := core.Show{
			When: func() bool { return len(pages) > 1 },
			Then: func(cx *core.Scope) core.Node {
				return core.Show{
					When: func() bool { return a.detailChrome.Get() },
					Then: func(cx *core.Scope) core.Node {
						thumbs := make([]core.Node, 0, len(pages))
						for i, pItem := range pages {
							idxCopy := i
							active := i == pageIdx
							thumbURL := a.itemThumbURL(pItem)
							thumbs = append(thumbs, core.View{
								TestID: "detail.page." + itoa(i),
								StyleFunc: func() core.Style {
									bc := core.Transparent
									bw := core.Points(0)
									if active {
										bc = colors.Accent()
										bw = 2
									}
									return core.Style{
										Width: core.Pt(44), Height: core.Pt(44),
										CornerRadius: 6, Overflow: core.Hidden,
										BorderWidth: bw, BorderColor: bc,
										Background: themeRole(cx, "surfaceAlt"),
										FlexShrink: 0,
									}
								},
								OnPress: func(core.PressEvent) { goToPage(idxCopy) },
								Children: []core.Node{
									core.Image{
										Source:       core.Src(thumbURL),
										ResizeMode:   core.Cover,
										FadeDuration: time.Millisecond,
										Style:        core.Style{Width: core.Pct(100), Height: core.Pct(100)},
									},
									core.View{
										Style: core.Style{
											Position: core.Absolute, Right: core.Pt(2), Bottom: core.Pt(2),
											Padding: core.Symmetric(1, 4), CornerRadius: 3, Background: overlay(),
										},
										Children: []core.Node{
											core.Text{Content: itoa(i + 1), Style: core.Style{FontSize: 9, LineHeight: 11, Color: onPreview()}},
										},
									},
								},
							})
						}
						return core.View{
							TestID: "detail.pageStrip",
							StyleFunc: func() core.Style {
								safe := cx.Env().SafeArea()
								return core.Style{
									Position: core.Absolute, Bottom: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0),
									Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6,
									Padding: core.Edges{
										Top: 8, Bottom: safe.Bottom + 10, Left: safe.Left + 8, Right: safe.Right + 8,
									},
									Background: overlay(),
									ZIndex:     20,
								}
							},
							Children: []core.Node{
								mobileIconButton(cx, mobileIconBtnSpec{
									TestID: "detail.pagePrev", Label: "上一页", Icon: lucide.ChevronLeft, IconSize: 18,
									Variant: mobileIconGhost, Tint: func() core.Color {
										if pageIdx == 0 {
											return onPreviewMuted()
										}
										return onPreview()
									},
									OnPress: func() {
										if pageIdx > 0 {
											goToPage(pageIdx - 1)
										}
									},
								}),
								core.Scroll{
									Horizontal: true,
									Style:      core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
									Children: []core.Node{
										core.View{
											Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6},
											Children: thumbs,
										},
									},
								},
								mobileIconButton(cx, mobileIconBtnSpec{
									TestID: "detail.pageNext", Label: "下一页", Icon: lucide.ChevronRight, IconSize: 18,
									Variant: mobileIconGhost, Tint: func() core.Color {
										if pageIdx >= len(pages)-1 {
											return onPreviewMuted()
										}
										return onPreview()
									},
									OnPress: func() {
										if pageIdx < len(pages)-1 {
											goToPage(pageIdx + 1)
										}
									},
								}),
							},
						}
					},
					Else: func(cx *core.Scope) core.Node {
						return core.View{
							TestID: "detail.pagePill",
							StyleFunc: func() core.Style {
								safe := cx.Env().SafeArea()
								return core.Style{
									Position: core.Absolute, Bottom: core.Pt(safe.Bottom + 16), Left: core.Pt(0), Right: core.Pt(0),
									AlignItems: core.AlignCenter, ZIndex: 19,
								}
							},
							Children: []core.Node{
								core.View{
									StyleFunc: func() core.Style {
										return core.Style{
											Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
											Padding: core.Symmetric(4, 8), CornerRadius: 16,
											Background: overlay(),
										}
									},
									Children: []core.Node{
										core.View{
											TestID: "detail.pagePillPrev",
											Style: core.Style{
												Width: core.Pt(24), Height: core.Pt(24),
												AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
											},
											OnPress: func(core.PressEvent) {
												if pageIdx > 0 {
													goToPage(pageIdx - 1)
												}
											},
											Children: []core.Node{nsui.Icon(lucide.ChevronLeft, 15, func() core.Color {
												if pageIdx == 0 {
													return onPreviewMuted()
												}
												return onPreview()
											})},
										},
										core.View{
											Style: core.Style{
												Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
												Padding: core.Symmetric(2, 4),
											},
											OnPress: func(core.PressEvent) { a.detailChrome.Set(true) },
											Children: []core.Node{
												nsui.Icon(lucide.Layers, 13, onPreview),
												core.Text{
													Content: itoa(pageIdx+1) + " / " + itoa(len(pages)),
													Style:   nsui.TypeCaption.Merge(core.Style{Color: onPreview()}),
												},
											},
										},
										core.View{
											TestID: "detail.pagePillNext",
											Style: core.Style{
												Width: core.Pt(24), Height: core.Pt(24),
												AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
											},
											OnPress: func(core.PressEvent) {
												if pageIdx < len(pages)-1 {
													goToPage(pageIdx + 1)
												}
											},
											Children: []core.Node{nsui.Icon(lucide.ChevronRight, 15, func() core.Color {
												if pageIdx >= len(pages)-1 {
													return onPreviewMuted()
												}
												return onPreview()
											})},
										},
									},
								},
							},
						}
					},
				}
			},
		}

		return core.View{
			StyleFunc: func() core.Style {
				return core.Style{
					Width: core.Pct(100), Height: core.Pct(100),
					Background: colors.Background(),
					Overflow:   core.Hidden,
				}
			},
			Children: []core.Node{
				mediaContent,
				floatingHeader,
				pageBar,
				a.metadataSheet(cx, item),
				a.deleteDialog(cx),
			},
		}
	})
}

func (a *App) nonImageMediaNode(cx *core.Scope, item model.ItemSummary) core.Node {
	colors := nsui.ColorsOf(cx)
	if item.IsVideo() {
		if msg := a.videoError.Get(); msg != "" {
			return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.All(24)}, Children: []core.Node{
				core.Text{Content: "视频无法在应用内播放", Style: nsui.TypeTitle, StyleFunc: nsui.Colored(colors.Text)},
				core.Text{Content: msg, MaxLines: 3, Style: nsui.TypeCaption.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: nsui.Colored(colors.Muted)},
				core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
					mobileActionButton(cx, colors, mobileBtnSpec{
						Text: "下载原视频", Variant: nsui.ButtonSecondary,
						OnPress: func() { a.exportItem(cx, item, false) },
					}),
					mobileActionButton(cx, colors, mobileBtnSpec{
						Text: "用其他应用打开", Variant: nsui.ButtonSecondary,
						OnPress: func() { a.exportItem(cx, item, true) },
					}),
				}},
			}}
		}
		a.ensureVideoCached(cx, item)
		localPath := a.videoLocalPath.Get()
		if localPath == "" {
			return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)}, Children: []core.Node{
				core.Text{Content: "正在缓冲视频...", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Muted)},
			}}
		}
		return core.View{
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Position: core.Relative},
			Children: []core.Node{
				core.Video{
					Source:     "file://" + localPath,
					Controls:   false,
					ResizeMode: core.Contain,
					Ref:        a.videoRef,
					PausedFunc: func() bool {
						return a.screen.Get() != screenDetail ||
							cx.Env().Lifecycle() != core.LifecycleActive ||
							a.videoPaused.Get() ||
							a.metaOpen.Get()
					},
					MutedFunc: func() bool { return a.videoMuted.Get() },
					RateFunc:  func() float32 { return a.videoRate.Get() },
					OnProgress: func(pos, dur time.Duration) {
						a.videoPos.Set(pos)
						if dur > 0 {
							a.videoDur.Set(dur)
						}
					},
					OnError: func(err error) {
						a.videoError.Set("不支持的编码格式或播放失败: " + err.Error())
					},
					Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100)},
					TestID: "detail.video",
				},
				// Gesture overlay: Left (-10s), Center (Play/Pause), Right (+10s & LongPress 2x)
				core.View{
					Style: core.Style{
						Position:  core.Absolute,
						Top:       core.Pt(0),
						Left:      core.Pt(0),
						Right:     core.Pt(0),
						Bottom:    core.Pt(64),
						Direction: core.Row,
					},
					Children: []core.Node{
						core.View{
							Style: core.Style{FlexGrow: 1, Height: core.Pct(100)},
							OnPress: func(core.PressEvent) {
								next := a.videoPos.Get() - 10*time.Second
								if next < 0 {
									next = 0
								}
								a.videoRef.Seek(next)
								a.showToast("后退 10 秒")
							},
						},
						core.View{
							Style: core.Style{FlexGrow: 1, Height: core.Pct(100)},
							OnPress: func(core.PressEvent) {
								a.videoPaused.Set(!a.videoPaused.Get())
							},
						},
						core.View{
							Style: core.Style{FlexGrow: 1, Height: core.Pct(100)},
							OnPress: func(core.PressEvent) {
								next := a.videoPos.Get() + 10*time.Second
								if dur := a.videoDur.Get(); dur > 0 && next > dur {
									next = dur
								}
								a.videoRef.Seek(next)
								a.showToast("前进 10 秒")
							},
							OnLongPress: func(core.PressEvent) {
								if a.videoRate.Get() > 1.5 {
									a.videoRate.Set(1.0)
									a.showToast("恢复 1.0x 正常速度")
								} else {
									a.videoRate.Set(2.0)
									a.showToast("2.0x 快进中")
								}
							},
						},
					},
				},
				// Bottom player control bar: Play/Pause, Progress, Volume/Mute, Speed
				core.View{
					StyleFunc: func() core.Style {
						return core.Style{
							Position:       core.Absolute,
							Left:           core.Pt(12),
							Right:          core.Pt(12),
							Bottom:         core.Pt(16),
							Direction:      core.Row,
							AlignItems:     core.AlignCenter,
							Gap:            10,
							Padding:        core.Symmetric(8, 12),
							CornerRadius:   12,
							Background:     colors.Surface(),
							BorderWidth:    1,
							BorderColor:    colors.Border(),
						}
					},
					Children: []core.Node{
						mobileActionButton(cx, colors, mobileBtnSpec{
							Text:    func() string { if a.videoPaused.Get() { return "播放" }; return "暂停" }(),
							Variant: nsui.ButtonSecondary, Compact: true,
							OnPress: func() { a.videoPaused.Set(!a.videoPaused.Get()) },
						}),
						core.View{
							Style: core.Style{FlexGrow: 1, Gap: 4},
							Children: []core.Node{
								core.View{
									StyleFunc: func() core.Style {
										return core.Style{
											Width:        core.Pct(100),
											Height:       core.Pt(4),
											CornerRadius: 2,
											Background:   colors.Inset(),
										}
									},
									Children: []core.Node{
										core.View{
											StyleFunc: func() core.Style {
												pct := float64(0)
												if dur := a.videoDur.Get(); dur > 0 {
													pct = float64(a.videoPos.Get()) / float64(dur) * 100
													if pct > 100 {
														pct = 100
													}
												}
												return core.Style{
													Width:        core.Pct(float32(pct)),
													Height:       core.Pct(100),
													CornerRadius: 2,
													Background:   colors.Accent(),
												}
											},
										},
									},
								},
								core.Text{
									ContentFunc: func() string {
										return fmt.Sprintf("%s / %s", formatDur(a.videoPos.Get()), formatDur(a.videoDur.Get()))
									},
									Style:     nsui.TypeCaption,
									StyleFunc: nsui.Colored(colors.Muted),
								},
							},
						},
						mobileActionButton(cx, colors, mobileBtnSpec{
							Text:    func() string { if a.videoMuted.Get() { return "静音" }; return "音量" }(),
							Variant: nsui.ButtonSecondary, Compact: true,
							OnPress: func() { a.videoMuted.Set(!a.videoMuted.Get()) },
						}),
						mobileActionButton(cx, colors, mobileBtnSpec{
							Text:    func() string { return fmt.Sprintf("%.1fx", a.videoRate.Get()) }(),
							Variant: nsui.ButtonGhost, Compact: true,
							OnPress: func() {
								if a.videoRate.Get() > 1.5 {
									a.videoRate.Set(1.0)
								} else {
									a.videoRate.Set(2.0)
								}
							},
						}),
					},
				},
			},
		}
	}
	if item.IsAudio() {
		return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.All(24)}, Children: []core.Node{
			core.Text{Content: "音频文件", Style: nsui.TypeTitle, StyleFunc: nsui.Colored(colors.Text)},
			core.Text{Content: item.Filename, MaxLines: 2, Style: nsui.TypeCaption.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: nsui.Colored(colors.Text)},
			core.Text{Content: "当前版本没有内置音频播放器，可下载或分享后用其他应用播放", MaxLines: 3,
				Style: nsui.TypeCaption.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: nsui.Colored(colors.Muted)},
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				mobileActionButton(cx, colors, mobileBtnSpec{
					Text: "下载", Variant: nsui.ButtonSecondary,
					OnPress: func() { a.exportItem(cx, item, false) },
				}),
				mobileActionButton(cx, colors, mobileBtnSpec{
					Text: "分享", Variant: nsui.ButtonSecondary,
					OnPress: func() { a.exportItem(cx, item, true) },
				}),
			}},
		}}
	}
	return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignCenter, Padding: core.All(24)}, Children: []core.Node{
		core.Text{Content: "暂无预览", Style: nsui.TypeTitle, StyleFunc: nsui.Colored(colors.Text)},
		core.Text{Content: item.Filename, MaxLines: 2, Style: nsui.TypeCaption.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: nsui.Colored(colors.Muted)},
		core.Text{Content: "可点击屏幕显示顶部菜单进行下载或分享", Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			mobileActionButton(cx, colors, mobileBtnSpec{
				Text: "下载", Variant: nsui.ButtonSecondary,
				OnPress: func() { a.exportItem(cx, item, false) },
			}),
			mobileActionButton(cx, colors, mobileBtnSpec{
				Text: "分享", Variant: nsui.ButtonSecondary,
				OnPress: func() { a.exportItem(cx, item, true) },
			}),
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
			mobileActionButton(cx, colors, mobileBtnSpec{
				Text: "复制汇总", Variant: nsui.ButtonSecondary, Compact: true,
				OnPress: func() {
					detail := a.detailCache[item.ID]
					cx.SetClipboard(itemInfoText(item, detail))
					a.showToast("已复制 汇总信息")
				},
			}),
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
						copyBtn = mobileIconButton(cx, mobileIconBtnSpec{
							Icon:     lucide.Copy,
							Label:    "复制" + label,
							IconSize: 16,
							Variant:  mobileIconGhost,
							OnPress:  func() { copyField(label, value) },
						})
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
								mobileIconButton(cx, mobileIconBtnSpec{
									Icon: lucide.Copy, Label: "复制主色", IconSize: 16, Variant: mobileIconGhost,
									OnPress: func() { copyField("主色", dc) },
								}),
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
								mobileIconButton(cx, mobileIconBtnSpec{
									Icon: lucide.Copy, Label: "复制调色板", IconSize: 16, Variant: mobileIconGhost,
									OnPress: func() { copyField("调色板", strings.Join(detail.Palette, ", ")) },
								}),
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
	colors := nsui.ColorsOf(cx)
	return nsui.Dialog{
		Open:         func() bool { return a.confirmDel.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.confirmDel.Set(open) },
		Title:        "删除素材？",
		Description:  "「" + a.quickDelTarget.DisplayName() + "」将移到电脑的废纸篓，可在废纸篓中恢复。",
		Footer: []core.Node{
			mobileActionButton(cx, colors, mobileBtnSpec{
				Text: "取消", Variant: nsui.ButtonGhost,
				OnPress: func() { a.confirmDel.Set(false) },
			}),
			mobileActionButton(cx, colors, mobileBtnSpec{
				Text: "移到废纸篓", Variant: nsui.ButtonPrimary,
				OnPress: func() {
					a.confirmDel.Set(false)
					a.deleteItem(cx, a.quickDelTarget)
				},
			}),
		},
	}
}

func (a *App) itemThumbURL(item model.ItemSummary) string {
	if thumb := a.client.MediaURL(item.ThumbnailURL); thumb != "" {
		return thumb
	}
	return a.client.ThumbURL(item.ID)
}

func (a *App) detailReturnScreen() string {
	if a.detailFrom == screenSearch {
		return screenSearch
	}
	return screenGallery
}

func (a *App) backFromDetail() {
	if a.matchStage.Get() != 0 {
		return
	}
	a.videoPaused.Set(true)
	from := a.detailReturnScreen()
	if a.detailIndex.Get() != a.matchOpenedIndex || a.detailPageIndex.Get() != 0 {
		a.cancelPreviewMatch()
		a.screen.Set(from)
		return
	}
	a.startPreviewMatch(false, func() {
		a.screen.Set(from)
	})
}

func (a *App) dismissDetail() {
	a.videoPaused.Set(true)
	a.cancelPreviewMatch()
	a.screen.Set(a.detailReturnScreen())
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

func (a *App) ensureVideoCached(cx *core.Scope, item model.ItemSummary) {
	if a.videoLocalID == item.ID {
		return
	}
	a.videoLocalID = item.ID
	a.videoLocalPath.Set("")
	ext := filepath.Ext(item.Filename)
	if ext == "" {
		ext = ".mp4"
	}
	cachePath := filepath.Join(os.TempDir(), "bowerbird-video-"+item.ID+ext)
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		a.videoLocalPath.Set(cachePath)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		resp, err := a.client.OpenOriginal(ctx, item.ID)
		if err != nil {
			if a.videoLocalID == item.ID {
				a.videoError.Set("视频下载失败: " + err.Error())
			}
			return
		}
		defer resp.Body.Close()
		tmpPath := cachePath + ".part"
		f, err := os.Create(tmpPath)
		if err != nil {
			if a.videoLocalID == item.ID {
				a.videoError.Set("无法创建缓存文件: " + err.Error())
			}
			return
		}
		_, copyErr := io.CopyBuffer(f, resp.Body, make([]byte, 1<<20))
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if a.videoLocalID == item.ID {
				if copyErr != nil {
					a.videoError.Set("视频缓冲失败: " + copyErr.Error())
				} else {
					a.videoError.Set("视频保存失败: " + closeErr.Error())
				}
			}
			return
		}
		_ = os.Rename(tmpPath, cachePath)
		if a.videoLocalID == item.ID {
			a.videoLocalPath.Set(cachePath)
		}
	}()
}

func (a *App) renderPluginUI(cx *core.Scope, item model.ItemSummary, node plugin.UINode, videoSource string) core.Node {
	colors := nsui.ColorsOf(cx)
	switch node.Type {
	case "video":
		src := videoSource
		if src == "" {
			src = node.Source
		}
		return core.View{
			Style: core.Style{Width: core.Pct(100), FlexGrow: 1, Background: colors.Background()},
			Children: []core.Node{
				core.Video{
					Source:     src,
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
				},
			},
		}
	case "title":
		return core.Text{
			Content:   node.Text,
			MaxLines:  2,
			Style:     nsui.TypeTitle,
			StyleFunc: nsui.Colored(colors.Text),
		}
	case "button":
		action := node.Action
		return mobileActionButton(cx, colors, mobileBtnSpec{
			Text:    node.Text,
			Variant: nsui.ButtonSecondary,
			OnPress: func() {
				switch action {
				case "download":
					a.exportItem(cx, item, false)
				case "openExternal":
					a.exportItem(cx, item, true)
				}
			},
		})
	case "row":
		var children []core.Node
		for _, ch := range node.Children {
			children = append(children, a.renderPluginUI(cx, item, ch, videoSource))
		}
		return core.View{
			Style:    core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8},
			Children: children,
		}
	default: // "column"
		var children []core.Node
		for _, ch := range node.Children {
			children = append(children, a.renderPluginUI(cx, item, ch, videoSource))
		}
		return core.View{
			Style:    core.Style{Width: core.Pct(100), Height: core.Pct(100), Gap: 10, Padding: core.Symmetric(12, 12)},
			Children: children,
		}
	}
}

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
}
