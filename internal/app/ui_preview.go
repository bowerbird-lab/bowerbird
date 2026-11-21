package app

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"bowerbird/core"
	"bowerbird/internal/player"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// The preview's zoom range. The header's drag bar maps it logarithmically so
// fine adjustments around 100% stay easy.
const (
	previewMinZoom float32 = 0.2
	previewMaxZoom float32 = 8
)

// previewZoomTrack places zoom on the drag bar's 0–100 range.
func previewZoomTrack(zoom float32) float64 {
	zoom = minF(maxF(zoom, previewMinZoom), previewMaxZoom)
	return math.Log(float64(zoom/previewMinZoom)) / math.Log(float64(previewMaxZoom/previewMinZoom)) * 100
}

// previewZoomFromTrack turns a drag bar position back into a zoom scale.
func previewZoomFromTrack(track float64) float32 {
	track = math.Max(0, math.Min(100, track))
	return float32(float64(previewMinZoom) * math.Pow(float64(previewMaxZoom/previewMinZoom), track/100))
}

// viewPreview is the full-content Quick Look layer. Images render as GPU
// textures with pinch/scroll zoom and pointer panning; Markdown uses the native
// rich text view; audio uses the Go playback kernel; unsupported formats show
// their metadata and open in the default app.
func (a *App) viewPreview(c *ui.Context) {
	p := a.palette(c)
	if len(a.items) == 0 {
		a.previewOpen = false
		return
	}
	if a.previewIndex < 0 || a.previewIndex >= len(a.items) {
		a.previewIndex = 0
	}
	item := a.workspaceItem(a.previewIndex)
	a.activeItem = &item
	ui.Column(c).Fill().Background(ui.RGBA(0, 0, 0, 0.0)).Children(func() {
		a.viewPreviewToolbar(c)
		if a.previewMode != "full" {
			ui.Box(c).FillWidth().Height(1).Background(p.hairline)
		}
		a.viewPreviewBody(c, item)
	})
}

func (a *App) viewPreviewToolbar(c *ui.Context) {
	p := a.palette(c)
	if a.previewIndex < 0 || a.previewIndex >= len(a.items) {
		return
	}
	item := a.workspaceItem(a.previewIndex)

	full := a.previewMode == "full"
	closeButton := func() {
		a.iconButton(c, "preview-close", iconClose, false, a.t("common.close"), a.closePreview)
	}

	// Both preview modes build the same header component. The inline preview
	// keeps the workspace chrome; the full-window overlay hides the sidebar
	// toggle, the title, the navigation and the zoom controls, leaving only the
	// close button at the far right.
	a.commonHeader(c, commonHeaderOptions{
		spansWindow:       full,
		pad:               gallerySidePadding,
		height:            headerHeight,
		hideSidebarToggle: full,
	}, func() {
		if full {
			ui.Box(c).Grow(1)
			closeButton()
			return
		}

		// The step buttons stay at the left end of the row, as the history
		// buttons do in the workspace header.
		a.iconButton(c, "preview-prev", iconBack, false, a.t("preview.previous"), func() { a.movePreview(-1) })
		a.iconButton(c, "preview-next", iconForward, false, a.t("preview.next"), func() { a.movePreview(1) })
		ui.Text(c, item.Name).FontSize(13).FontWeight(600).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		ui.Textf(c, "%d / %d", a.previewIndex+1, len(a.items)).FontSize(11).TextColor(p.faint).FontFeatures("tnum")
		ui.Box(c).Width(1).Height(18).Background(p.hairline)
		// Zoom uses the same kind of drag bar as the gallery, mapped on a log
		// scale so the usual 50–200% range gets most of the track.
		zoomTrack := previewZoomTrack(a.previewZoom)
		ui.Slider(c.Key("preview-zoom-slider"), &zoomTrack, 0, 100).Width(110)
		if z := previewZoomFromTrack(zoomTrack); math.Abs(float64(z-a.previewZoom)) > 0.001 {
			a.previewZoom = z
		}
		ui.Textf(c, "%d%%", int(math.Round(float64(a.previewZoom*100)))).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum").Width(40).TextAlign(ui.Center)
		a.iconButton(c, "preview-fit", iconGrid, false, a.t("preview.fit"), func() {
			a.previewZoom = 1
			a.previewPan = ui.Point{}
		})
		a.iconButton(c, "preview-open", iconExternal, false, a.t("contextMenu.openWithDefaultApp"), func() {
			_ = a.svc.OpenWithDefaultApp(item.ID)
		})
		ui.Box(c).Width(1).Height(18).Background(p.hairline)
		// Closing is the last action on the row, so it never moves when the
		// zoom and navigation controls change width.
		closeButton()
	}, nil)
}

func (a *App) viewPreviewBody(c *ui.Context, item core.Item) {
	p := a.palette(c)
	ui.Column(c).Grow(1).Shrink(1).FillWidth().Children(func() {
		ext := strings.ToLower(item.Extension)
		switch {
		case ext == "psd" || ext == "psb":
			if a.nativeFeature("official.psd-preview") {
				a.previewPSD(c, item)
			} else {
				a.previewDocument(c, item)
			}
		case ext == "hsj":
			if a.nativeFeature("official.hsj-preview") {
				a.previewPSD(c, item)
			} else {
				a.previewDocument(c, item)
			}
		case ext == "ipv":
			if a.nativeFeature("official.ipv-preview") {
				a.previewPSD(c, item)
			} else {
				a.previewDocument(c, item)
			}
		case categoryOf(item) == "image":
			a.previewImage(c, item)
		case categoryOf(item) == "audio":
			if a.nativeFeature("official.audio-player") {
				a.previewAudio(c, item)
			} else {
				a.previewDocument(c, item)
			}
		case categoryOf(item) == "video":
			if a.nativeFeature("builtin.video") {
				a.previewVideo(c, item)
			} else {
				a.previewDocument(c, item)
			}
		default:
			a.previewDocument(c, item)
		}
		// Film strip of the current page. It takes the width the window has
		// and shows as many cells as fit in it, so a wide window offers more
		// neighbours and a narrow one fewer, the current item staying centred.
		if len(a.items) > 1 && !a.isImmersiveFullscreen() {
			ui.Box(c).FillWidth().Height(1).Background(p.hairline)
			strip := ui.Row(c).FillWidth().Height(64).PaddingX(stripPaddingX).Gap(stripGap).AlignItems(ui.Center).Justify(ui.Center).Bind(&a.stripBox)
			strip.Children(func() {
				start, end := a.stripRange(c)
				a.beginFilmstripLoad(start, end)
				ready, waiting, failed, missing := 0, 0, 0, 0
				for i := start; i < end; i++ {
					i := i
					it := a.workspaceItem(i)
					box := ui.ButtonBase(c.Key(fmt.Sprintf("strip-%s", it.ID))).Width(stripThumbWidth).Height(stripThumbHeight).Radius(6).Background(p.surface2).Clip().FocusRing(false).KeepFocus()
					if i == a.previewIndex {
						box.Border(2, p.accent)
					}
					path := ""
					if it.HasThumbnail && it.ItemPath != "" {
						path = filepath.Join(it.ItemPath, "thumbnail.jpg")
					}
					var bmp *ui.Bitmap
					var err error
					if path == "" {
						missing++
					} else {
						bmp, err = a.filmstripBitmap(it, i, path)
						if err != nil {
							failed++
						} else if bmp != nil {
							ready++
						} else {
							waiting++
						}
					}
					box.Children(func() {
						if bmp != nil {
							ui.Image(c, bmp).Fill().Fit(ui.Cover)
						} else {
							ui.Icon(c, categoryIcon(it)).Size(16, 16).TextColor(p.faint)
						}
					})
					if box.Clicked() {
						a.previewIndex = i
						a.activeItem = &a.items[i]
						a.activeItemID = it.ID
						a.previewZoom = 1
						a.previewPan = ui.Point{}
						a.loadPreviewContent()
					}
				}
				a.recordFilmstripLoad(c, ready, waiting, failed, missing)
			})
		}
	})
}

// Film strip geometry: one cell's size, the gap between cells and the
// padding on either end. The strip itself spans the whole width.
const (
	stripThumbWidth  = 52
	stripThumbHeight = 48
	stripGap         = 6
	stripPaddingX    = 12
)

// stripFitCount returns how many strip cells fit in width.
func stripFitCount(width float32) int {
	n := int((width - 2*stripPaddingX + stripGap) / (stripThumbWidth + stripGap))
	if n < 1 {
		return 1
	}
	return n
}

// stripWindow picks the slice of cells to show around index: fit cells,
// centred on index where the ends allow it.
func stripWindow(index, total, fit int) (start, end int) {
	if fit < 1 {
		fit = 1
	}
	if fit > total {
		fit = total
	}
	start = index - fit/2
	if start < 0 {
		start = 0
	}
	if start+fit > total {
		start = total - fit
	}
	return start, start + fit
}

// stripRange is the strip window for the laid-out width. The box comes from
// the previous frame; the window size stands in until the first layout.
func (a *App) stripRange(c *ui.Context) (start, end int) {
	width := a.stripBox.Bounds(c).W
	if width <= 0 {
		width, _ = c.Size()
	}
	return stripWindow(a.previewIndex, len(a.items), min(a.filmstripImages.limit, stripFitCount(width)))
}

func (a *App) handlePreviewImageInput(ev ui.InputEvent) bool {
	if a.previewError != "" && ev.Kind != ui.InputKeyDown {
		return false
	}
	previewMods, previewKey, previewOK := a.shortcutKeys(core.ShortcutTogglePreview)
	dismissMods, dismissKey, dismissOK := a.shortcutKeys(core.ShortcutDismiss)
	switch ev.Kind {
	case ui.InputKeyDown:
		if (previewOK && ev.Mods == previewMods && ev.Key == previewKey) ||
			(dismissOK && ev.Mods == dismissMods && ev.Key == dismissKey) {
			a.closePreview()
			return true
		}
	case ui.InputZoom:
		a.previewZoom = minF(maxF(a.previewZoom*ev.Scale, previewMinZoom), previewMaxZoom)
		return true
	case ui.InputScroll:
		if ev.Mods&ui.Cmd != 0 || ev.Mods&ui.Ctrl != 0 {
			factor := float32(1) - ev.DY*0.01
			a.previewZoom = minF(maxF(a.previewZoom*factor, previewMinZoom), previewMaxZoom)
		} else {
			a.previewPan.X -= ev.DX
			a.previewPan.Y -= ev.DY
		}
		return true
	case ui.InputPointerDown:
		a.previewPanning = true
		a.previewLastX, a.previewLastY = ev.X, ev.Y
		return true
	case ui.InputPointerMove:
		if a.previewPanning {
			a.previewPan.X += ev.X - a.previewLastX
			a.previewPan.Y += ev.Y - a.previewLastY
			a.previewLastX, a.previewLastY = ev.X, ev.Y
		}
		return true
	case ui.InputPointerUp:
		a.previewPanning = false
		return true
	}
	return false
}

func (a *App) drawPreviewImageCanvas(c *ui.Context, bmp *ui.Bitmap) {
	p := a.palette(c)
	ui.Box(c).Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
		// Soft checkerboard behind transparent images.
		cell := float32(14)
		for y := float32(0); y < r.H; y += cell {
			for x := float32(0); x < r.W; x += cell {
				if (int(x/cell)+int(y/cell))%2 == 0 {
					pt.Fill(ui.Rect{X: r.X + x, Y: r.Y + y, W: cell, H: cell}, p.surface.Alpha(0.25), 0)
				}
			}
		}
	}).Children(func() {
		ui.Image(c, bmp).Fill().Fit(ui.Contain).Scale(a.previewZoom, a.previewZoom).Translate(a.previewPan.X, a.previewPan.Y)
	})
}

func (a *App) previewImage(c *ui.Context, item core.Item) {
	ui.Box(c).Fill().Grow(1).Shrink(1).Clip().HandleInput(a.handlePreviewImageInput).Children(func() {
		var bmp *ui.Bitmap
		if a.previewError == "" {
			path := item.FilePath
			if path == "" && item.HasThumbnail && item.ItemPath != "" {
				path = filepath.Join(item.ItemPath, "thumbnail.jpg")
			}
			var err error
			bmp, err = a.previewImages.getResult(path, 4096, a.previewTrace)
			if err != nil {
				a.previewError = err.Error()
				if a.previewTrace != nil {
					a.previewTrace.record("error", "display_failed", a.previewTrace.started, fmt.Sprintf("error=%q", err))
				}
			}
		}
		if a.previewError != "" {
			a.previewImageFailure(c)
			return
		}
		if bmp == nil {
			if trace := a.previewTrace; trace != nil && !a.previewSlowLogged {
				elapsed := time.Since(trace.started)
				if elapsed >= 2*time.Second {
					a.previewSlowLogged = true
					a.previewImages.mu.Lock()
					active, queued := a.previewImages.active, len(a.previewImages.queue)
					a.previewImages.mu.Unlock()
					trace.record("warn", "waiting", trace.started, fmt.Sprintf("active=%d queued=%d", active, queued))
				} else {
					c.After(2*time.Second - elapsed)
				}
			}
			ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
			return
		}
		if !a.previewImageShown {
			a.previewImageShown = true
			if trace := a.previewTrace; trace != nil {
				width, height := bmp.Size()
				trace.record("info", "display_ready", trace.started, fmt.Sprintf("width=%d height=%d", width, height))
			}
		}
		a.drawPreviewImageCanvas(c, bmp)
	})
}

func (a *App) previewImageFailure(c *ui.Context) {
	p := a.palette(c)
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		ui.Icon(c, iconImage).Size(34, 34).TextColor(p.faint)
		ui.Text(c, a.t("preview.imageLoadFailed")).FontSize(14).TextColor(p.text)
		ui.Text(c, a.previewError).FontSize(12).TextColor(p.muted).MaxWidth(480)
		if ui.Button(c.Key("preview-image-retry"), a.t("preview.retry")).Clicked() {
			a.loadPreviewContent()
		}
	})
}

func (a *App) previewAudio(c *ui.Context, item core.Item) {
	p := a.palette(c)
	if a.player == nil && item.FilePath != "" {
		// Do not autoplay; opening is explicit on the play button.
	}
	ui.Column(c).Fill().Grow(1).Center().Gap(18).Children(func() {
		box := ui.Box(c).Size(160, 160).Radius(24).Background(p.surface2).Center().Children(func() {
			ui.Icon(c, iconAudio).Size(64, 64).TextColor(p.muted)
		})
		ui.Box(c).Width(1).Height(1).Children(func() { _ = box })
		ui.Text(c, item.Name).FontSize(17).FontWeight(650).TextColor(p.text).MaxWidth(480).SingleLine()
		// Spectrum bars animate while playing; source is the playhead.
		bars := ui.Row(c).Width(360).Height(54).Gap(3).AlignItems(ui.End).Children
		bars(func() {
			pos := float64(0)
			playing := false
			if a.player != nil {
				pos = a.player.Position().Seconds()
				playing = a.player.Playing()
			}
			for i := 0; i < 36; i++ {
				h := float32(6)
				if playing {
					h = 6 + float32(30*math.Abs(math.Sin(float64(i)*0.7+pos*3)))
				}
				ui.Box(c).Grow(1).Height(h).Radius(2).Background(p.accent.Alpha(0.75))
			}
		})
		ui.Row(c).Width(360).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c.Key("audio-play"), map[bool]string{true: "❚❚", false: "▶"}[a.player != nil && a.player.Playing()]).Size(42, 42).Radius(999).Clicked() {
				if a.player == nil {
					go func() {
						pl, err := playerOpen(item.FilePath)
						a.post(func() {
							if err != nil {
								a.showToast(err.Error())
								return
							}
							a.player = pl
							pl.Play()
						})
					}()
				} else {
					a.player.Toggle()
				}
			}
			dur, pos := 0.0, 0.0
			if a.player != nil {
				dur = a.player.Duration().Seconds()
				pos = a.player.Position().Seconds()
			}
			ui.Text(c, formatDuration(pos)).FontSize(10).TextColor(p.faint).FontFeatures("tnum")
			prog := ui.Box(c).Grow(1).Height(6).Radius(3).Background(p.surface2)
			prog.Children(func() {
				if dur > 0 {
					frac := float32(pos / dur)
					ui.Box(c).Width(360 * frac).Height(6).Radius(3).Background(p.accent)
				}
			})
			ui.Text(c, formatDuration(dur)).FontSize(10).TextColor(p.faint).FontFeatures("tnum")
		})
		ui.Text(c, a.t("preview.audioHint")).FontSize(11).TextColor(p.faint)
	})
	if a.player != nil && a.player.Playing() {
		c.After(80 * time.Millisecond)
	}
}

func (a *App) previewVideo(c *ui.Context, item core.Item) {
	p := a.palette(c)
	if a.video == nil {
		ui.Column(c).Fill().Grow(1).Center().Gap(14).Children(func() {
			ui.Box(c).Size(120, 120).Radius(22).Background(p.surface2).Center().Children(func() {
				if a.videoErr != "" {
					ui.Icon(c, iconVideo).Size(48, 48).TextColor(p.faint)
				} else {
					ui.Spinner(c)
				}
			})
			ui.Text(c, item.Name).FontSize(16).FontWeight(650).TextColor(p.text)
			if a.videoErr != "" {
				ui.Text(c, a.videoErr).FontSize(11.5).TextColor(p.muted).MaxWidth(520).Wrap().TextAlign(ui.Center)
			} else {
				ui.Text(c, a.t("preview.videoLoading")).FontSize(11.5).TextColor(p.muted)
			}
			if ui.PrimaryButton(c.Key("video-open"), a.t("contextMenu.openWithDefaultApp")).Clicked() {
				_ = a.svc.OpenWithDefaultApp(item.ID)
			}
		})
		return
	}

	stage := ui.Box(c.Key("video-stage")).Fill().Grow(1).Clip().DrawOver(func(pt *ui.Painter, r ui.Rect) {
		if (!a.videoDragging && !a.videoHovering) || a.video == nil {
			return
		}
		dur := a.video.Duration().Seconds()
		if dur <= 0 {
			return
		}
		previewSec := a.videoHoverPos
		if a.videoDragging {
			previewSec = a.videoScrub
		}
		thumb := a.video.FrameThumbnail(time.Duration(previewSec * float64(time.Second)))
		if thumb == nil {
			return
		}
		ratio := float32(previewSec / dur)
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		cardW := float32(104)
		cardH := float32(62)
		tb := a.videoTrackRect
		if tb.W <= 0 {
			tb = r
		}
		cx := tb.X + tb.W*ratio - cardW/2
		if cx < r.X+8 {
			cx = r.X + 8
		}
		if cx+cardW > r.X+r.W-8 {
			cx = r.X + r.W - 8 - cardW
		}
		cy := r.Y + r.H - cardH - 76
		if cy < r.Y+8 {
			cy = r.Y + 8
		}
		thumbBg := p.surface
		if a.vibrancyShows(c) {
			thumbBg = p.surface.Alpha(0.85)
		}
		pt.Fill(ui.Rect{X: cx, Y: cy, W: cardW, H: cardH}, thumbBg, 8)
		pt.Stroke(ui.Rect{X: cx, Y: cy, W: cardW, H: cardH}, p.hairline, 8, 1)
		pt.Clip(ui.Rect{X: cx + 4, Y: cy + 4, W: 96, H: 54}, 5, func() {
			pt.Image(thumb, ui.Rect{X: cx + 4, Y: cy + 4, W: 96, H: 54}, ui.Cover)
		})
	}).Children(func() {
		ui.Video(c, a.video.Native()).Fill().Fit(ui.Contain)
		showControls := !a.videoControlsHidden && (!a.video.Playing() || a.videoDragging || a.videoHovering || a.videoMoreOpen || a.videoVolHover || a.videoRateOpen || time.Since(a.videoLastMove) < 2*time.Second)
		if showControls {
			ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).ZIndex(10).Children(func() {
				a.renderVideoControls(c, item, a.video)
			})
			if a.video.Playing() && !a.videoDragging && !a.videoHovering && !a.videoMoreOpen && !a.videoVolHover && !a.videoRateOpen {
				c.After(250 * time.Millisecond)
			}
		}
	})
	if stage.Clicked() && !a.videoDragging {
		a.videoControlsHidden = !a.videoControlsHidden
		if !a.videoControlsHidden {
			a.videoLastMove = time.Now()
		}
	}
	if stage.Hovered() || a.videoDragging {
		if px, py, ok := stage.PointerPosition(); ok {
			if px != a.videoLastPtrX || py != a.videoLastPtrY {
				a.videoLastPtrX = px
				a.videoLastPtrY = py
				a.videoLastMove = time.Now()
				a.videoControlsHidden = false
			}
		}
	}
	if a.videoDragging || a.videoHovering || a.videoMoreOpen || a.videoVolHover || a.videoRateOpen {
		a.videoLastMove = time.Now()
		a.videoControlsHidden = false
	}
	if a.video.Playing() {
		c.After(100 * time.Millisecond)
	}
}

func (a *App) previewDocument(c *ui.Context, item core.Item) {
	p := a.palette(c)
	ext := strings.ToLower(item.Extension)
	isText := ext == "md" || ext == "markdown" || ext == "txt" || ext == "json" || ext == "js" || ext == "ts" || ext == "go" || ext == "py" || ext == "prompt"
	if isText && a.previewText != "" {
		ui.Scroll(c).Fill().Grow(1).Shrink(1).Padding(28, 48, 28, 48).Children(func() {
			if (ext == "md" || ext == "markdown") && a.nativeFeature("official.markdown-preview") {
				markdown.View(c, a.previewText, nil).MaxWidth(820)
			} else if ext == "prompt" && a.nativeFeature("official.prompt-metadata") {
				markdown.View(c, a.previewText, nil).MaxWidth(820)
			} else {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					for _, line := range strings.Split(a.previewText, "\n") {
						ui.Text(c, line).FontSize(12).FontFeatures("tnum").TextColor(p.text).Wrap()
					}
				})
			}
		})
		return
	}
	ui.Column(c).Fill().Grow(1).Center().Gap(12).Children(func() {
		ui.Box(c).Size(110, 110).Radius(20).Background(p.surface2).Center().Children(func() {
			ui.Icon(c, categoryIcon(item)).Size(44, 44).TextColor(p.muted)
		})
		ui.Text(c, item.Name).FontSize(16).FontWeight(650).TextColor(p.text)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c.Key("doc-open"), a.t("contextMenu.openWithDefaultApp")).Clicked() {
				_ = a.svc.OpenWithDefaultApp(item.ID)
			}
			if ui.Button(c, a.t("inspector.reveal")).Clicked() {
				_ = a.svc.RevealInFinder(item.ID)
			}
		})
	})
}

func (a *App) loadPreviewContent() {
	a.closeVideo()
	a.previewSeq++
	if trace := a.previewTrace; trace != nil {
		trace.record("info", "superseded", trace.started, "")
	}
	a.previewImages.restore()
	a.filmstripImages.restore()
	a.filmstripTrace = nil
	a.previewImages.clear()
	a.previewTrace = nil
	a.previewImageShown = false
	a.previewSlowLogged = false
	a.previewImagePath = ""
	a.previewBitmap = nil
	a.psdInfo = nil
	a.previewText = ""
	a.previewError = ""
	if a.previewIndex < 0 || a.previewIndex >= len(a.items) {
		return
	}
	item := a.items[a.previewIndex]
	trace := &previewLoadTrace{
		sequence: a.previewSeq, itemID: item.ID, path: item.FilePath,
		index: a.previewIndex, mode: a.previewMode, started: time.Now(),
	}
	a.previewTrace = trace
	trace.record("info", "requested", trace.started, fmt.Sprintf("name=%q", item.Name))
	a.activeItem = &item
	a.activeItemID = item.ID
	if a.itemPages != nil && a.svc != nil {
		loaded, err := a.svc.GetItem(item.ID)
		if err != nil || loaded == nil {
			if err == nil {
				err = fmt.Errorf("preview item was not found")
			}
			a.previewError = err.Error()
			trace.record("error", "metadata_failed", trace.started, fmt.Sprintf("error=%q", err))
			return
		}
		item = *loaded
	}
	trace.path = item.FilePath
	trace.record("perf", "metadata_ready", trace.started, "")
	a.activeItem = &item
	a.activeItemID = item.ID
	if categoryOf(item) == "image" {
		a.previewImagePath = item.FilePath
	}
	ext := strings.ToLower(item.Extension)
	if ext == "md" || ext == "markdown" || ext == "txt" || ext == "json" || ext == "js" || ext == "ts" || ext == "go" || ext == "py" {
		path := item.FilePath
		a.previewSeq++
		seq := a.previewSeq
		go func() {
			const maxPreviewText = 16 << 20
			data, err := readFilePrefix(path, maxPreviewText+1)
			if err == nil && len(data) > maxPreviewText {
				data = nil
				err = fmt.Errorf("document exceeds the 16 MiB preview limit")
			}
			a.post(func() {
				if seq != a.previewSeq {
					return
				}
				if err != nil {
					a.previewError = err.Error()
				} else {
					a.previewText = string(data)
				}
			})
		}()
	} else if strings.TrimSpace(item.Prompt) != "" && ext == "prompt" {
		a.previewText = item.Prompt
	} else if categoryOf(item) == "video" && a.nativeFeature("builtin.video") {
		path, width, height := item.FilePath, item.Width, item.Height
		seq := a.videoSeq
		go func() {
			v, err := player.OpenVideo(path, width, height)
			a.post(func() {
				if seq != a.videoSeq {
					if v != nil {
						v.Close()
					}
					return
				}
				if err != nil {
					a.videoErr = err.Error()
					return
				}
				a.video = v
			})
		}()
	} else if ext == "psd" || ext == "psb" {
		a.previewBitmap = nil
		a.psdInfo = nil
		path := item.FilePath
		a.previewSeq++
		seq := a.previewSeq
		go func() {
			info, img, err := core.ReadPSD(path)
			a.post(func() {
				if seq != a.previewSeq {
					return
				}
				if err != nil {
					a.previewError = err.Error()
					return
				}
				a.psdInfo = info
				if img != nil {
					a.previewBitmap = ui.NewBitmap(img)
				}
			})
		}()
	} else if ext == "hsj" {
		a.previewBitmap = nil
		path := item.FilePath
		a.previewSeq++
		seq := a.previewSeq
		go func() {
			img, _, _, err := core.ReadHSJ(path)
			a.post(func() {
				if seq != a.previewSeq {
					return
				}
				if err != nil {
					a.previewError = err.Error()
					return
				}
				if img != nil {
					a.previewBitmap = ui.NewBitmap(img)
				}
			})
		}()
	} else if ext == "ipv" {
		a.previewBitmap = nil
		path := item.FilePath
		a.previewSeq++
		seq := a.previewSeq
		go func() {
			img, _, _, err := core.ReadIPV(path)
			a.post(func() {
				if seq != a.previewSeq {
					return
				}
				if err != nil {
					a.previewError = err.Error()
					return
				}
				if img != nil {
					a.previewBitmap = ui.NewBitmap(img)
				}
			})
		}()
	}
	a.loadInspector(item.ID)
}

func playerOpen(path string) (*player.Player, error) { return player.Open(path) }

// closeVideo stops the native video player and invalidates any pending open.
func (a *App) closeVideo() {
	a.videoSeq++
	if a.video != nil {
		a.video.Close()
		a.video = nil
	}
	a.videoErr = ""
	a.videoScrub = 0
}

func (a *App) previewPSD(c *ui.Context, item core.Item) {
	p := a.palette(c)
	ui.Box(c).Fill().Grow(1).Shrink(1).Clip().HandleInput(a.handlePreviewImageInput).Children(func() {
		if a.previewBitmap != nil {
			a.drawPreviewImageCanvas(c, a.previewBitmap)
		} else if a.previewError != "" {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Icon(c, iconImage).Size(34, 34).TextColor(p.faint)
				ui.Text(c, a.previewError).FontSize(12).TextColor(p.muted)
			})
		} else {
			ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
		}
	})
}
