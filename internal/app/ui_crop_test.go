package app

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCropRectCoversTheBox(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		boxW float32
		boxH float32
		zoom float64
		pan  ui.Point
		want ui.Rect
	}{
		{
			name: "landscape at cover",
			w:    800, h: 400, boxW: 200, boxH: 200, zoom: 1,
			want: ui.Rect{X: 200, Y: 0, W: 400, H: 400},
		},
		{
			name: "portrait at cover",
			w:    400, h: 800, boxW: 200, boxH: 200, zoom: 1,
			want: ui.Rect{X: 0, Y: 200, W: 400, H: 400},
		},
		{
			name: "square at cover",
			w:    500, h: 500, boxW: 200, boxH: 200, zoom: 1,
			want: ui.Rect{X: 0, Y: 0, W: 500, H: 500},
		},
		{
			name: "zoomed in halves the window",
			w:    800, h: 400, boxW: 200, boxH: 200, zoom: 2,
			want: ui.Rect{X: 300, Y: 100, W: 200, H: 200},
		},
		{
			name: "wide window follows its aspect",
			w:    1600, h: 900, boxW: 320, boxH: 180, zoom: 1,
			want: ui.Rect{X: 0, Y: 0, W: 1600, H: 900},
		},
		{
			name: "wide window zoomed",
			w:    1600, h: 900, boxW: 320, boxH: 180, zoom: 2,
			want: ui.Rect{X: 400, Y: 225, W: 800, H: 450},
		},
		{
			name: "pan is clamped to the picture",
			w:    800, h: 400, boxW: 200, boxH: 200, zoom: 1, pan: ui.Point{X: 1000, Y: -1000},
			want: ui.Rect{X: 0, Y: 0, W: 400, H: 400},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cropRect(tc.w, tc.h, tc.boxW, tc.boxH, tc.zoom, tc.pan)
			if diff := absF(got.X-tc.want.X) + absF(got.Y-tc.want.Y) + absF(got.W-tc.want.W) + absF(got.H-tc.want.H); diff > 0.01 {
				t.Fatalf("cropRect = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCropPanLimitKeepsTheWindowFilled(t *testing.T) {
	limit := cropPanLimit(800, 400, 200, 200, 1)
	if absF(limit.X-100) > 0.01 || limit.Y != 0 {
		t.Fatalf("pan limit = %+v, want 100×0", limit)
	}
	if limit := cropPanLimit(800, 400, 200, 200, 2); absF(limit.X-300) > 0.01 || absF(limit.Y-100) > 0.01 {
		t.Fatalf("pan limit at 2x = %+v, want 300×100", limit)
	}
	if limit := cropPanLimit(1600, 900, 320, 180, 1); absF(limit.X) > 0.01 || absF(limit.Y) > 0.01 {
		t.Fatalf("pan limit for an exactly covered picture = %+v, want 0×0", limit)
	}
}

func TestCropImageResamplesToTheRequestedSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			c := color.RGBA{R: 255, A: 255}
			if x >= 100 {
				c = color.RGBA{B: 255, A: 255}
			}
			src.Set(x, y, c)
		}
	}
	// The right-hand square: everything from x=100 on.
	icon := cropImage(src, ui.Rect{X: 100, Y: 0, W: 100, H: 100}, 64, 64)
	if icon.Bounds() != image.Rect(0, 0, 64, 64) {
		t.Fatalf("crop bounds = %v, want 64×64", icon.Bounds())
	}
	if r, _, b, _ := icon.At(32, 32).RGBA(); r > 0x1000 || b < 0xf000 {
		t.Fatalf("cropped centre is not the right-hand blue half: r=%d b=%d", r, b)
	}

	wide := cropImage(src, ui.Rect{X: 0, Y: 0, W: 200, H: 100}, 512, 288)
	if wide.Bounds() != image.Rect(0, 0, 512, 288) {
		t.Fatalf("wide crop bounds = %v, want 512×288", wide.Bounds())
	}
}

func TestDecodeCropSourceCapsTheLongestSide(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	src := image.NewRGBA(image.Rect(0, 0, 3000, 1000))
	for y := 0; y < 1000; y += 100 {
		for x := 0; x < 3000; x += 100 {
			src.Set(x, y, color.RGBA{G: 200, A: 255})
		}
	}
	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	img, err := decodeCropSource(path, cropMaxSource)
	if err != nil {
		t.Fatalf("decodeCropSource: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != cropMaxSource {
		t.Fatalf("width = %d, want %d", b.Dx(), cropMaxSource)
	}
	if b.Dy() != 533 {
		t.Fatalf("height = %d, want 533", b.Dy())
	}
}

func TestCropDialogOutputFollowsAspect(t *testing.T) {
	square := NewCropDialog(image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if w, h := square.outputSize(); w != cropDefaultSize || h != cropDefaultSize {
		t.Fatalf("default output = %dx%d, want square %d", w, h, cropDefaultSize)
	}
	wide := NewCropDialog(image.NewRGBA(image.Rect(0, 0, 10, 10)))
	wide.Aspect, wide.Output = 16.0/9.0, 512
	if w, h := wide.outputSize(); w != 512 || h != 288 {
		t.Fatalf("wide output = %dx%d, want 512x288", w, h)
	}
	tall := NewCropDialog(image.NewRGBA(image.Rect(0, 0, 10, 10)))
	tall.Aspect, tall.Output = 9.0/16.0, 512
	if w, h := tall.outputSize(); w != 288 || h != 512 {
		t.Fatalf("tall output = %dx%d, want 288x512", w, h)
	}
}

func TestCropDialogAppliesAndDismisses(t *testing.T) {
	a := seededApp(t)
	src := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	newDialog := func() *CropDialog {
		d := NewCropDialog(src)
		d.Title = "Crop picture"
		d.Hint = "Drag to move"
		d.ZoomLabel = "Zoom"
		d.ApplyLabel = "Use this picture"
		d.CancelLabel = "Cancel"
		d.Output = 64
		return d
	}
	var applied image.Image
	dialog := newDialog()
	dialog.OnApply = func(img image.Image) { applied = img }
	a.crop = dialog
	tester := ui.NewTester(func(c *ui.Context) { a.viewCropDialog(c) }, 480, 480)
	tester.Frame()
	for _, want := range []string{"Crop picture", "Use this picture", "Cancel"} {
		if !tester.HasText(want) {
			t.Fatalf("crop dialog is missing %q: %v", want, tester.Texts())
		}
	}

	if err := tester.Click("Use this picture"); err != nil {
		t.Fatalf("click apply: %v", err)
	}
	tester.Frame()
	if applied == nil {
		t.Fatal("apply did not hand back a picture")
	}
	if applied.Bounds() != image.Rect(0, 0, 64, 64) {
		t.Fatalf("applied bounds = %v, want 64×64", applied.Bounds())
	}
	if a.crop != nil {
		t.Fatal("the dialog stayed open after applying")
	}

	a.crop = newDialog()
	tester.Frame()
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if a.crop != nil {
		t.Fatal("Escape did not close the cropper")
	}

	a.crop = newDialog()
	tester.Frame()
	if err := tester.Click("Cancel"); err != nil {
		t.Fatalf("click cancel: %v", err)
	}
	tester.Frame()
	if a.crop != nil {
		t.Fatal("cancel did not close the cropper")
	}
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
