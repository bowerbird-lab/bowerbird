package core

import "testing"

func folderNames(folders []Folder) string {
	names := ""
	for _, f := range folders {
		if names != "" {
			names += ","
		}
		names += f.Name
		if len(f.Children) > 0 {
			names += "(" + folderNames(f.Children) + ")"
		}
	}
	return names
}

func TestFolderOrderRestartsUnderEachParent(t *testing.T) {
	mgr := newQueryTestManager(t)

	rootA, err := mgr.CreateFolder("A", "")
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := mgr.CreateFolder("B", "")
	if err != nil {
		t.Fatal(err)
	}
	childA1, err := mgr.CreateFolder("A1", rootA.ID)
	if err != nil {
		t.Fatal(err)
	}
	childA2, err := mgr.CreateFolder("A2", rootA.ID)
	if err != nil {
		t.Fatal(err)
	}
	childB1, err := mgr.CreateFolder("B1", rootB.ID)
	if err != nil {
		t.Fatal(err)
	}

	// sort_order starts at zero and restarts under every parent.
	if rootA.SortOrder != 0 || rootB.SortOrder != 1 {
		t.Fatalf("root sort orders = %d,%d, want 0,1", rootA.SortOrder, rootB.SortOrder)
	}
	if childA1.SortOrder != 0 || childA2.SortOrder != 1 || childB1.SortOrder != 0 {
		t.Fatalf("child sort orders = %d,%d,%d, want 0,1,0", childA1.SortOrder, childA2.SortOrder, childB1.SortOrder)
	}

	folders, err := mgr.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	if got := folderNames(folders); got != "A(A1,A2),B(B1)" {
		t.Fatalf("initial folder tree = %q", got)
	}

	// Reordering roots leaves every child order untouched.
	if err := mgr.SetFolderOrder("", []string{rootB.ID, rootA.ID}); err != nil {
		t.Fatal(err)
	}
	folders, err = mgr.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	if got := folderNames(folders); got != "B(B1),A(A1,A2)" {
		t.Fatalf("reordered root tree = %q", got)
	}

	// Reordering one parent does not touch its siblings' parent.
	if err := mgr.SetFolderOrder(rootA.ID, []string{childA2.ID, childA1.ID}); err != nil {
		t.Fatal(err)
	}
	folders, err = mgr.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	if got := folderNames(folders); got != "B(B1),A(A2,A1)" {
		t.Fatalf("reordered child tree = %q", got)
	}

	// A new folder appends to the level rather than taking an existing slot.
	rootC, err := mgr.CreateFolder("C", "")
	if err != nil {
		t.Fatal(err)
	}
	if rootC.SortOrder != 2 {
		t.Fatalf("new root sort order = %d, want 2", rootC.SortOrder)
	}
	folders, err = mgr.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	if got := folderNames(folders); got != "B(B1),A(A2,A1),C" {
		t.Fatalf("tree after create = %q", got)
	}

	// The order must describe exactly this parent's folders.
	if err := mgr.SetFolderOrder("", []string{rootA.ID}); err == nil {
		t.Fatal("short folder order was accepted")
	}
	if err := mgr.SetFolderOrder(rootA.ID, []string{childA1.ID, childA1.ID}); err == nil {
		t.Fatal("duplicate folder order was accepted")
	}
	if err := mgr.SetFolderOrder(rootA.ID, []string{rootB.ID, childA1.ID}); err == nil {
		t.Fatal("cross-parent folder order was accepted")
	}
	folders, err = mgr.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	if got := folderNames(folders); got != "B(B1),A(A2,A1),C" {
		t.Fatalf("invalid orders changed the tree: %q", got)
	}
}
