package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"bowerbird/core"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// Drag previews are the picture that follows the pointer while a gallery card
// or the inspector hero is dragged out. The dragged files themselves are
// unchanged: Finder and other apps still receive file URLs. The preview is
// built from the item's image only, without the card's background, so what
// moves looks like the asset rather than a cropped screenshot. A multi-item
// drag shows the selection as a stack.
const (
	// dragPreviewSingleEdge is the longest edge of a single dragged image, in
	// DIPs. The platform draws a preview pixel per DIP.
	dragPreviewSingleEdge = 160
	// dragPreviewStackEdge is the longest edge of one image in a stack, and
	// dragPreviewStackCanvas is the square the stack is centred in.
	dragPreviewStackEdge   = 92
	dragPreviewStackCanvas = 132
	dragPreviewStackSlots  = 4
	// dragPreviewCorner is the rounded corner of one stacked image, and
	// dragPreviewBorderAlpha the neutral edge that separates overlapping
	// images of different colours.
	dragPreviewCorner      = 8.0
	dragPreviewBorderAlpha = 42
	// dragPreviewCacheLimit bounds decoded previews; the gallery only needs
	// the hovered card's, but a selection changes keys as the user clicks.
	dragPreviewCacheLimit = 12
	// dragPreviewMaxPixels refuses originals too large to decode on the main
	// thread during a drag.
	dragPreviewMaxPixels = 24 << 20
)

type dragPreview struct {
	img     image.Image
	hotspot image.Point
}

// dragOptions prepares the native preview for e while the pointer is on it.
// Decoding happens once per selection and is cached; cards that cannot start a
// drag keep zero options, so they use the platform's default preview.
func (a *App) dragOptions(e ui.Element, item core.Item) transfer.DragOptions {
	if !e.Hovered() && !e.Pressed() {
		return transfer.DragOptions{}
	}
	opts := transfer.DragOptions{
		Done: func(transfer.Result) {
			a.itemDragging = false
		},
	}
	if p := a.dragPreviewFor(item); p != nil {
		opts.Preview = p.img
		opts.Hotspot = p.hotspot
	}
	return opts
}

// dragPreviewFor returns the cached preview for the drag starting at item. The
// key covers the whole dragged selection, so dragging a member of a
// multi-selection carries the same stack the inspector shows.
func (a *App) dragPreviewFor(item core.Item) *dragPreview {
	items := a.dragPreviewItems(item)
	if len(items) == 0 {
		return nil
	}
	key := dragPreviewKey(items)
	if p, ok := a.dragPreviews[key]; ok {
		// A nil entry remembers that this selection has no image, so a hover
		// over a video without an icon does not try to decode it each frame.
		return p
	}
	p := buildDragPreview(a.dragPreviewSource, items)
	if a.dragPreviews == nil {
		a.dragPreviews = make(map[string]*dragPreview)
	}
	a.dragPreviews[key] = p
	a.dragPreviewOrder = append(a.dragPreviewOrder, key)
	for len(a.dragPreviewOrder) > dragPreviewCacheLimit {
		delete(a.dragPreviews, a.dragPreviewOrder[0])
		a.dragPreviewOrder = a.dragPreviewOrder[1:]
	}
	return p
}

// dragPreviewItems lists the items a drag starting at item carries, most
// recently selected last so the newest image sits on top of the stack.
func (a *App) dragPreviewItems(item core.Item) []core.Item {
	if len(a.selected) <= 1 || !a.selected[item.ID] {
		return []core.Item{item}
	}
	byID := make(map[string]core.Item, len(a.selected))
	for _, it := range a.items {
		if a.selected[it.ID] {
			byID[it.ID] = it
		}
	}
	items := make([]core.Item, 0, len(a.selected))
	seen := make(map[string]bool, len(a.selected))
	for _, id := range a.selectedOrder {
		if !a.selected[id] || seen[id] {
			continue
		}
		if it, ok := byID[id]; ok {
			items = append(items, it)
			seen[id] = true
		}
	}
	// A selection made without an order (restored state, Select All) still
	// drags every selected item, in gallery order.
	for _, it := range a.items {
		if a.selected[it.ID] && !seen[it.ID] {
			items = append(items, it)
			seen[it.ID] = true
		}
	}
	// The pressed item may not be in the current gallery (a stale inspector
	// hero, for one): keep it in the drag rather than dropping it.
	if !seen[item.ID] {
		items = append(items, item)
	}
	return items
}

func dragPreviewKey(items []core.Item) string {
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "%s\x00%d\x00%t\x00", it.ID, it.UpdatedAt, it.HasThumbnail)
	}
	return b.String()
}

// buildDragPreview composes the preview for items, load supplies one decoded
// image per item (nil when the item has none).
func buildDragPreview(load func(core.Item) image.Image, items []core.Item) *dragPreview {
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		src := load(items[0])
		if src == nil {
			return nil
		}
		img := scaleImageToFit(src, dragPreviewSingleEdge, dragPreviewSingleEdge)
		b := img.Bounds()
		return &dragPreview{img: img, hotspot: image.Pt(b.Dx()/2, b.Dy()/2)}
	}
	return buildStackedDragPreview(load, items)
}

// buildStackedDragPreview lays out the selection like the inspector's stack:
// the newest image on top, older ones fanned out behind it.
func buildStackedDragPreview(load func(core.Item) image.Image, items []core.Item) *dragPreview {
	if len(items) > dragPreviewStackSlots {
		items = items[len(items)-dragPreviewStackSlots:]
	}
	presets := [...]struct {
		angle, dx, dy float64
	}{
		{0, 0, 0},
		{-5, -7, 4},
		{6, 6, 6},
		{-8, -10, 10},
	}
	canvas := float64(dragPreviewStackCanvas)
	dst := image.NewRGBA(image.Rect(0, 0, dragPreviewStackCanvas, dragPreviewStackCanvas))
	drawn := false
	for i, it := range items {
		src := load(it)
		if src == nil {
			continue
		}
		p := presets[(len(items)-1-i)%len(presets)]
		img := rotateImage(roundPreviewLayer(scaleImageToFit(src, dragPreviewStackEdge, dragPreviewStackEdge)), p.angle)
		if img == nil {
			continue
		}
		b := img.Bounds()
		x := math.Round((canvas-float64(b.Dx()))/2 + p.dx)
		y := math.Round((canvas-float64(b.Dy()))/2 + p.dy)
		rect := image.Rect(int(x), int(y), int(x)+b.Dx(), int(y)+b.Dy())
		draw.Draw(dst, rect, img, b.Min, draw.Over)
		drawn = true
	}
	if !drawn {
		return nil
	}
	return &dragPreview{img: dst, hotspot: image.Pt(dragPreviewStackCanvas/2, dragPreviewStackCanvas/2)}
}

// dragPreviewSource reads the picture used for one item: its thumbnail when
// the pipeline made one, the original for images without a thumbnail, and the
// file's system icon otherwise.
func (a *App) dragPreviewSource(item core.Item) image.Image {
	if item.FilePath == "" && a.svc != nil && a.itemPages != nil {
		if loaded, err := a.svc.GetWorkspaceCards([]string{item.ID}); err == nil && len(loaded) > 0 {
			item = loaded[0]
		}
	}
	var paths []string
	if item.HasThumbnail {
		paths = append(paths, filepath.Join(item.ItemPath, "thumbnail.jpg"))
	}
	if categoryOf(item) == "image" && item.FilePath != "" {
		paths = append(paths, item.FilePath)
	}
	for _, path := range paths {
		if img := decodeDragImage(path); img != nil {
			return img
		}
	}
	if data := core.GetSystemFileIconPNG(item.Extension, item.FilePath); len(data) > 0 {
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			return img
		}
	}
	return nil
}

// decodeDragImage decodes a file for a preview. It reads the header first so
// a huge image is skipped instead of decoded on the main thread.
func decodeDragImage(path string) image.Image {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > dragPreviewMaxPixels {
		return nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

// scaleImageToFit shrinks src to fit the box, keeping its aspect ratio. Small
// images keep their pixels instead of being upscaled into a blur.
func scaleImageToFit(src image.Image, maxW, maxH int) image.Image {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	scale := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	if scale >= 1 {
		return src
	}
	dw := max(1, int(math.Round(float64(w)*scale)))
	dh := max(1, int(math.Round(float64(h)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// roundPreviewLayer rounds one stacked image and adds a hairline edge, so
// overlapping images stay distinguishable. The corners are transparent, which
// is what keeps the drag free of a card background.
func roundPreviewLayer(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	r := math.Min(dragPreviewCorner, math.Min(float64(w), float64(h))/2)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	fill := image.NewAlpha(dst.Bounds())
	ring := image.NewAlpha(dst.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sd := roundedRectDistance(float64(x)+0.5, float64(y)+0.5, float64(w), float64(h), r)
			cover := clampUnit(0.5 - sd)
			if cover == 0 {
				continue
			}
			fill.SetAlpha(x, y, color.Alpha{A: uint8(math.Round(cover * 255))})
			// The edge is the one DIP inside the shape.
			edge := cover * (1 - clampUnit(0.5-(sd+1)))
			if edge > 0 {
				ring.SetAlpha(x, y, color.Alpha{A: uint8(math.Round(edge * dragPreviewBorderAlpha))})
			}
		}
	}
	draw.DrawMask(dst, dst.Bounds(), src, b.Min, fill, image.Point{}, draw.Src)
	draw.DrawMask(dst, dst.Bounds(), image.NewUniform(color.NRGBA{118, 120, 126, 255}), image.Point{}, ring, image.Point{}, draw.Over)
	return dst
}

// roundedRectDistance is the signed distance from a point to a rounded
// rectangle with its top-left at the origin: negative inside.
func roundedRectDistance(px, py, w, h, r float64) float64 {
	qx := math.Abs(px-w/2) - (w/2 - r)
	qy := math.Abs(py-h/2) - (h/2 - r)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	return outside + inside - r
}

func clampUnit(v float64) float64 {
	return math.Min(math.Max(v, 0), 1)
}

// rotateImage rotates src about its centre, growing the bounds to fit the
// turned corners. It returns src unchanged when there is nothing to turn.
// The turn is sampled at twice the size and then scaled down, so the turned
// edges are smooth instead of stepped.
func rotateImage(src image.Image, angle float64) image.Image {
	if src == nil || angle == 0 {
		return src
	}
	b := src.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	if w <= 0 || h <= 0 {
		return src
	}
	rad := angle * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	dw := int(math.Ceil(math.Abs(w*cos) + math.Abs(h*sin)))
	dh := int(math.Ceil(math.Abs(w*sin) + math.Abs(h*cos)))
	if dw < 1 || dh < 1 {
		return src
	}
	const scale = 2
	big := image.NewRGBA(image.Rect(0, 0, dw*scale, dh*scale))
	cx, cy := float64(dw*scale)/2, float64(dh*scale)/2
	scx, scy := float64(b.Min.X)+w/2, float64(b.Min.Y)+h/2
	// The matrix also grows the source to the supersampled size; rotating at
	// its own size would leave it in the middle of a larger canvas.
	cos, sin = cos*scale, sin*scale
	m := f64.Aff3{
		cos, -sin, cx - cos*scx + sin*scy,
		sin, cos, cy - sin*scx - cos*scy,
	}
	xdraw.CatmullRom.Transform(big, m, src, b, draw.Src, nil)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), big, big.Bounds(), draw.Src, nil)
	return dst
}
