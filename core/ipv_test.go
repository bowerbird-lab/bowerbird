package core

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestReadIPV_SyntheticFile(t *testing.T) {
	// Small layer PNG (4x4) followed by composite canvas PNG (10x8) and trailing metadata.
	makePNG := func(w, h int) []byte {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 200, G: 60, B: 90, A: 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("png.Encode: %v", err)
		}
		return buf.Bytes()
	}

	layerPNG := makePNG(4, 4)
	compositePNG := makePNG(10, 8)

	var raw bytes.Buffer
	raw.Write(bytes.Repeat([]byte{0x01}, 16))
	_ = binary.Write(&raw, binary.BigEndian, uint32(10))
	_ = binary.Write(&raw, binary.BigEndian, uint32(8))
	raw.Write(bytes.Repeat([]byte{0x00}, 32))
	raw.Write(layerPNG)
	raw.Write(bytes.Repeat([]byte{0x00}, 16))
	raw.Write(compositePNG)
	raw.Write([]byte("trailing-meta"))

	ipvPath := filepath.Join(t.TempDir(), "sample.ipv")
	if err := os.WriteFile(ipvPath, raw.Bytes(), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	img, width, height, err := ReadIPV(ipvPath)
	if err != nil {
		t.Fatalf("ReadIPV failed: %v", err)
	}
	if img == nil || width != 10 || height != 8 {
		t.Fatalf("unexpected decoded dimensions: %dx%d", width, height)
	}
}

func TestReadIPV_RealSampleIfPresent(t *testing.T) {
	const samplePath = "/Volumes/Data/Library/ACG.library/items/LETIONFY7RAOBUHL/吸血鬼.ipv"
	if _, err := os.Stat(samplePath); err != nil {
		t.Skip("real .ipv sample not present")
	}
	img, width, height, err := ReadIPV(samplePath)
	if err != nil {
		t.Fatalf("ReadIPV(sample) failed: %v", err)
	}
	if img == nil || width != 2048 || height != 2048 {
		t.Fatalf("unexpected sample dimensions: %dx%d", width, height)
	}
}
