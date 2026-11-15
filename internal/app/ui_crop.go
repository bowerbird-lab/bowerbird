package app

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"

	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// CropDialog is the reusable picture cropper: a window locked to an aspect
// ratio, over the picture, with pointer panning and wheel/slider zoom. It knows
// nothing about what it crops: the caller supplies the labels and the picture,
// and gets the cropped pixels back through OnApply.
//
//	dialog := NewCropDialog(img)
//	dialog.Title, dialog.ApplyLabel = "Crop avatar", "Use this picture"
//	dialog.Aspect, dialog.Output = 1, 512
//	dialog.OnApply = func(out image.Image) { saveAvatar(out) }
//	app.crop = dialog // viewCropDialog draws and closes it
//
// A dialog is owned by one caller at a time and lives until View reports it
// closed, either by cancel, Escape, a click outside, or a successful apply.
type CropDialog struct {
	// Caller-supplied copy. Empty labels are drawn as empty text, so supply
	// the localized strings.
	Title, Hint, ZoomLabel, ApplyLabel, CancelLabel string
	// Aspect is the crop window's width divided by its height; 1, the zero
	// value's default, is a square.
	Aspect float32
	// PreviewWidth is the crop window's width in DIPs (cropPreviewSize when 0).
	PreviewWidth float32
	// Output is the long side of the picture OnApply receives, in pixels
	// (cropDefaultSize when 0).
	Output int
	// OnApply receives the cropped picture after the user confirms. It is not
	// called when the dialog is dismissed.
	OnApply func(image.Image)

	// Picture state. Source and Bitmap are set by NewCropDialog; Pan moves the
	// picture in DIPs and Zoom multiplies the cover scale.
	Source image.Image
	Bitmap *ui.Bitmap
	Zoom   float64
	Pan    ui.Point

	box          ui.Handle
	panning      bool
	lastX, lastY float32
}

const (
	cropPreviewSize = float32(260)
	cropDefaultSize = 512
	// cropMaxSource caps the picture the cropper works from: a crop never
	// needs more, and a huge photo would only cost memory.
	cropMaxSource = 1600
)

// NewCropDialog prepares a cropper for img, which must already be at most
// cropMaxSource on its longest side (decodeCropSource does that).
func NewCropDialog(img image.Image) *CropDialog {
	return &CropDialog{
		Source: img,
		Bitmap: ui.NewBitmap(img),
		Zoom:   1,
	}
}

func (d *CropDialog) aspect() float32 {
	if d.Aspect > 0 {
		return d.Aspect
	}
	return 1
}

func (d *CropDialog) previewWidth() float32 {
	if d.PreviewWidth > 0 {
		return d.PreviewWidth
	}
	return cropPreviewSize
}

// previewBox is the crop window's size in DIPs.
func (d *CropDialog) previewBox() (w, h float32) {
	w = d.previewWidth()
	return w, w / d.aspect()
}

// boxSize is the crop window as laid out last frame, for geometry while
// drawing and applying.
func (d *CropDialog) boxSize(c *ui.Context) (w, h float32) {
	if r := d.box.Bounds(c); r.W > 0 && r.H > 0 {
		return r.W, r.H
	}
	return d.previewBox()
}

func (d *CropDialog) clampPan(c *ui.Context) {
	b := d.Source.Bounds()
	w, h := d.boxSize(c)
	limit := cropPanLimit(b.Dx(), b.Dy(), w, h, d.Zoom)
	d.Pan.X = minF(maxF(d.Pan.X, -limit.X), limit.X)
	d.Pan.Y = minF(maxF(d.Pan.Y, -limit.Y), limit.Y)
}

// outputSize is the pixel size OnApply hands back: the long side is Output and
// the other follows Aspect.
func (d *CropDialog) outputSize() (int, int) {
	long := d.Output
	if long <= 0 {
		long = cropDefaultSize
	}
	a := float64(d.aspect())
	if a >= 1 {
		return long, max(1, int(math.Round(float64(long)/a)))
	}
	return max(1, int(math.Round(float64(long)*a))), long
}

// crop returns the picture the window shows, resampled to the output size.
func (d *CropDialog) crop(c *ui.Context) image.Image {
	if d.Source == nil {
		return nil
	}
	b := d.Source.Bounds()
	bw, bh := d.boxSize(c)
	rect := cropRect(b.Dx(), b.Dy(), bw, bh, d.Zoom, d.Pan)
	w, h := d.outputSize()
	return cropImage(d.Source, rect, w, h)
}

// View draws the dialog over the window and reports whether it closed. Style
// comes from the caller's palette, so the reusable cropper follows the app's
// themes without knowing about them.
func (d *CropDialog) View(c *ui.Context, p palette) (closed bool) {
	if d.Source == nil || d.Bitmap == nil {
		return true
	}
	open := true
	applied := false
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		panel.Width(d.previewWidth()+72).Padding(20).Radius(14).Background(p.surface).
			Border(1, p.hairline).Shadow(0, 26, 64, -18, p.cardShade)
		panel.Children(func() {
			ui.Column(c).FillWidth().Gap(12).Children(func() {
				ui.Text(c, d.Title).FontSize(16).FontWeight(650).TextColor(p.text)
				if d.Hint != "" {
					ui.Text(c, d.Hint).FontSize(11).TextColor(p.faint).Wrap()
				}
				d.drawPreview(c, p)
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, d.ZoomLabel).FontSize(11).TextColor(p.muted).Width(44).Shrink(0)
					slider := ui.Slider(c, &d.Zoom, 1, 4).Grow(1)
					if slider.Changed() {
						d.clampPan(c)
					}
					ui.Textf(c, "%d%%", int(math.Round(d.Zoom*100))).FontSize(10.5).
						TextColor(p.faint).FontFeatures("tnum").Width(38).TextAlign(ui.End)
				})
				ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
					if ui.Button(c, d.CancelLabel).Clicked() {
						open = false
					}
					if ui.PrimaryButton(c, d.ApplyLabel).Clicked() {
						applied = true
						if cropped := d.crop(c); cropped != nil && d.OnApply != nil {
							d.OnApply(cropped)
						}
					}
				})
			})
		})
	})
	return applied || !open
}

// drawPreview draws the crop window: a checkerboard behind the picture, third
// lines over it, and the pan and zoom gestures.
func (d *CropDialog) drawPreview(c *ui.Context, p palette) {
	w, h := d.previewBox()
	ui.Box(c).FillWidth().Center().Children(func() {
		box := ui.Box(c).Size(w, h).Radius(10).Clip().Cursor(ui.CursorGrab).Bind(&d.box)
		box.Draw(func(pt *ui.Painter, r ui.Rect) {
			cell := float32(12)
			for y := float32(0); y < r.H; y += cell {
				for x := float32(0); x < r.W; x += cell {
					if (int(x/cell)+int(y/cell))%2 == 0 {
						pt.Fill(ui.Rect{X: r.X + x, Y: r.Y + y, W: cell, H: cell}, p.surface.Alpha(0.55), 0)
					}
				}
			}
			b := d.Source.Bounds()
			src := cropRect(b.Dx(), b.Dy(), r.W, r.H, d.Zoom, d.Pan)
			if src.W > 0 && src.H > 0 {
				pt.ImageRegion(d.Bitmap, src, r, ui.ImageOptions{})
			}
		})
		box.DrawOver(func(pt *ui.Painter, r ui.Rect) {
			line := p.hairline.Alpha(0.5)
			pt.Line(r.X+r.W/3, r.Y, r.X+r.W/3, r.Y+r.H, 1, line)
			pt.Line(r.X+r.W*2/3, r.Y, r.X+r.W*2/3, r.Y+r.H, 1, line)
			pt.Line(r.X, r.Y+r.H/3, r.X+r.W, r.Y+r.H/3, 1, line)
			pt.Line(r.X, r.Y+r.H*2/3, r.X+r.W, r.Y+r.H*2/3, 1, line)
		})
		box.HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputScroll:
				d.Zoom = minF64(maxF64(d.Zoom*float64(1-ev.DY*0.01), 1), 4)
				d.clampPan(c)
				return true
			case ui.InputZoom:
				d.Zoom = minF64(maxF64(d.Zoom*float64(ev.Scale), 1), 4)
				d.clampPan(c)
				return true
			case ui.InputPointerDown:
				d.panning = true
				d.lastX, d.lastY = ev.X, ev.Y
				return true
			case ui.InputPointerMove:
				if d.panning {
					d.Pan.X += ev.X - d.lastX
					d.Pan.Y += ev.Y - d.lastY
					d.clampPan(c)
					d.lastX, d.lastY = ev.X, ev.Y
				}
				return true
			case ui.InputPointerUp:
				d.panning = false
				return true
			}
			return false
		})
	})
}

// viewCropDialog draws the app's open cropper, if any, and forgets it once it
// closes. The cropper itself is generic; this is the window's slot for it.
func (a *App) viewCropDialog(c *ui.Context) {
	if a.crop == nil {
		return
	}
	if a.crop.View(c, a.palette(c)) {
		a.crop = nil
	}
}

// cropRect is the part of an imgW×imgH picture a boxW×boxH window shows when
// the picture covers the window at zoom (1 = cover) and moves by pan DIPs.
func cropRect(imgW, imgH int, boxW, boxH float32, zoom float64, pan ui.Point) ui.Rect {
	if imgW <= 0 || imgH <= 0 || boxW <= 0 || boxH <= 0 {
		return ui.Rect{}
	}
	cover := maxF(boxW/float32(imgW), boxH/float32(imgH))
	scale := cover * float32(math.Max(zoom, 0.01))
	if scale <= 0 {
		return ui.Rect{}
	}
	w := minF(boxW/scale, float32(imgW))
	h := minF(boxH/scale, float32(imgH))
	limit := cropPanLimit(imgW, imgH, boxW, boxH, zoom)
	px := minF(maxF(pan.X, -limit.X), limit.X)
	py := minF(maxF(pan.Y, -limit.Y), limit.Y)
	x := (float32(imgW)-w)/2 - px/scale
	y := (float32(imgH)-h)/2 - py/scale
	x = minF(maxF(x, 0), float32(imgW)-w)
	y = minF(maxF(y, 0), float32(imgH)-h)
	return ui.Rect{X: x, Y: y, W: w, H: h}
}

// cropPanLimit is how far the picture may move at zoom without leaving a gap in
// the crop window, in DIPs.
func cropPanLimit(imgW, imgH int, boxW, boxH float32, zoom float64) ui.Point {
	if imgW <= 0 || imgH <= 0 || boxW <= 0 || boxH <= 0 {
		return ui.Point{}
	}
	cover := maxF(boxW/float32(imgW), boxH/float32(imgH))
	scale := cover * float32(math.Max(zoom, 0.01))
	return ui.Point{
		X: maxF(float32(imgW)*scale-boxW, 0) / 2,
		Y: maxF(float32(imgH)*scale-boxH, 0) / 2,
	}
}

// cropImage resamples rect of src into a w×h picture.
func cropImage(src image.Image, rect ui.Rect, w, h int) *image.RGBA {
	if w <= 0 {
		w = cropDefaultSize
	}
	if h <= 0 {
		h = cropDefaultSize
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	source := image.Rect(
		b.Min.X+int(math.Round(float64(rect.X))),
		b.Min.Y+int(math.Round(float64(rect.Y))),
		b.Min.X+int(math.Round(float64(rect.X+rect.W))),
		b.Min.Y+int(math.Round(float64(rect.Y+rect.H))),
	).Intersect(b)
	if source.Dx() <= 0 || source.Dy() <= 0 {
		return dst
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, source, draw.Src, nil)
	return dst
}

// decodeCropSource reads a picture, moves its origin to (0,0), and caps its
// longest side at max (cropMaxSource when max <= 0), so a cropper never holds a
// full camera-resolution copy.
func decodeCropSource(path string, limit int) (image.Image, error) {
	if limit <= 0 {
		limit = cropMaxSource
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return fitCropSource(img, limit), nil
}

func fitCropSource(img image.Image, limit int) image.Image {
	if limit <= 0 {
		limit = cropMaxSource
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return img
	}
	if b.Min != (image.Point{}) {
		origin := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(origin, origin.Bounds(), img, b.Min, draw.Src)
		img, b = origin, origin.Bounds()
	}
	longest := maxInt(w, h)
	if longest <= limit {
		return img
	}
	scale := float64(limit) / float64(longest)
	nw := maxInt(1, int(math.Round(float64(w)*scale)))
	nh := maxInt(1, int(math.Round(float64(h)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minF64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
