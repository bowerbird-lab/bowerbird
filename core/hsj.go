package core

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
)

var hsjEntryMagic = []byte("8E9E23DF")

// ReadHSJ reads a Huashijie (.hsj) archive and decodes its primary preview image
// along with pixel dimensions.
func ReadHSJ(path string) (image.Image, int, int, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open hsj archive: %w", err)
	}
	defer zr.Close()

	var target *zip.File
	for _, f := range zr.File {
		if f.Name == "0" {
			target = f
			break
		}
		if target == nil && !f.FileInfo().IsDir() {
			target = f
		}
	}
	if target == nil {
		return nil, 0, 0, errors.New("hsj archive contains no canvas entries")
	}

	rc, err := target.Open()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open hsj entry %q: %w", target.Name, err)
	}
	defer rc.Close()

	// Cap read at 64 MiB to guard against malformed archives while covering large canvases.
	const maxEntryBytes = 64 << 20
	data, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read hsj entry %q: %w", target.Name, err)
	}
	return DecodeHSJEntry(data)
}

// DecodeHSJEntry extracts and decodes the embedded JPEG/PNG preview from a
// single .hsj entry payload.
func DecodeHSJEntry(data []byte) (image.Image, int, int, error) {
	payload := data
	if len(data) >= 36 && bytes.Equal(data[:8], hsjEntryMagic) {
		headerLen := int(binary.BigEndian.Uint32(data[8:12]))
		metaOffset := binary.BigEndian.Uint64(data[20:28])
		if headerLen >= 36 && headerLen < len(data) {
			end := len(data)
			if metaOffset > uint64(headerLen) && metaOffset <= uint64(len(data)) {
				end = int(metaOffset)
			}
			payload = data[headerLen:end]
		}
	}

	for _, sig := range [][]byte{
		{0xff, 0xd8, 0xff},                   // JPEG
		{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}, // PNG
	} {
		if idx := bytes.Index(payload, sig); idx >= 0 {
			img, _, err := image.Decode(bytes.NewReader(payload[idx:]))
			if err == nil && img != nil {
				b := img.Bounds()
				return img, b.Dx(), b.Dy(), nil
			}
		}
	}
	return nil, 0, 0, errors.New("no embedded preview image found in hsj entry")
}
