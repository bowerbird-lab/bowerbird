package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"bowerbird/core"
)

func solidImage(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestSingleDragPreviewIsTheImageWithoutCardBackground(t *testing.T) {
	src := solidImage(120, 60, color.RGBA{210, 90, 60, 255})
	p := buildDragPreview(func(core.Item) image.Image { return src }, []core.Item{{ID: "a"}})
	if p == nil {
		t.Fatal("no preview")
	}
	if b := p.img.Bounds(); b.Dx() != 120 || b.Dy() != 60 {
		t.Fatalf("preview bounds = %v, want the image's own 120x60 without padding", b)
	}
	if p.hotspot != image.Pt(60, 30) {
		t.Fatalf("hotspot = %v, want the preview's centre", p.hotspot)
	}
}

func TestLargeDragPreviewIsScaledToThePreviewBox(t *testing.T) {
	src := solidImage(800, 400, color.RGBA{60, 90, 210, 255})
	p := buildDragPreview(func(core.Item) image.Image { return src }, []core.Item{{ID: "a"}})
	if p == nil {
		t.Fatal("no preview")
	}
	if b := p.img.Bounds(); b.Dx() != dragPreviewSingleEdge || b.Dy() != dragPreviewSingleEdge/2 {
		t.Fatalf("preview bounds = %v, want %dx%d keeping the aspect", b, dragPreviewSingleEdge, dragPreviewSingleEdge/2)
	}
}

func TestMultiDragPreviewStacksTheSelection(t *testing.T) {
	src := solidImage(200, 200, color.RGBA{70, 140, 90, 255})
	items := []core.Item{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	p := buildDragPreview(func(core.Item) image.Image { return src }, items)
	if p == nil {
		t.Fatal("no preview")
	}
	if b := p.img.Bounds(); b.Dx() != dragPreviewStackCanvas || b.Dy() != dragPreviewStackCanvas {
		t.Fatalf("stack bounds = %v, want %dx%d", b, dragPreviewStackCanvas, dragPreviewStackCanvas)
	}
	if p.hotspot != image.Pt(dragPreviewStackCanvas/2, dragPreviewStackCanvas/2) {
		t.Fatalf("hotspot = %v, want the stack's centre", p.hotspot)
	}
	// The stack is images only: its corners stay transparent instead of
	// carrying the card's background, while its middle is covered.
	if _, _, _, a := p.img.At(0, 0).RGBA(); a != 0 {
		t.Fatal("stack corner is not transparent")
	}
	if _, _, _, a := p.img.At(p.hotspot.X, p.hotspot.Y).RGBA(); a == 0 {
		t.Fatal("stack centre is transparent")
	}
}

func TestDragPreviewItemsFollowSelectionOrder(t *testing.T) {
	a := seededApp(t)
	a.selected = map[string]bool{"a": true, "b": true, "c": true}
	a.selectedOrder = []string{"c", "a", "b"}

	got := a.dragPreviewItems(a.items[1])
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("drag items = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("drag item %d = %q, want %q (newest last)", i, got[i].ID, want[i])
		}
	}

	// Dragging an item outside the selection carries only that item.
	if got := a.dragPreviewItems(core.Item{ID: "outside"}); len(got) != 1 || got[0].ID != "outside" {
		t.Fatalf("outside drag items = %v, want only the pressed item", got)
	}
}

func TestDragPreviewReadsTheThumbnailAndCachesIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thumbnail.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, solidImage(300, 150, color.RGBA{120, 80, 200, 255}), nil); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	a := seededApp(t)
	a.items = []core.Item{{ID: "x", ItemPath: dir, HasThumbnail: true, FilePath: filepath.Join(dir, "missing.png")}}
	a.selected = map[string]bool{"x": true}

	p := a.dragPreviewFor(a.items[0])
	if p == nil {
		t.Fatal("no preview from the item's thumbnail")
	}
	if b := p.img.Bounds(); b.Dx() != dragPreviewSingleEdge || b.Dy() != dragPreviewSingleEdge/2 {
		t.Fatalf("preview bounds = %v, want a scaled 300x150 thumbnail", b)
	}
	if again := a.dragPreviewFor(a.items[0]); again != p {
		t.Fatal("preview was not cached")
	}
}
