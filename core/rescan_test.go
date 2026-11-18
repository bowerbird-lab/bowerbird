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

func writeRescanTestPNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestRescanLibraryRestoresMissingItemsSafely(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())

	libRoot := t.TempDir()
	mgr := NewLibraryManager()
	info, err := mgr.CreateLibrary(libRoot, "RescanTest")
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	t.Cleanup(func() { _ = mgr.CloseLibrary() })

	// 1. Import a normal item with a folder and tag so we verify existing data is untouched.
	folder, err := mgr.CreateFolder("Design", "")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	srcDir := t.TempDir()
	existingSrc := filepath.Join(srcDir, "existing.png")
	writeRescanTestPNG(t, existingSrc, 32, 32, color.RGBA{R: 200, G: 50, B: 50, A: 255})
	existingItem, err := mgr.ImportSingleFileWithTagsAndFolder(existingSrc, []string{"keep-tag"}, folder.ID)
	if err != nil {
		t.Fatalf("ImportSingleFileWithTagsAndFolder: %v", err)
	}

	// 1b. Import another item and move it to trash so we verify existing trashed items stay in trash.
	trashedSrc := filepath.Join(srcDir, "already-trashed.png")
	writeRescanTestPNG(t, trashedSrc, 24, 24, color.RGBA{R: 120, G: 120, B: 120, A: 255})
	existingTrashed, err := mgr.ImportSingleFileWithTagsAndFolder(trashedSrc, nil, "")
	if err != nil {
		t.Fatalf("ImportSingleFile(trashed): %v", err)
	}
	if err := mgr.MoveToTrash(existingTrashed.ID); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}

	// 2. Create an orphaned item directory WITH an existing thumbnail.png and metadata.json
	//    (simulating an interrupted cloud sync where files exist on disk but SQLite missed them).
	orphan1ID := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	orphan1Dir := filepath.Join(info.Path, "items", orphan1ID)
	if err := os.MkdirAll(orphan1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRescanTestPNG(t, filepath.Join(orphan1Dir, "orphan-one.png"), 64, 48, color.RGBA{R: 30, G: 120, B: 220, A: 255})
	// Write a valid PNG and verify its bytes are untouched after rescan.
	orphan1Thumb := filepath.Join(orphan1Dir, "thumbnail.png")
	writeRescanTestPNG(t, orphan1Thumb, 16, 12, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	thumbBefore, err := os.ReadFile(orphan1Thumb)
	if err != nil {
		t.Fatal(err)
	}
	metaBytes, _ := json.Marshal(ItemMetadata{
		ID:      orphan1ID,
		Hex:     orphan1ID,
		Name:    "Orphan One",
		Tags:    []string{"should-not-be-added"},
		Folders: []string{"nonexistent-folder"},
	})
	if err := os.WriteFile(filepath.Join(orphan1Dir, "metadata.json"), metaBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Create a second orphaned item directory WITHOUT thumbnail or metadata.json.
	orphan2ID := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	orphan2Dir := filepath.Join(info.Path, "items", orphan2ID)
	if err := os.MkdirAll(orphan2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRescanTestPNG(t, filepath.Join(orphan2Dir, "orphan-two.png"), 40, 40, color.RGBA{R: 40, G: 180, B: 90, A: 255})

	// 3b. Create a third orphaned item whose metadata.json indicates it was in Trash.
	orphanTrashedID := "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	orphanTrashedDir := filepath.Join(info.Path, "items", orphanTrashedID)
	if err := os.MkdirAll(orphanTrashedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRescanTestPNG(t, filepath.Join(orphanTrashedDir, "orphan-trashed.png"), 20, 20, color.RGBA{R: 90, G: 90, B: 90, A: 255})
	trashedMetaBytes, _ := json.Marshal(ItemMetadata{
		ID:        orphanTrashedID,
		Hex:       orphanTrashedID,
		Name:      "Orphan Trashed",
		TrashedAt: 1700000123,
	})
	if err := os.WriteFile(filepath.Join(orphanTrashedDir, "metadata.json"), trashedMetaBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Run RescanLibrary.
	added, err := mgr.RescanLibrary()
	if err != nil {
		t.Fatalf("RescanLibrary: %v", err)
	}
	if added != 3 {
		t.Fatalf("RescanLibrary added = %d, want 3", added)
	}

	// 5. Verify existing item kept its tags and folders untouched.
	gotExisting, err := mgr.GetItem(existingItem.ID)
	if err != nil || gotExisting == nil {
		t.Fatalf("GetItem(existing): %v", err)
	}
	if len(gotExisting.Tags) != 1 || gotExisting.Tags[0] != "keep-tag" {
		t.Fatalf("existing tags modified: %v", gotExisting.Tags)
	}
	if len(gotExisting.Folders) != 1 || gotExisting.Folders[0] != folder.ID {
		t.Fatalf("existing folders modified: %v", gotExisting.Folders)
	}

	// 6. Verify orphaned items are in All with NO tags and NO folders.
	for _, id := range []string{orphan1ID, orphan2ID} {
		it, err := mgr.GetItem(id)
		if err != nil || it == nil {
			t.Fatalf("GetItem(%s) missing after rescan: %v", id, err)
		}
		if len(it.Tags) != 0 {
			t.Fatalf("rescanned item %s should have no tags, got %v", id, it.Tags)
		}
		if len(it.Folders) != 0 {
			t.Fatalf("rescanned item %s should have no folders, got %v", id, it.Folders)
		}
		if !it.HasThumbnail {
			t.Fatalf("rescanned item %s should have HasThumbnail=true", id)
		}
	}

	// 5b. Verify existing trashed item stayed in trash.
	gotExistingTrashed, err := mgr.GetItem(existingTrashed.ID)
	if err != nil || gotExistingTrashed == nil {
		t.Fatalf("GetItem(existingTrashed): %v", err)
	}
	if !gotExistingTrashed.InTrash || gotExistingTrashed.TrashedAt == 0 {
		t.Fatalf("existing trashed item left trash: InTrash=%v TrashedAt=%d", gotExistingTrashed.InTrash, gotExistingTrashed.TrashedAt)
	}

	// 6b. Verify orphaned trashed item was restored into trash, not All.
	gotOrphanTrashed, err := mgr.GetItem(orphanTrashedID)
	if err != nil || gotOrphanTrashed == nil {
		t.Fatalf("GetItem(orphanTrashed): %v", err)
	}
	if !gotOrphanTrashed.InTrash || gotOrphanTrashed.TrashedAt != 1700000123 {
		t.Fatalf("orphan trashed item = (InTrash=%v, TrashedAt=%d), want (true, 1700000123)", gotOrphanTrashed.InTrash, gotOrphanTrashed.TrashedAt)
	}

	// 7. Verify existing thumbnail file on disk was NOT overwritten.
	thumbAfter, err := os.ReadFile(orphan1Thumb)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(thumbBefore, thumbAfter) {
		t.Fatal("existing thumbnail.png was overwritten during rescan")
	}

	// 8. Second rescan is idempotent and adds 0 items.
	addedAgain, err := mgr.RescanLibrary()
	if err != nil {
		t.Fatalf("RescanLibrary(second): %v", err)
	}
	if addedAgain != 0 {
		t.Fatalf("second RescanLibrary added = %d, want 0", addedAgain)
	}
}
