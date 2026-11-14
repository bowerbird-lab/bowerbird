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

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

type thumbCache struct {
	mu       sync.Mutex
	entries  map[string]*thumbEntry
	order    []string
	queue    []thumbRequest
	limit    int
	maxBytes int64
	bytes    int64
	active   int
	closed   bool
	notify   func()
	decode   func(string, int) (*ui.Bitmap, error)
}

type thumbEntry struct {
	bmp   *ui.Bitmap
	bytes int64
	path  string
}

type thumbRequest struct {
	key      string
	entry    *thumbEntry
	targetPx int
	iconExt  string
	iconPath string
	icon     bool
}

func newThumbCache() *thumbCache {
	return &thumbCache{
		entries: map[string]*thumbEntry{}, limit: 96,
		maxBytes: 64 << 20, decode: decodePath,
	}
}

func (c *thumbCache) setNotify(fn func()) {
	c.mu.Lock()
	c.notify = fn
	c.mu.Unlock()
}

func thumbTarget(targetPx int) int {
	return min(4096, max(128, (targetPx+127)/128*128))
}

func (c *thumbCache) get(path string, targetPx int) *ui.Bitmap {
	if path == "" {
		return nil
	}
	targetPx = thumbTarget(targetPx)
	key := fmt.Sprintf("%s\x00%d", path, targetPx)
	return c.request(thumbRequest{key: key, entry: &thumbEntry{path: path}, targetPx: targetPx})
}

func (c *thumbCache) getSystemIcon(ext string, filePath string) *ui.Bitmap {
	key := "sysicon:" + ext + ":" + filePath
	return c.request(thumbRequest{
		key: key, entry: &thumbEntry{path: filePath},
		icon: true, iconExt: ext, iconPath: filePath,
	})
}

func (c *thumbCache) request(request thumbRequest) *ui.Bitmap {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if entry := c.entries[request.key]; entry != nil {
		c.touchLocked(request.key)
		return entry.bmp
	}
	c.entries[request.key] = request.entry
	c.touchLocked(request.key)
	c.queue = append(c.queue, request)
	c.trimLocked()
	c.startLocked()
	return nil
}

func (c *thumbCache) startLocked() {
	for !c.closed && c.active < 2 && len(c.queue) > 0 {
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
	if request.icon {
		data := core.GetSystemFileIconPNG(request.iconExt, request.iconPath)
		if len(data) > 0 {
			bitmap, _ = ui.DecodeBitmap(data)
		}
	} else {
		bitmap, _ = c.decode(request.entry.path, request.targetPx)
	}
	c.mu.Lock()
	c.active--
	current := !c.closed && c.entries[request.key] == request.entry
	if current && bitmap != nil {
		width, height := bitmap.Size()
		request.entry.bmp = bitmap
		request.entry.bytes = int64(width) * int64(height) * 6
		c.bytes += request.entry.bytes
		c.trimLocked()
	}
	notify := c.notify
	c.startLocked()
	c.mu.Unlock()
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

func (c *thumbCache) trim(keep int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	keep = max(0, keep)
	for len(c.order) > keep {
		c.removeLocked(c.order[0])
		c.order = c.order[1:]
	}
	if keep == 0 {
		c.queue = nil
		c.closed = true
	}
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
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, 0); err != nil {
		return nil, err
	}
	if max(config.Width, config.Height) <= targetPx {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return ui.DecodeBitmap(data)
	}
	source, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	width := max(1, config.Width*targetPx/max(config.Width, config.Height))
	height := max(1, config.Height*targetPx/max(config.Width, config.Height))
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), source, source.Bounds(), draw.Src, nil)
	var output bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&output, resized, &jpeg.Options{Quality: 90}); err != nil {
			return nil, err
		}
		if _, err := file.Seek(0, 0); err == nil {
			header := make([]byte, 65540)
			length, _ := file.Read(header)
			header = header[:length]
			for offset := 2; offset+4 <= len(header) && header[offset] == 0xff; {
				marker := header[offset+1]
				length := int(binary.BigEndian.Uint16(header[offset+2:]))
				end := offset + 2 + length
				if length < 2 || end > len(header) || marker == 0xda {
					break
				}
				if marker == 0xe1 && bytes.HasPrefix(header[offset+4:end], []byte("Exif\x00\x00")) {
					data := output.Bytes()
					upright := make([]byte, 0, len(data)+end-offset)
					upright = append(upright, data[:2]...)
					upright = append(upright, header[offset:end]...)
					upright = append(upright, data[2:]...)
					return ui.DecodeBitmap(upright)
				}
				offset = end
			}
		}
	} else if err := png.Encode(&output, resized); err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(output.Bytes())
}

func thumbPath(itemPath string) string {
	if itemPath == "" {
		return ""
	}
	return filepath.Join(itemPath, "thumbnail.jpg")
}
