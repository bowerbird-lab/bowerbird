package app

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

func writePreviewPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, width, height)))
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func TestImageCacheNotifiesOnSuccessAndFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			cache := newThumbCache()
			notified := make(chan struct{}, 1)
			expected := errors.New("decode failed")
			cache.setNotify(func() { notified <- struct{}{} })
			cache.decode = func(path string, target int) (*ui.Bitmap, error) {
				if failure {
					return nil, expected
				}
				return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 16))), nil
			}
			cache.get("image", 128)
			select {
			case <-notified:
			case <-time.After(3 * time.Second):
				t.Fatal("image completion did not notify the view")
			}
			bitmap, err := cache.getResult("image", 128, nil)
			if failure {
				if bitmap != nil || !errors.Is(err, expected) {
					t.Fatalf("failure result = %v, %v", bitmap, err)
				}
			} else if bitmap == nil || err != nil {
				t.Fatalf("success result = %v, %v", bitmap, err)
			}
		})
	}
}

func TestImageCacheRetainsErrorsUntilRetry(t *testing.T) {
	cache := newPreviewImageCache()
	var attempts atomic.Int32
	expected := errors.New("invalid image")
	cache.decode = func(path string, target int) (*ui.Bitmap, error) {
		if attempts.Add(1) == 1 {
			return nil, expected
		}
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 16))), nil
	}
	cache.get("image", 128)
	waitThumbIdle(t, cache)
	for range 20 {
		if bitmap, err := cache.getResult("image", 128, nil); bitmap != nil || !errors.Is(err, expected) {
			t.Fatalf("failed image should expose the decode error: bitmap=%v err=%v", bitmap, err)
		}
	}
	if attempts.Load() != 1 {
		t.Fatal("redrawing repeatedly decoded the failed image")
	}
	cache.clear()
	cache.get("image", 128)
	waitThumbIdle(t, cache)
	if bitmap, err := cache.getResult("image", 128, nil); bitmap == nil || err != nil || attempts.Load() != 2 {
		t.Fatalf("retry did not reload the image: bitmap=%v err=%v", bitmap, err)
	}
}

func TestImagePreviewBeyondFirstPageSurvivesFilmstripPressure(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	app := seededApp(t)
	const currentIndex = workspacePageSize*2 + 5
	app.items = make([]core.Item, workspacePageSize*4)
	app.itemPages = map[int]bool{0: true, 1: true, 2: true, 3: true}
	app.svc = core.NewService(nil)
	for index := range app.items {
		app.items[index] = core.Item{
			ID: fmt.Sprint(index), Name: fmt.Sprintf("Image %d", index),
			Extension: "png", MimeType: "image/png", Width: 320, Height: 160,
		}
	}
	path := filepath.Join(t.TempDir(), "original.png")
	writePreviewPNG(t, path, 320, 160)
	active := app.items[currentIndex]
	active.FilePath = path
	app.activeItem = &active
	app.activeItemID = active.ID
	app.previewIndex = currentIndex
	app.previewMode = "full"
	app.previewOpen = true
	app.previewTrace = &previewLoadTrace{
		sequence: 123, itemID: active.ID, path: path, index: currentIndex,
		mode: "full", started: time.Now(),
	}
	app.previewImages.maxBytes = 1
	app.thumbs.decode = func(path string, target int) (*ui.Bitmap, error) {
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8))), nil
	}
	tester := ui.NewTester(app.viewPreview, 1280, 800)
	tester.Frame()
	waitThumbIdle(t, app.previewImages)
	tester.Frame()
	if !app.previewImageShown || app.previewError != "" {
		t.Fatalf("paged preview did not display the original: shown=%t err=%s", app.previewImageShown, app.previewError)
	}
	for index := 0; index < 300; index++ {
		app.thumbs.get(fmt.Sprintf("filmstrip-%d", index), 128)
	}
	waitThumbIdle(t, app.thumbs)
	for range 10 {
		tester.Frame()
	}
	bitmap, err := app.previewImages.getResult(path, 4096, app.previewTrace)
	if bitmap == nil || err != nil {
		t.Fatalf("filmstrip evicted the over-budget original: bitmap=%v err=%v", bitmap, err)
	}
	readyLogs, readLogs := 0, 0
	for _, entry := range core.GetLogsSnapshot() {
		if entry.Category != "preview" || !strings.Contains(entry.Detail, path) {
			continue
		}
		switch entry.Message {
		case "Image preview display_ready":
			readyLogs++
		case "Image preview read_file":
			readLogs++
		}
	}
	if readyLogs != 1 || readLogs != 1 {
		t.Fatalf("unchanged preview repeatedly loaded or logged: ready=%d reads=%d", readyLogs, readLogs)
	}
	app.closePreview()
	app.previewImages.mu.Lock()
	remaining := len(app.previewImages.entries)
	app.previewImages.mu.Unlock()
	if remaining != 0 {
		t.Fatal("closing preview retained the original")
	}
}

func TestImagePreviewFailureShowsErrorAndRetries(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	app := seededApp(t)
	path := filepath.Join(t.TempDir(), "missing.png")
	app.items = []core.Item{{
		ID: "broken", Name: "Broken image", Extension: "png",
		MimeType: "image/png", FilePath: path, Width: 320, Height: 160,
	}}
	app.openPreview("broken", "full")
	tester := ui.NewTester(app.viewPreview, 1280, 800)
	tester.Frame()
	waitThumbIdle(t, app.previewImages)
	tester.Frame()
	if app.previewError == "" || !tester.HasText(app.t("preview.imageLoadFailed")) || !tester.HasText(app.t("preview.retry")) {
		t.Fatal("failed image remained in a loading state without a retry action")
	}
	previousRequest := app.previewTrace.sequence
	writePreviewPNG(t, path, 320, 160)
	if err := tester.Click(app.t("preview.retry")); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	waitThumbIdle(t, app.previewImages)
	tester.Frame()
	if app.previewError != "" || !app.previewImageShown || app.previewTrace.sequence == previousRequest {
		t.Fatalf("retry did not display the repaired image: shown=%t err=%s", app.previewImageShown, app.previewError)
	}
	failed, retried := false, false
	for _, entry := range core.GetLogsSnapshot() {
		if entry.Category != "preview" || !strings.Contains(entry.Detail, path) {
			continue
		}
		failed = failed || entry.Message == "Image preview failed"
		retried = retried || entry.Message == "Image preview display_ready"
	}
	if !failed || !retried {
		t.Fatalf("preview trace is incomplete: failure=%t retry=%t", failed, retried)
	}
}

func TestImagePreviewTraceSeparatesDecodeStages(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "resized.png")
	writePreviewPNG(t, path, 256, 128)
	trace := &previewLoadTrace{
		sequence: 42, itemID: "trace-image", path: path, index: 200, mode: "full", started: time.Now(),
	}
	if bitmap, err := decodePathTraced(path, 128, trace); err != nil || bitmap == nil {
		t.Fatalf("trace decode = %v, %v", bitmap, err)
	}
	want := []string{"read_header", "read_file", "decode_source", "resize", "encode_resized", "decode_bitmap"}
	var stages []string
	for _, entry := range core.GetLogsSnapshot() {
		if entry.Category == "preview" && strings.Contains(entry.Detail, path) {
			if !strings.Contains(entry.Detail, "request=42 item=trace-image index=200") {
				t.Fatalf("trace lost its request identity: %s", entry.Detail)
			}
			stages = append(stages, strings.TrimPrefix(entry.Message, "Image preview "))
		}
	}
	if strings.Join(stages, ",") != strings.Join(want, ",") {
		t.Fatalf("decode stages = %v, want %v", stages, want)
	}
}

func TestImagePreviewReportsMissingMetadata(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	app := seededApp(t)
	app.svc = core.NewService(nil)
	app.itemPages = map[int]bool{0: true}
	app.openPreview("a", "full")
	tester := ui.NewTester(app.viewPreview, 1280, 800)
	tester.Frame()
	if app.previewError == "" || !tester.HasText(app.t("preview.imageLoadFailed")) {
		t.Fatal("missing metadata left the preview loading")
	}
	app.previewImages.mu.Lock()
	active, queued := app.previewImages.active, len(app.previewImages.queue)
	app.previewImages.mu.Unlock()
	if active != 0 || queued != 0 {
		t.Fatal("missing metadata started an image decode")
	}
}

func TestFilmstripRendersAtMiddleUnderGalleryCachePressure(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	app := seededApp(t)
	app.items = make([]core.Item, 30000)
	for index := range app.items {
		app.items[index] = core.Item{
			ID: fmt.Sprint(index), Name: fmt.Sprintf("Image %d", index),
			Extension: "png", MimeType: "image/png", HasThumbnail: true,
		}
	}
	const currentIndex = 24149
	const width, height = 1280, 800
	start, end := stripWindow(currentIndex, len(app.items), stripFitCount(width))
	directory := t.TempDir()
	for index := start; index < end; index++ {
		itemPath := filepath.Join(directory, fmt.Sprint(index))
		if err := os.Mkdir(itemPath, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(itemPath, "thumbnail.jpg")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		thumbnail := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for pixel := 0; pixel < len(thumbnail.Pix); pixel += 4 {
			copy(thumbnail.Pix[pixel:pixel+4], []byte{240, 20, 170, 255})
		}
		err = png.Encode(file, thumbnail)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		app.items[index].ItemPath = itemPath
	}
	original := filepath.Join(directory, "original.png")
	writePreviewPNG(t, original, 320, 160)
	app.items[currentIndex].FilePath = original
	app.previewIndex = currentIndex
	app.previewOpen = true
	app.previewMode = "full"
	app.activeItem = &app.items[currentIndex]
	app.activeItemID = app.activeItem.ID
	app.previewTrace = &previewLoadTrace{
		sequence: app.previewSeq, itemID: app.activeItemID, index: currentIndex,
		path: original, mode: "full", started: time.Now(),
	}
	app.thumbs.decode = func(path string, target int) (*ui.Bitmap, error) {
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8))), nil
	}
	view := func(c *ui.Context) {
		for index := 0; index < 128; index++ {
			app.thumbs.get(fmt.Sprintf("gallery-%d", index), 256)
		}
		app.viewPreview(c)
	}
	tester := ui.NewTester(view, width, height)
	for frame := 0; frame < 4; frame++ {
		waitThumbIdle(t, app.thumbs)
		waitThumbIdle(t, app.previewImages)
		waitThumbIdle(t, app.filmstripImages)
		tester.Frame()
	}
	capture := tester.Image()
	coloredPixels := 0
	for row := height - 58; row < height-8; row++ {
		for column := 0; column < width; column++ {
			red, green, blue, _ := capture.At(column, row).RGBA()
			if red>>8 > 200 && green>>8 < 60 && blue>>8 > 120 {
				coloredPixels++
			}
		}
	}
	if coloredPixels < (end-start)*1000 {
		t.Fatalf("filmstrip still shows placeholders: colored pixels=%d visible thumbnails=%d", coloredPixels, end-start)
	}
	for index := start; index < end; index++ {
		path := filepath.Join(app.items[index].ItemPath, "thumbnail.jpg")
		if bitmap, err := app.filmstripImages.getResult(path, 120, nil); bitmap == nil || err != nil {
			t.Fatalf("gallery pressure evicted filmstrip index=%d bitmap=%v err=%v", index, bitmap, err)
		}
	}
	app.closePreview()
	app.filmstripImages.mu.Lock()
	remaining := len(app.filmstripImages.entries)
	app.filmstripImages.mu.Unlock()
	if remaining != 0 {
		t.Fatal("closed preview retained filmstrip images")
	}
}
