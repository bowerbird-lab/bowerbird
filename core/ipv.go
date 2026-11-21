package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"os"
)

var ipvPNGSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// ReadIPV reads an ibisPaint (.ipv) project file and decodes its embedded
// composite preview PNG along with pixel dimensions.
func ReadIPV(path string) (image.Image, int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read ipv file: %w", err)
	}
	return DecodeIPV(data)
}

// DecodeIPV extracts and decodes the composite preview PNG from an ibisPaint
// (.ipv) payload. The composite image is stored as the final PNG chunk before
// the trailing metadata block.
func DecodeIPV(data []byte) (image.Image, int, int, error) {
	if len(data) < 24 {
		return nil, 0, 0, errors.New("ipv file too small")
	}

	var canvasW, canvasH int
	w := int(binary.BigEndian.Uint32(data[16:20]))
	h := int(binary.BigEndian.Uint32(data[20:24]))
	if w > 0 && w <= 32768 && h > 0 && h <= 32768 {
		canvasW, canvasH = w, h
	}

	// Scan from the end: the composited full-canvas PNG is placed last, just
	// before the trailing project metadata.
	end := len(data)
	for end > 0 {
		idx := bytes.LastIndex(data[:end], ipvPNGSignature)
		if idx < 0 {
			break
		}
		img, _, err := image.Decode(bytes.NewReader(data[idx:]))
		if err == nil && img != nil {
			b := img.Bounds()
			if (canvasW == 0 && canvasH == 0) || (b.Dx() == canvasW && b.Dy() == canvasH) {
				return img, b.Dx(), b.Dy(), nil
			}
			// Keep searching if this isn't the canvas-sized composite, but fall
			// back to the last valid PNG if none match exact canvas dimensions.
			if idx == 0 {
				return img, b.Dx(), b.Dy(), nil
			}
			// Check if any earlier PNG matches canvasW x canvasH; otherwise return this last one.
			if earlier := findExactCanvasPNG(data[:idx], canvasW, canvasH); earlier != nil {
				eb := earlier.Bounds()
				return earlier, eb.Dx(), eb.Dy(), nil
			}
			return img, b.Dx(), b.Dy(), nil
		}
		end = idx
	}
	return nil, 0, 0, errors.New("no embedded preview PNG found in ipv file")
}

func findExactCanvasPNG(data []byte, canvasW, canvasH int) image.Image {
	end := len(data)
	for end > 0 {
		idx := bytes.LastIndex(data[:end], ipvPNGSignature)
		if idx < 0 {
			return nil
		}
		if img, _, err := image.Decode(bytes.NewReader(data[idx:])); err == nil && img != nil {
			b := img.Bounds()
			if b.Dx() == canvasW && b.Dy() == canvasH {
				return img
			}
		}
		end = idx
	}
	return nil
}
