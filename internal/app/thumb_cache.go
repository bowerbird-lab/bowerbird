package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

type thumbCache struct {
	mu            sync.Mutex
	entries       map[string]*thumbEntry
	order         []string
	queue         []thumbRequest
	limit         int
	maxBytes      int64
	bytes         int64
	active        int
	workers       int
	closed        bool
	releasePixels bool
	notify        func()
	decode        func(string, int) (*ui.Bitmap, error)
}

type thumbEntry struct {
	bmp      *ui.Bitmap
	err      error
	bytes    int64
	path     string
	drawn    bool
	released bool
}

type thumbRequest struct {
	key      string
	entry    *thumbEntry
	targetPx int
	iconExt  string
	iconPath string
	icon     bool
	trace    *previewLoadTrace
	queuedAt time.Time
}

func newThumbCache() *thumbCache {
	return &thumbCache{
		entries: map[string]*thumbEntry{}, limit: 256,
		maxBytes: 128 << 20, workers: 2, decode: decodePath,
	}
}

func newPreviewImageCache() *thumbCache {
	cache := newThumbCache()
	cache.limit = 1
	cache.workers = 1
	return cache
}

func newFilmstripImageCache() *thumbCache {
	cache := newThumbCache()
	cache.maxBytes = 12 << 20
	return cache
}

func (c *thumbCache) setNotify(fn func()) {
	c.mu.Lock()
	c.notify = fn
	c.mu.Unlock()
}

func (c *thumbCache) setReleasePixels(enabled bool) {
	c.mu.Lock()
	c.releasePixels = enabled
	c.mu.Unlock()
}

func (c *thumbCache) setLimits(limit int, maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if limit > 0 {
		c.limit = limit
	}
	if maxBytes > 0 {
		c.maxBytes = maxBytes
	}
	c.trimLocked()
}

func (c *thumbCache) isActive() bool {
	c.mu.Lock()
	active := c.active > 0 || len(c.queue) > 0
	c.mu.Unlock()
	return active
}

func (c *thumbCache) releaseDrawnPixels() int {
	c.mu.Lock()
	var bitmaps []*ui.Bitmap
	for _, entry := range c.entries {
		if entry != nil && entry.bmp != nil && entry.drawn && !entry.released {
			entry.released = true
			c.shrinkEntryBytesLocked(entry)
			bitmaps = append(bitmaps, entry.bmp)
		}
	}
	c.mu.Unlock()
	for _, bmp := range bitmaps {
		bmp.ReleasePixels()
	}
	return len(bitmaps)
}

func (c *thumbCache) shrinkEntryBytesLocked(entry *thumbEntry) {
	if entry == nil || entry.bytes <= 0 {
		return
	}
	// ReleasePixels drops the decoded RGBA + mipmap buffers (~6 B/px), leaving
	// only the compressed image data for remake (~1/16 of decoded size).
	kept := max(int64(4096), entry.bytes/16)
	if kept < entry.bytes {
		c.bytes -= entry.bytes - kept
		if c.bytes < 0 {
			c.bytes = 0
		}
		entry.bytes = kept
	}
}

func thumbTarget(targetPx int) int {
	return min(4096, max(128, (targetPx+127)/128*128))
}

func (c *thumbCache) get(path string, targetPx int) *ui.Bitmap {
	if path == "" {
		return nil
	}
	bitmap, _ := c.getResult(path, targetPx, nil)
	return bitmap
}

func (c *thumbCache) getResult(path string, targetPx int, trace *previewLoadTrace) (*ui.Bitmap, error) {
	if path == "" {
		return nil, fmt.Errorf("image path is empty")
	}
	targetPx = thumbTarget(targetPx)
	key := fmt.Sprintf("%s\x00%d", path, targetPx)
	return c.request(thumbRequest{key: key, entry: &thumbEntry{path: path}, targetPx: targetPx, trace: trace})
}

func (c *thumbCache) getSystemIcon(ext string, filePath string) *ui.Bitmap {
	key := "sysicon:" + ext + ":" + filePath
	bitmap, _ := c.request(thumbRequest{
		key: key, entry: &thumbEntry{path: filePath},
		icon: true, iconExt: ext, iconPath: filePath,
	})
	return bitmap
}

func (c *thumbCache) request(request thumbRequest) (*ui.Bitmap, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("image cache is paused")
	}
	if entry := c.entries[request.key]; entry != nil {
		c.touchLocked(request.key)
		bmp, err := entry.bmp, entry.err
		var releaseBmp *ui.Bitmap
		if bmp != nil {
			if !entry.drawn {
				entry.drawn = true
			} else if c.releasePixels && !entry.released {
				entry.released = true
				c.shrinkEntryBytesLocked(entry)
				releaseBmp = bmp
			}
		}
		c.mu.Unlock()
		if releaseBmp != nil {
			releaseBmp.ReleasePixels()
		}
		return bmp, err
	}
	c.entries[request.key] = request.entry
	c.touchLocked(request.key)
	request.queuedAt = time.Now()
	c.queue = append(c.queue, request)
	c.trimLocked()
	c.startLocked()
	c.mu.Unlock()
	return nil, nil
}

func (c *thumbCache) startLocked() {
	for !c.closed && c.active < c.workers && len(c.queue) > 0 {
		request := c.queue[0]
		c.queue[0] = thumbRequest{}
		c.queue = c.queue[1:]
		if c.entries[request.key] != request.entry {
			continue
		}
		c.active++
		go c.run(request)
	}
	if len(c.queue) == 0 {
		c.queue = nil
	}
}

func (c *thumbCache) run(request thumbRequest) {
	var bitmap *ui.Bitmap
	var err error
	if request.trace != nil && !request.trace.compact {
		request.trace.record("info", "decode_started", request.queuedAt, fmt.Sprintf("target_px=%d", request.targetPx))
	}
	if request.icon {
		data := core.GetSystemFileIconPNG(request.iconExt, request.iconPath)
		if len(data) > 0 {
			bitmap, err = ui.DecodeBitmap(data)
		}
	} else if request.trace != nil && !request.trace.compact {
		bitmap, err = decodePathTraced(request.entry.path, request.targetPx, request.trace)
	} else {
		bitmap, err = c.decode(request.entry.path, request.targetPx)
	}
	if bitmap == nil && err == nil {
		err = fmt.Errorf("image decoder returned no bitmap")
	}
	c.mu.Lock()
	c.active--
	current := !c.closed && c.entries[request.key] == request.entry
	if current {
		request.entry.err = err
	}
	if current && bitmap != nil {
		width, height := bitmap.Size()
		request.entry.bmp = bitmap
		request.entry.bytes = int64(width) * int64(height) * 6
		c.bytes += request.entry.bytes
		c.trimLocked()
	}
	notify := c.notify
	retained := current && c.entries[request.key] == request.entry
	cacheBytes := c.bytes
	c.startLocked()
	c.mu.Unlock()
	if request.trace != nil {
		switch {
		case !retained:
			request.trace.record("info", "discarded", request.queuedAt, "request was superseded or evicted")
		case err != nil:
			request.trace.record("error", "failed", request.queuedAt, fmt.Sprintf("error=%q", err))
		default:
			width, height := bitmap.Size()
			request.trace.record("info", "cached", request.queuedAt, fmt.Sprintf("width=%d height=%d cache_bytes=%d", width, height, cacheBytes))
		}
	}
	if current && notify != nil {
		notify()
	}
}

func (c *thumbCache) touchLocked(key string) {
	for index, existing := range c.order {
		if existing == key {
			copy(c.order[index:], c.order[index+1:])
			c.order = c.order[:len(c.order)-1]
			break
		}
	}
	c.order = append(c.order, key)
}

func (c *thumbCache) removeLocked(key string) {
	if entry := c.entries[key]; entry != nil {
		c.bytes -= entry.bytes
		delete(c.entries, key)
	}
}

func (c *thumbCache) trimLocked() {
	for len(c.order) > c.limit || c.bytes > c.maxBytes && len(c.order) > 1 {
		key := c.order[0]
		copy(c.order, c.order[1:])
		c.order = c.order[:len(c.order)-1]
		c.removeLocked(key)
	}
	if len(c.queue) > c.limit*2 {
		queue := make([]thumbRequest, 0, c.limit)
		for _, request := range c.queue {
			if c.entries[request.key] == request.entry {
				queue = append(queue, request)
			}
		}
		c.queue = queue
	}
}

func (c *thumbCache) invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	order := c.order[:0]
	for _, key := range c.order {
		if entry := c.entries[key]; entry != nil && entry.path == path {
			c.removeLocked(key)
		} else {
			order = append(order, key)
		}
	}
	c.order = order
}

func (c *thumbCache) trim(keep int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	keep = max(0, keep)
	evicted := 0
	for len(c.order) > keep {
		c.removeLocked(c.order[0])
		c.order[0] = ""
		c.order = c.order[1:]
		evicted++
	}
	if keep == 0 {
		c.queue = nil
		c.closed = true
	} else if len(c.queue) > keep*2 {
		queue := make([]thumbRequest, 0, keep)
		for _, request := range c.queue {
			if c.entries[request.key] == request.entry {
				queue = append(queue, request)
			}
		}
		c.queue = queue
	}
	return evicted
}

func (c *thumbCache) restore() {
	c.mu.Lock()
	c.closed = false
	c.mu.Unlock()
}

func (c *thumbCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*thumbEntry{}
	c.order = nil
	c.queue = nil
	c.bytes = 0
}

func decodePath(path string, targetPx int) (*ui.Bitmap, error) {
	return decodePathTraced(path, targetPx, nil)
}

func decodePathTraced(path string, targetPx int, trace *previewLoadTrace) (*ui.Bitmap, error) {
	start := time.Now()
	file, err := os.Open(path)
	if err != nil {
		trace.record("error", "read_header", start, fmt.Sprintf("error=%q", err))
		return nil, err
	}
	config, format, err := image.DecodeConfig(file)
	_ = file.Close()
	if err != nil {
		trace.record("error", "read_header", start, fmt.Sprintf("error=%q", err))
		return nil, err
	}
	trace.record("perf", "read_header", start, fmt.Sprintf("source_width=%d source_height=%d format=%s", config.Width, config.Height, format))
	start = time.Now()
	data, err := os.ReadFile(path)
	if err != nil {
		trace.record("error", "read_file", start, fmt.Sprintf("error=%q", err))
		return nil, err
	}
	trace.record("perf", "read_file", start, fmt.Sprintf("bytes=%d", len(data)))
	if max(config.Width, config.Height) <= targetPx {
		start = time.Now()
		bitmap, err := ui.DecodeBitmap(data)
		trace.recordDecode("decode_bitmap", start, bitmap, err)
		return bitmap, err
	}
	start = time.Now()
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		trace.record("error", "decode_source", start, fmt.Sprintf("error=%q", err))
		return nil, err
	}
	trace.record("perf", "decode_source", start, "")
	start = time.Now()
	width := max(1, config.Width*targetPx/max(config.Width, config.Height))
	height := max(1, config.Height*targetPx/max(config.Width, config.Height))
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), source, source.Bounds(), draw.Src, nil)
	trace.record("perf", "resize", start, fmt.Sprintf("width=%d height=%d", width, height))
	var output bytes.Buffer
	start = time.Now()
	if format == "jpeg" {
		if err := jpeg.Encode(&output, resized, &jpeg.Options{Quality: 90}); err != nil {
			trace.record("error", "encode_resized", start, fmt.Sprintf("error=%q", err))
			return nil, err
		}
		header := data[:min(len(data), 65540)]
		for offset := 2; offset+4 <= len(header) && header[offset] == 0xff; {
			marker := header[offset+1]
			length := int(binary.BigEndian.Uint16(header[offset+2:]))
			end := offset + 2 + length
			if length < 2 || end > len(header) || marker == 0xda {
				break
			}
			if marker == 0xe1 && bytes.HasPrefix(header[offset+4:end], []byte("Exif\x00\x00")) {
				encoded := output.Bytes()
				upright := make([]byte, 0, len(encoded)+end-offset)
				upright = append(upright, encoded[:2]...)
				upright = append(upright, header[offset:end]...)
				upright = append(upright, encoded[2:]...)
				trace.record("perf", "encode_resized", start, fmt.Sprintf("bytes=%d", len(upright)))
				start = time.Now()
				bitmap, err := ui.DecodeBitmap(upright)
				trace.recordDecode("decode_bitmap", start, bitmap, err)
				return bitmap, err
			}
			offset = end
		}
	} else if err := png.Encode(&output, resized); err != nil {
		trace.record("error", "encode_resized", start, fmt.Sprintf("error=%q", err))
		return nil, err
	}
	trace.record("perf", "encode_resized", start, fmt.Sprintf("bytes=%d", output.Len()))
	start = time.Now()
	bitmap, err := ui.DecodeBitmap(output.Bytes())
	trace.recordDecode("decode_bitmap", start, bitmap, err)
	return bitmap, err
}

func thumbPath(itemPath string) string {
	if itemPath == "" {
		return ""
	}
	return filepath.Join(itemPath, "thumbnail.jpg")
}
