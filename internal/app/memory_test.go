package app

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

func waitThumbIdle(t *testing.T, cache *thumbCache) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		idle := cache.active == 0 && len(cache.queue) == 0
		cache.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("thumbnail workers did not finish")
}

func TestThumbCacheBoundsWorkersAndRejectsClearedResults(t *testing.T) {
	cache := newThumbCache()
	cache.limit = 4
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	cache.decode = func(path string, target int) (*ui.Bitmap, error) {
		started <- struct{}{}
		<-release
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 16))), nil
	}
	for index := 0; index < 1000; index++ {
		cache.get(fmt.Sprint(index), 128)
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			close(release)
			t.Fatal("workers did not start")
		}
	}
	cache.mu.Lock()
	active, entries, queued := cache.active, len(cache.entries), len(cache.queue)
	cache.mu.Unlock()
	if active != 2 || entries > 4 || queued > 8 {
		close(release)
		t.Fatalf("unbounded cache: active=%d entries=%d queue=%d", active, entries, queued)
	}
	cache.clear()
	close(release)
	waitThumbIdle(t, cache)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) != 0 || len(cache.order) != 0 || cache.bytes != 0 {
		t.Fatal("a cleared decode was resurrected")
	}
}

func TestThumbCacheInvalidationRejectsOldDecode(t *testing.T) {
	cache := newThumbCache()
	first := make(chan struct{})
	second := make(chan struct{})
	started := make(chan int, 2)
	calls := 0
	cache.decode = func(path string, target int) (*ui.Bitmap, error) {
		cache.mu.Lock()
		calls++
		call := calls
		cache.mu.Unlock()
		started <- call
		if call == 1 {
			<-first
		} else {
			<-second
		}
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, call*16, 16))), nil
	}
	cache.get("same", 128)
	<-started
	cache.invalidate("same")
	cache.get("same", 128)
	<-started
	close(second)
	close(first)
	waitThumbIdle(t, cache)
	bitmap := cache.get("same", 128)
	if bitmap == nil {
		t.Fatal("the replacement decode is missing")
	}
	if width, _ := bitmap.Size(); width != 32 {
		t.Fatalf("old decode replaced the new one: width=%d", width)
	}
}

func TestThumbCacheEvictsByBytesAndKeysBySize(t *testing.T) {
	cache := newThumbCache()
	cache.maxBytes = 16 * 16 * 6 * 3
	cache.decode = func(path string, target int) (*ui.Bitmap, error) {
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 16))), nil
	}
	for index := 0; index < 20; index++ {
		cache.get(fmt.Sprint(index), 128)
		waitThumbIdle(t, cache)
	}
	cache.get("19", 256)
	waitThumbIdle(t, cache)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.bytes > cache.maxBytes || len(cache.entries) > 3 {
		t.Fatalf("cache exceeded byte budget: bytes=%d entries=%d", cache.bytes, len(cache.entries))
	}
	if cache.entries["19\x00128"] == nil || cache.entries["19\x00256"] == nil {
		t.Fatal("thumbnail sizes share a cache key")
	}
}

func TestDecodePathBoundsPixelsAndPreservesJPEGOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			var data bytes.Buffer
			if format == "png" {
				if err := png.Encode(&data, source); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := jpeg.Encode(&data, source, nil); err != nil {
					t.Fatal(err)
				}
			}
			encoded := data.Bytes()
			if format == "jpeg" {
				exif := []byte{
					0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0,
					'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0,
					0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0,
				}
				encoded = append(append(append([]byte{}, encoded[:2]...), exif...), encoded[2:]...)
			}
			path := filepath.Join(t.TempDir(), "source."+format)
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			bitmap, err := decodePath(path, 256)
			if err != nil {
				t.Fatal(err)
			}
			width, height := bitmap.Size()
			if format == "png" && (width != 256 || height != 128) ||
				format == "jpeg" && (width != 128 || height != 256) {
				t.Fatalf("decoded dimensions = %dx%d", width, height)
			}
		})
	}
}

func TestWorkspacePagesEvictDetailsAndKeepGeometry(t *testing.T) {
	app := New(nil)
	app.items = make([]core.Item, workspacePageSize*(workspacePageLimit+2))
	app.itemPages = map[int]bool{}
	for index := range app.items {
		app.items[index] = core.Item{
			ID: fmt.Sprint(index), Name: fmt.Sprint(index), Width: 1600, Height: 900, Size: 123,
		}
	}
	for page := 0; page < workspacePageLimit+2; page++ {
		start := page * workspacePageSize
		items := append([]core.Item(nil), app.items[start:start+workspacePageSize]...)
		for index := range items {
			items[index].FilePath = "/original"
			items[index].Prompt = "large prompt"
		}
		app.storeItemPage(page, items)
	}
	if len(app.itemPages) != workspacePageLimit || len(app.itemPageOrder) != workspacePageLimit {
		t.Fatal("workspace retained too many pages")
	}
	for index, item := range app.items {
		if index < workspacePageSize*2 && (item.FilePath != "" || item.Prompt != "") {
			t.Fatal("evicted page still retains details")
		}
		if item.Width != 1600 || item.Height != 900 || item.Size != 123 || item.ID != fmt.Sprint(index) {
			t.Fatal("eviction changed layout or selection identity")
		}
	}
}

func TestAutomaticThumbnailQueueIsBounded(t *testing.T) {
	app := New(nil)
	items := make([]core.Item, 1000)
	for index := range items {
		items[index] = core.Item{
			ID: fmt.Sprint(index), Extension: "jpg", FilePath: "/missing",
		}
	}
	app.backfillMissingThumbnails(items)
	if len(app.thumbJobs.queue) != 64 {
		t.Fatalf("automatic cover queue has %d entries", len(app.thumbJobs.queue))
	}
	oldQueue := app.thumbJobs
	app.thumbJobs = newThumbJobQueue()
	app.finishThumbnail(thumbTask{owner: oldQueue}, nil, fmt.Errorf("old result"))
	if app.thumbJobs.active != 0 || len(app.thumbJobs.queue) != 0 {
		t.Fatal("an old library's thumbnail task changed the new queue")
	}
}

func TestGalleryLayoutReusesRows(t *testing.T) {
	app := seededApp(t)
	first := &app.rows[0][0]
	if allocations := testing.AllocsPerRun(100, app.ensureRows); allocations != 0 {
		t.Fatalf("unchanged layout allocates %g objects", allocations)
	}
	if &app.rows[0][0] != first {
		t.Fatal("unchanged gallery replaced its rows")
	}
	app.rowHeight += 20
	app.ensureRows()
	if &app.rows[0][0] == first {
		t.Fatal("zoom did not rebuild the gallery")
	}
}

func TestSelectionSummaryKeepsOnlyLastFiveItems(t *testing.T) {
	app := New(nil)
	app.items = make([]core.Item, 30000)
	for index := range app.items {
		id := fmt.Sprint(index)
		app.items[index] = core.Item{ID: id, Size: 100}
		app.selected[id] = true
	}
	stack, count, size := app.selectionSummary()
	if len(stack) != 5 || count != 30000 || size != 3000000 {
		t.Fatalf("selection summary: stack=%d count=%d size=%d", len(stack), count, size)
	}
	if stack[0].ID != "29995" || stack[4].ID != "29999" {
		t.Fatalf("fallback selection stack = %v", stack)
	}
	app.selectedOrder = make([]string, len(app.items))
	for index := range app.items {
		app.selectedOrder[index] = app.items[len(app.items)-1-index].ID
	}
	stack, count, size = app.selectionSummary()
	if stack[0].ID != "4" || stack[4].ID != "0" || count != 30000 || size != 3000000 {
		t.Fatal("selection stack lost FIFO order")
	}
}

func BenchmarkGalleryLayout30000(b *testing.B) {
	app := New(nil)
	app.items = make([]core.Item, 30000)
	for index := range app.items {
		app.items[index] = core.Item{ID: fmt.Sprint(index), Width: 1600, Height: 900}
	}
	app.rebuildRows()
	b.Run("rebuild", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			app.rebuildRows()
		}
	})
	b.Run("unchanged_frame", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			app.ensureRows()
		}
	})
}
