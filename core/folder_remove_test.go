package core_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBatchRemoveItemsFromFoldersUnlinksOnlyTheRequestedFolders(t *testing.T) {
	mgr := newRaceLibrary(t, "Unlink")
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}
	items, err := mgr.ImportFiles([]string{first, second})
	if err != nil || len(items) != 2 {
		t.Fatalf("ImportFiles = %v, %v", items, err)
	}
	folderA, err := mgr.CreateFolder("A", "")
	if err != nil {
		t.Fatalf("CreateFolder A: %v", err)
	}
	folderB, err := mgr.CreateFolder("B", "")
	if err != nil {
		t.Fatalf("CreateFolder B: %v", err)
	}
	if err := mgr.AddItemToFolder(items[0].ID, folderA.ID); err != nil {
		t.Fatalf("add first to A: %v", err)
	}
	if err := mgr.AddItemToFolder(items[0].ID, folderB.ID); err != nil {
		t.Fatalf("add first to B: %v", err)
	}
	if err := mgr.AddItemToFolder(items[1].ID, folderA.ID); err != nil {
		t.Fatalf("add second to A: %v", err)
	}

	if err := mgr.BatchRemoveItemsFromFolders(
		[]string{items[0].ID, items[1].ID},
		[]string{folderA.ID},
	); err != nil {
		t.Fatalf("BatchRemoveItemsFromFolders: %v", err)
	}

	firstAfter, err := mgr.GetItem(items[0].ID)
	if err != nil {
		t.Fatalf("GetItem first: %v", err)
	}
	if len(firstAfter.Folders) != 1 || firstAfter.Folders[0] != folderB.ID {
		t.Fatalf("first folders = %v, want only B", firstAfter.Folders)
	}
	secondAfter, err := mgr.GetItem(items[1].ID)
	if err != nil {
		t.Fatalf("GetItem second: %v", err)
	}
	if len(secondAfter.Folders) != 0 {
		t.Fatalf("second folders = %v, want none", secondAfter.Folders)
	}

	folders, err := mgr.GetFolders()
	if err != nil {
		t.Fatalf("GetFolders: %v", err)
	}
	counts := map[string]int{}
	for _, folder := range folders {
		counts[folder.ID] = folder.ItemCount
	}
	if counts[folderA.ID] != 0 || counts[folderB.ID] != 1 {
		t.Fatalf("folder counts = %v, want A=0 B=1", counts)
	}
}
