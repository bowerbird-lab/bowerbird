package app

import (
	"time"

	"bowerbird/core"
	"bowerbird/internal/player"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// renderVideoControls draws the floating capsule video controller inside the
// preview stage, including hover/drag frame thumbnails, expandable audio
// waveform, volume/rate controls, and the overflow action menu.
func (a *App) renderVideoControls(c *ui.Context, item core.Item, v *player.VideoPlayer) {
	if v == nil {
		return
	}
	p := a.palette(c)
	accentTint := p.accent.Alpha(0.10)
	useMaterial := a.vibrancyShows(c)
	pillBg := p.surface
	if useMaterial {
		pillBg = p.surface.Alpha(0.76)
	}
	pillText := p.text
	pillMuted := p.muted

	var trackBounds ui.Rect
	duration := v.Duration().Seconds()
	position := v.Position().Seconds()

	var volPopActive bool
	ui.Column(c).FillWidth().Padding(4, 20, 14, 20).AlignItems(ui.End).Gap(4).Children(func() {
		if a.videoMoreOpen {
			a.renderVideoMoreMenu(c, item, v, pillBg, pillText, p.hairline, p.accent)
		} else if a.videoRateOpen {
			a.renderVideoRateMenu(c, v, pillBg, pillText, p.hairline, p.accent)
		} else if a.videoVolHover {
			// Vertical volume popover right above the speaker icon (offset from right edge)
			volPopActive = a.renderVideoVolPopover(c, v, pillBg, p.hairline, p.accent)
		}

		displayPos := position
		if a.videoDragging {
			displayPos = a.videoScrub
		}
		remaining := duration - displayPos
		if remaining < 0 {
			remaining = 0
		}

		pillH := float32(56)
		trackBoxH := float32(36)
		waveform := v.AudioWaveform(140)
		targetAnim := float32(0)
		if a.videoDragging {
			targetAnim = 1.0
		} else if a.videoHovering {
			targetAnim = 0.45
		}
		if diff := targetAnim - a.videoTrackAnim; diff > 0.02 || diff < -0.02 {
			a.videoTrackAnim += diff * 0.28
			c.After(16 * time.Millisecond)
		} else {
			a.videoTrackAnim = targetAnim
		}
		anim := a.videoTrackAnim

		var pillRow, muteBtn, rateBtn ui.Element
		pillWrap := ui.Box(c).FillWidth().Height(pillH).Radius(p.pillRadius).Clip()
		if useMaterial {
			pillWrap.Material(glass.Blur{Radius: p.headerBlur})
		}
		pillWrap.Children(func() {
			pillRow = ui.Row(c).Fill().PaddingX(16).Gap(10).AlignItems(ui.Center).Background(pillBg).Border(1, p.hairline).Radius(p.pillRadius)
			pillRow.Children(func() {
			backBtn := videoHoverPill(c, "video-back", 30, 30, 7, "倒退 5 秒", p.accent, func() {
				ui.Icon(c, iconRewind5).Size(16, 16).TextColor(pillText)
			})
			if backBtn.Clicked() {
				_ = v.Seek(v.Position() - 5*time.Second)
			}

			playing := v.Playing()
			playSvg := iconPlay
			playTip := "播放"
			if playing {
				playSvg = iconPause
				playTip = "暂停"
			}
			playBtn := videoHoverPill(c, "video-play", 32, 32, 8, playTip, p.accent, func() {
				ui.Icon(c, playSvg).Size(16, 16).TextColor(pillText)
			})
			if playBtn.Clicked() {
				v.Toggle()
			}

			fwdBtn := videoHoverPill(c, "video-forward", 30, 30, 7, "前进 5 秒", p.accent, func() {
				ui.Icon(c, iconForward5).Size(16, 16).TextColor(pillText)
			})
			if fwdBtn.Clicked() {
				_ = v.Seek(v.Position() + 5*time.Second)
			}

			ui.Text(c, formatDuration(displayPos)).FontSize(11).TextColor(pillMuted).FontFeatures("tnum")

			if duration > 0 {
				if !a.videoDragging && !a.videoPendingSeek {
					a.videoScrub = position
				}
				track := ui.Box(c.Key("video-track")).Grow(1).Height(trackBoxH).Draw(func(pt *ui.Painter, r ui.Rect) {
					ratio := float32(0)
					if duration > 0 {
						ratio = float32(a.videoScrub / duration)
						if ratio < 0 {
							ratio = 0
						}
						if ratio > 1 {
							ratio = 1
						}
					}
					if !a.videoDragging {
						trackH := float32(8) + 4*anim
						ty := r.Y + r.H/2 - trackH/2
						pt.Fill(ui.Rect{X: r.X, Y: ty, W: r.W, H: trackH}, p.accent.Alpha(0.22), trackH/2)
						fillW := r.W * ratio
						if fillW > 0 {
							pt.Clip(ui.Rect{X: r.X, Y: ty, W: r.W, H: trackH}, trackH/2, func() {
								pt.Fill(ui.Rect{X: r.X, Y: ty, W: fillW, H: trackH}, p.accent.Alpha(0.50), 0)
							})
						}
						if a.videoHovering && duration > 0 && anim > 0.05 {
							hRatio := float32(a.videoHoverPos / duration)
							if hRatio < 0 {
								hRatio = 0
							}
							if hRatio > 1 {
								hRatio = 1
							}
							hx := r.X + r.W*hRatio
							halfSpan := float32(28) * (anim / 0.45)
							leftX := hx - halfSpan
							rightX := hx + halfSpan
							if leftX < r.X {
								leftX = r.X
							}
							if rightX > r.X+r.W {
								rightX = r.X + r.W
							}
							// Expanded hover zone + 10% accent tint + accent left/right focus border markers
							focusH := trackH + 6
							fy := r.Y + r.H/2 - focusH/2
							pt.Fill(ui.Rect{X: leftX, Y: fy, W: rightX - leftX, H: focusH}, accentTint, 4)
							pt.Fill(ui.Rect{X: leftX, Y: fy - 2, W: 2, H: focusH + 4}, p.accent, 1)
							pt.Fill(ui.Rect{X: rightX - 2, Y: fy - 2, W: 2, H: focusH + 4}, p.accent, 1)
						}
						return
					}
					n := len(waveform)
					if n == 0 {
						n = 140
					}
					slotW := r.W / float32(n)
					barW := float32(2)
					if slotW < 3 {
						barW = slotW * 0.55
					}
					maxH := (r.H - 6) * 0.6
					for i := 0; i < n; i++ {
						amp := float32(0.03)
						if i < len(waveform) {
							amp = waveform[i]
						}
						posNorm := float32(i) / float32(n)
						dist := posNorm - ratio
						if dist < 0 {
							dist = -dist
						}
						// Stronger local magnification around the active cursor
						boost := float32(1.0)
						if dist < 0.09 {
							t := 1.0 - dist/0.09
							boost = 1.0 + 1.15*t*t*anim
						}
						bh := maxH * amp * boost
						if bh > r.H-4 {
							bh = r.H - 4
						}
						if bh < 1.5 {
							bh = 1.5
						}
						bx := r.X + float32(i)*slotW + (slotW-barW)/2
						by := r.Y + (r.H-bh)/2
						col := p.accent.Alpha(0.24)
						if posNorm <= ratio {
							col = p.accent.Alpha(0.50)
						}
						if dist < 0.04 {
							col = p.accent
						}
						pt.Fill(ui.Rect{X: bx, Y: by, W: barW, H: bh}, col, barW/2)
					}
					// Left/right focus brackets & center playhead in theme accent color
					tx := r.X + r.W*ratio
					span := float32(32) * anim
					lx := tx - span
					rx := tx + span
					if lx < r.X {
						lx = r.X
					}
					if rx > r.X+r.W {
						rx = r.X + r.W
					}
					pt.Fill(ui.Rect{X: lx, Y: r.Y + 4, W: rx - lx, H: r.H - 8}, accentTint, 4)
					pt.Fill(ui.Rect{X: lx, Y: r.Y + 4, W: 1.5, H: r.H - 8}, p.accent, 1)
					pt.Fill(ui.Rect{X: rx - 1.5, Y: r.Y + 4, W: 1.5, H: r.H - 8}, p.accent, 1)
					pt.Fill(ui.Rect{X: tx - 1, Y: r.Y + 2, W: 2, H: r.H - 4}, p.accent, 1)
				})
				trackBounds = track.Bounds()
				a.videoTrackRect = trackBounds
				a.videoHovering = false
				if track.Pressed() {
					wasDragging := a.videoDragging
					a.videoDragging = true
					if lx, _, ok := track.PointerPosition(); ok && trackBounds.W > 0 {
						ratio := float64(lx) / float64(trackBounds.W)
						if ratio < 0 {
							ratio = 0
						}
						if ratio > 1 {
							ratio = 1
						}
						target := ratio * duration
						if !wasDragging || target != a.videoScrub {
							a.videoScrub = target
							_ = v.Seek(time.Duration(a.videoScrub * float64(time.Second)))
						}
					}
				} else {
					a.videoDragging = false
					a.videoPendingSeek = false
					if track.Hovered() {
						if lx, _, ok := track.PointerPosition(); ok && trackBounds.W > 0 {
							ratio := float64(lx) / float64(trackBounds.W)
							if ratio < 0 {
								ratio = 0
							}
							if ratio > 1 {
								ratio = 1
							}
							a.videoHovering = true
							a.videoHoverPos = ratio * duration
						}
					}
				}
			} else {
				ui.Box(c).Grow(1)
			}

			ui.Text(c, "-"+formatDuration(remaining)).FontSize(11).TextColor(pillMuted).FontFeatures("tnum")

			volSvg := iconVolume
			volTip := "静音"
			if v.Muted() || v.Volume() == 0 {
				volSvg = iconMuted
				volTip = "取消静音"
			}
			muteBtn = videoHoverPill(c, "video-mute", 28, 28, 6, volTip, p.accent, func() {
				ui.Icon(c, volSvg).Size(15, 15).TextColor(pillText)
			})
			if muteBtn.Clicked() {
				v.SetMuted(!v.Muted())
			}
			if muteBtn.Hovered() || volPopActive {
				a.videoVolHover = true
				a.videoVolLeaveAt = time.Now()
			} else if a.videoVolHover {
				if time.Since(a.videoVolLeaveAt) > 250*time.Millisecond {
					a.videoVolHover = false
				} else {
					c.After(60 * time.Millisecond)
				}
			}

			rate := v.Rate()
			rateLabel := formatRateLabel(rate)
			rateBtn = ui.Row(c.Key("video-rate")).Height(28).PaddingX(8).Radius(p.pillRadius).AlignItems(ui.Center).Tooltip("倍速播放").Draw(func(pt *ui.Painter, r ui.Rect) {
				if rateBtn.Hovered() || a.videoRateOpen {
					pt.Fill(r, p.accent.Alpha(0.14), r.H/2)
				}
			}).Children(func() {
				ui.Text(c, rateLabel).FontSize(11.5).TextColor(pillText).FontFeatures("tnum")
			})
			if rateBtn.Clicked() {
				a.videoRateOpen = !a.videoRateOpen
				if a.videoRateOpen {
					a.videoMoreOpen = false
					a.videoVolHover = false
				}
			}

			fullSvg := iconFullscreen
			fullTip := "全屏"
			if a.win != nil && a.win.IsFullScreen() {
				fullSvg = iconExitFullscreen
				fullTip = "退出全屏"
			}
			fullBtn := videoHoverPill(c, "video-fullscreen", 30, 30, 7, fullTip, p.accent, func() {
				ui.Icon(c, fullSvg).Size(15, 15).TextColor(pillText)
			})
			if fullBtn.Clicked() && a.win != nil {
				a.win.ToggleFullScreen()
			}

			moreBtn := videoHoverPill(c, "video-more", 30, 30, 7, "更多", p.accent, func() {
				ui.Icon(c, iconMore).Size(16, 16).TextColor(pillText)
			})
			if moreBtn.Clicked() {
				a.videoMoreOpen = !a.videoMoreOpen
				if a.videoMoreOpen {
					a.videoRateOpen = false
					a.videoVolHover = false
				}
			}
			})
		})
		if pr, mb := pillRow.Bounds(), muteBtn.Bounds(); pr.W > 0 && mb.W > 0 {
			pad := (pr.X + pr.W) - (mb.X + mb.W/2) - 17
			if pad >= 0 {
				a.videoVolRightPad = pad
			}
		}
		if pr, rb := pillRow.Bounds(), rateBtn.Bounds(); pr.W > 0 && rb.W > 0 {
			pad := (pr.X + pr.W) - (rb.X + rb.W/2) - 42
			if pad >= 0 {
				a.videoRateRightPad = pad
			}
		}
	})
}

func (a *App) renderVideoMoreMenu(c *ui.Context, item core.Item, v *player.VideoPlayer, bg, textCol, borderCol, accent ui.Color) {
	p := a.palette(c)
	wrap := ui.Box(c).MinWidth(164).Radius(p.menuRadius).Clip()
	if a.vibrancyShows(c) {
		wrap.Material(glass.Blur{Radius: p.headerBlur})
	}
	wrap.Children(func() {
		ui.Column(c).FillWidth().Padding(6, 6, 6, 6).Gap(2).Background(bg).Border(1, borderCol).Radius(p.menuRadius).Children(func() {
			if videoMenuItem(c, "video-restart", "回到开头", textCol, accent) {
				_ = v.Seek(0)
				a.videoMoreOpen = false
			}
			if videoMenuItem(c, "video-open", a.t("contextMenu.openWithDefaultApp"), textCol, accent) {
				_ = a.svc.OpenWithDefaultApp(item.ID)
				a.videoMoreOpen = false
			}
			if videoMenuItem(c, "video-reveal", "在文件夹中打开", textCol, accent) {
				_ = a.svc.RevealInFinder(item.ID)
				a.videoMoreOpen = false
			}
		})
	})
}

func videoMenuItem(c *ui.Context, key, label string, textCol, accent ui.Color) bool {
	var row ui.Element
	row = ui.Row(c.Key(key)).FillWidth().Height(30).PaddingX(10).Radius(8).AlignItems(ui.Center).Draw(func(pt *ui.Painter, r ui.Rect) {
		if row.Hovered() {
			pt.Fill(r, accent.Alpha(0.14), 8)
		}
	}).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(textCol)
	})
	return row.Clicked()
}

func (a *App) renderVideoVolPopover(c *ui.Context, v *player.VideoPlayer, bg, borderCol, accent ui.Color) bool {
	vol := v.Volume()
	if v.Muted() {
		vol = 0
	}
	rightPad := a.videoVolRightPad
	if rightPad <= 0 {
		rightPad = 76
	}
	var hovered, pressed bool
	p := a.palette(c)
	ui.Row(c).Padding(0, rightPad, 0, 0).Children(func() {
		wrap := ui.Box(c).Width(34).Height(112).Radius(p.menuRadius).Clip()
		if a.vibrancyShows(c) {
			wrap.Material(glass.Blur{Radius: p.headerBlur})
		}
		var pop ui.Element
		wrap.Children(func() {
			pop = ui.Column(c.Key("video-vol-pop")).Fill().Padding(10, 10, 10, 10).AlignItems(ui.Center).Background(bg).Border(1, borderCol).Radius(p.menuRadius)
			pop.Children(func() {
				track := ui.Box(c.Key("video-vol-track")).Width(14).Grow(1).Draw(func(pt *ui.Painter, r ui.Rect) {
					barW := float32(6)
					bx := r.X + (r.W-barW)/2
					pt.Fill(ui.Rect{X: bx, Y: r.Y, W: barW, H: r.H}, accent.Alpha(0.24), barW/2)
					fillH := r.H * float32(vol)
					if fillH > 0 {
						pt.Clip(ui.Rect{X: bx, Y: r.Y, W: barW, H: r.H}, barW/2, func() {
							pt.Fill(ui.Rect{X: bx, Y: r.Y + r.H - fillH, W: barW, H: fillH}, accent, 0)
						})
					}
					knobR := float32(5)
					ky := r.Y + r.H - fillH
					if ky < r.Y+knobR {
						ky = r.Y + knobR
					}
					if ky > r.Y+r.H-knobR {
						ky = r.Y + r.H - knobR
					}
					pt.Fill(ui.Rect{X: r.X + r.W/2 - knobR, Y: ky - knobR, W: knobR * 2, H: knobR * 2}, accent, knobR)
				})
				if track.Pressed() {
					pressed = true
					tb := track.Bounds()
					if _, ly, ok := track.PointerPosition(); ok && tb.H > 0 {
						next := 1.0 - float64(ly)/float64(tb.H)
						if next < 0 {
							next = 0
						}
						if next > 1 {
							next = 1
						}
						v.SetVolume(next)
					}
				}
				if track.Hovered() {
					hovered = true
				}
			})
		})
		if pop.Hovered() {
			hovered = true
		}
	})
	return hovered || pressed
}

func formatRateLabel(rate float64) string {
	switch {
	case rate >= 7.5:
		return "8x"
	case rate >= 3.5:
		return "4x"
	case rate >= 2.5:
		return "3x"
	case rate >= 1.9:
		return "2x"
	case rate >= 1.4:
		return "1.5x"
	case rate >= 1.2:
		return "1.25x"
	case rate >= 0.9:
		return "1x"
	case rate >= 0.7:
		return "0.75x"
	case rate >= 0.45:
		return "0.5x"
	default:
		return "0.25x"
	}
}

func (a *App) renderVideoRateMenu(c *ui.Context, v *player.VideoPlayer, bg, textCol, borderCol, accent ui.Color) {
	rightPad := a.videoRateRightPad
	if rightPad < 0 {
		rightPad = 36
	}
	rates := []struct {
		label string
		val   float64
	}{
		{"8x", 8.0},
		{"4x", 4.0},
		{"3x", 3.0},
		{"2x", 2.0},
		{"1.5x", 1.5},
		{"1.25x", 1.25},
		{"1x", 1.0},
		{"0.75x", 0.75},
		{"0.5x", 0.5},
		{"0.25x", 0.25},
	}
	cur := v.Rate()
	p := a.palette(c)
	ui.Row(c).Padding(0, rightPad, 0, 0).Children(func() {
		wrap := ui.Box(c).Width(84).Radius(p.menuRadius).Clip()
		if a.vibrancyShows(c) {
			wrap.Material(glass.Blur{Radius: p.headerBlur})
		}
		wrap.Children(func() {
			ui.Column(c).FillWidth().Padding(6, 6, 6, 6).Gap(2).Background(bg).Border(1, borderCol).Radius(p.menuRadius).Children(func() {
				for _, r := range rates {
					col := textCol
					if cur >= r.val-0.05 && cur <= r.val+0.05 {
						col = accent
					}
					if videoMenuItem(c, "video-rate-"+r.label, r.label, col, accent) {
						v.SetRate(r.val)
						a.videoRateOpen = false
					}
				}
			})
		})
	})
}

func videoHoverPill(c *ui.Context, key string, w, h, padX float32, tip string, accent ui.Color, body func()) ui.Element {
	var el ui.Element
	el = ui.Row(c.Key(key)).Size(w, h).Radius(999).PaddingX(padX).AlignItems(ui.Center).Tooltip(tip).Draw(func(pt *ui.Painter, r ui.Rect) {
		if el.Hovered() {
			pt.Fill(r, accent.Alpha(0.14), r.H/2)
		}
	}).Children(body)
	return el
}
