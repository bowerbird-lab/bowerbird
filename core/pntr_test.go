package core

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPNTR_SyntheticArchive(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			src.Set(x, y, color.RGBA{R: 50, G: 140, B: 220, A: 255})
		}
	}
	var jpgBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, src, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}

	pntrPath := filepath.Join(t.TempDir(), "sample.pntr")
	f, err := os.Create(pntrPath)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("preview_full")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := w.Write(jpgBuf.Bytes()); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file.Close: %v", err)
	}

	img, width, height, err := ReadPNTR(pntrPath)
	if err != nil {
		t.Fatalf("ReadPNTR failed: %v", err)
	}
	if img == nil || width != 16 || height != 12 {
		t.Fatalf("unexpected decoded dimensions: %dx%d", width, height)
	}
}

func TestReadPNTR_RealSampleIfPresent(t *testing.T) {
	const samplePath = "/Volumes/Data/Library/ACG.library/items/NM1J9NVS7AB0EMGB/项目 (20250701031635).pntr"
	if _, err := os.Stat(samplePath); err != nil {
		t.Skip("real .pntr sample not present")
	}
	img, width, height, err := ReadPNTR(samplePath)
	if err != nil {
		t.Fatalf("ReadPNTR(sample) failed: %v", err)
	}
	if img == nil || width <= 0 || height <= 0 {
		t.Fatalf("unexpected sample dimensions: %dx%d", width, height)
	}
}
