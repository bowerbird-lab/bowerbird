package core

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestReadHSJ_SyntheticArchive(t *testing.T) {
	// Create a small 8x6 JPEG in memory.
	src := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			src.Set(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
		}
	}
	var jpgBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, src, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}

	// Build an 8E9E23DF-framed entry with padding before JPEG and trailing metadata.
	const headerLen = 36
	pad := bytes.Repeat([]byte{0}, 64)
	meta := []byte{0x08, 0x01, 0x10, 0x02}
	payloadStart := headerLen + len(pad) + jpgBuf.Len()
	totalLen := payloadStart + len(meta)

	var entry bytes.Buffer
	entry.WriteString("8E9E23DF")
	_ = binary.Write(&entry, binary.BigEndian, uint32(headerLen))
	_ = binary.Write(&entry, binary.BigEndian, uint64(totalLen))
	_ = binary.Write(&entry, binary.BigEndian, uint64(payloadStart))
	_ = binary.Write(&entry, binary.BigEndian, uint64(len(meta)))
	entry.Write(pad)
	entry.Write(jpgBuf.Bytes())
	entry.Write(meta)

	hsjPath := filepath.Join(t.TempDir(), "sample.hsj")
	f, err := os.Create(hsjPath)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("0")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := w.Write(entry.Bytes()); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file.Close: %v", err)
	}

	img, width, height, err := ReadHSJ(hsjPath)
	if err != nil {
		t.Fatalf("ReadHSJ failed: %v", err)
	}
	if img == nil || width != 8 || height != 6 {
		t.Fatalf("unexpected decoded image (%dx%d)", width, height)
	}
}

func TestReadHSJ_RealSampleIfPresent(t *testing.T) {
	const samplePath = "/Volumes/Data/Library/ACG.library/items/INQB6JRI1B5VWR5F/画.hsj"
	if _, err := os.Stat(samplePath); err != nil {
		t.Skip("real .hsj sample not present")
	}
	img, width, height, err := ReadHSJ(samplePath)
	if err != nil {
		t.Fatalf("ReadHSJ(sample) failed: %v", err)
	}
	if img == nil || width != 1619 || height != 1011 {
		t.Fatalf("unexpected sample dimensions: %dx%d", width, height)
	}
}
