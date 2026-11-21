package core

import (
	"archive/zip"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
)

// ReadPNTR reads an Infinite Painter (.pntr) project archive and decodes its
// embedded preview image along with pixel dimensions.
func ReadPNTR(path string) (image.Image, int, int, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open pntr archive: %w", err)
	}
	defer zr.Close()

	byName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			byName[f.Name] = f
		}
	}

	for _, candidate := range []string{"preview_full", "preview", "playback/last_frame"} {
		f, ok := byName[candidate]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		img, _, decErr := image.Decode(io.LimitReader(rc, 32<<20))
		_ = rc.Close()
		if decErr == nil && img != nil {
			b := img.Bounds()
			return img, b.Dx(), b.Dy(), nil
		}
	}
	return nil, 0, 0, errors.New("no embedded preview image found in pntr archive")
}
