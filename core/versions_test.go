package core

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func makeSolidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestItemReplaceBackupRestoreAndDeleteVersion(t *testing.T) {
	tmp := t.TempDir()
	mgr := &LibraryManager{}
	if _, err := mgr.CreateLibrary(tmp, "VersionTest"); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	defer mgr.CloseLibrary()

	origPNG := makeSolidPNG(t, 32, 24, color.RGBA{R: 200, G: 40, B: 40, A: 255})
	origPath := filepath.Join(tmp, "red.png")
	if err := os.WriteFile(origPath, origPNG, 0644); err != nil {
		t.Fatalf("write orig: %v", err)
	}

	items, err := mgr.ImportFiles([]string{origPath})
	if err != nil || len(items) != 1 {
		t.Fatalf("ImportFiles = %d, %v", len(items), err)
	}
	item := items[0]
	origHex := item.Hex

	// 1. Replace with a new blue PNG (64x48) -> old red PNG should be backed up in same directory
	newPNG := makeSolidPNG(t, 64, 48, color.RGBA{R: 30, G: 90, B: 220, A: 255})
	updated, err := mgr.ReplaceItemWithBackup(item.ID, newPNG, ReplaceItemOptions{
		Extension: "png",
		Prompt:    "turn it blue",
		Source:    "official.mage-ai-image",
	})
	if err != nil {
		t.Fatalf("ReplaceItemWithBackup: %v", err)
	}
	if updated.Width != 64 || updated.Height != 48 {
		t.Fatalf("updated dimensions = %dx%d, want 64x48", updated.Width, updated.Height)
	}
	if updated.Prompt != "turn it blue" {
		t.Fatalf("updated prompt = %q, want %q", updated.Prompt, "turn it blue")
	}
	if len(updated.Versions) != 1 {
		t.Fatalf("versions count = %d, want 1", len(updated.Versions))
	}

	v0 := updated.Versions[0]
	if v0.Hex != origHex {
		t.Fatalf("backup hex = %q, want %q", v0.Hex, origHex)
	}
	if v0.Width != 32 || v0.Height != 24 {
		t.Fatalf("backup dimensions = %dx%d, want 32x24", v0.Width, v0.Height)
	}
	backupFile := filepath.Join(updated.ItemPath, v0.Filename)
	if _, err := os.Stat(backupFile); err != nil {
		t.Fatalf("backup file missing in same dir %s: %v", backupFile, err)
	}

	// Verify metadata.json persisted versions
	metaBytes, err := os.ReadFile(filepath.Join(updated.ItemPath, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	var meta ItemMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	if len(meta.Versions) != 1 || meta.Versions[0].ID != v0.ID {
		t.Fatalf("metadata.json versions = %+v, want 1 entry with id %s", meta.Versions, v0.ID)
	}

	// 2. Restore original version -> active becomes 32x24 red again, and blue 64x48 becomes a backup
	restored, err := mgr.RestoreItemVersion(item.ID, v0.ID)
	if err != nil {
		t.Fatalf("RestoreItemVersion: %v", err)
	}
	if restored.Width != 32 || restored.Height != 24 || restored.Hex != origHex {
		t.Fatalf("restored item = %dx%d hex=%s, want 32x24 hex=%s", restored.Width, restored.Height, restored.Hex, origHex)
	}
	if len(restored.Versions) != 1 {
		t.Fatalf("versions after restore = %d, want 1", len(restored.Versions))
	}
	if restored.Versions[0].Width != 64 || restored.Versions[0].Height != 48 {
		t.Fatalf("backup after restore = %dx%d, want 64x48", restored.Versions[0].Width, restored.Versions[0].Height)
	}

	// 3. Delete historical version -> backup file removed from disk and metadata.json updated
	delID := restored.Versions[0].ID
	delFile := filepath.Join(restored.ItemPath, restored.Versions[0].Filename)
	afterDel, err := mgr.DeleteItemVersion(item.ID, delID)
	if err != nil {
		t.Fatalf("DeleteItemVersion: %v", err)
	}
	if len(afterDel.Versions) != 0 {
		t.Fatalf("versions after delete = %d, want 0", len(afterDel.Versions))
	}
	if _, err := os.Stat(delFile); !os.IsNotExist(err) {
		t.Fatalf("deleted backup file still exists: %s", delFile)
	}

	// 4. SaveNewItemFromBytes creates a distinct new item
	saved, err := mgr.SaveNewItemFromBytes(newPNG, "blue-copy.png", ImportItemOptions{
		Prompt: "make it blue",
	})
	if err != nil {
		t.Fatalf("SaveNewItemFromBytes: %v", err)
	}
	if saved.ID == item.ID {
		t.Fatalf("expected new item ID, got same ID %s", saved.ID)
	}
	if saved.Width != 64 || saved.Height != 48 || saved.Prompt != "make it blue" {
		t.Fatalf("saved new item = %+v", saved)
	}
}
