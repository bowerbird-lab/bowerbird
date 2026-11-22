package core

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestItemVariantsLifecycle(t *testing.T) {
	tmp := t.TempDir()
	mgr := &LibraryManager{}
	if _, err := mgr.CreateLibrary(tmp, "VariantTest"); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	defer mgr.CloseLibrary()

	// Import an original image and two arbitrary non-image files (markdown + binary video placeholder).
	origPath := filepath.Join(tmp, "original.png")
	if err := os.WriteFile(origPath, makeSolidPNG(t, 32, 24, color.RGBA{R: 180, G: 60, B: 60, A: 255}), 0o644); err != nil {
		t.Fatalf("write orig: %v", err)
	}
	mdPath := filepath.Join(tmp, "notes.md")
	if err := os.WriteFile(mdPath, []byte("# AI Prompt Notes\nMake it cinematic"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}
	binPath := filepath.Join(tmp, "gen-video.mp4")
	if err := os.WriteFile(binPath, []byte("fake-mp4-stream-data-123456"), 0o644); err != nil {
		t.Fatalf("write mp4: %v", err)
	}

	imported, err := mgr.ImportFiles([]string{origPath, mdPath, binPath})
	if err != nil || len(imported) != 3 {
		t.Fatalf("ImportFiles = %d, %v", len(imported), err)
	}
	origID := imported[0].ID
	mdID := imported[1].ID
	mp4ID := imported[2].ID

	// 1. Link arbitrary files as variants of origID.
	parent, err := mgr.LinkVariants(origID, []string{mdID, mp4ID})
	if err != nil {
		t.Fatalf("LinkVariants: %v", err)
	}
	if len(parent.Variants) != 2 || parent.Variants[0] != mdID || parent.Variants[1] != mp4ID {
		t.Fatalf("parent.Variants = %v, want [%s %s]", parent.Variants, mdID, mp4ID)
	}

	// Verify metadata.json on disk contains variants.
	metaBytes, err := os.ReadFile(filepath.Join(parent.ItemPath, itemMetadataName))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	var meta ItemMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	if len(meta.Variants) != 2 {
		t.Fatalf("metadata.json variants = %v, want 2", meta.Variants)
	}

	// Verify WorkspaceIndexContext returns Variants and VariantOf.
	page, err := mgr.WorkspaceIndexContext(context.Background(), QuerySpec{}, "desc", nil, false, false)
	if err != nil {
		t.Fatalf("WorkspaceIndexContext: %v", err)
	}
	byID := map[string]Item{}
	for _, it := range page.Items {
		byID[it.ID] = it
	}
	if len(byID[origID].Variants) != 2 {
		t.Fatalf("workspace item variants = %v, want 2", byID[origID].Variants)
	}
	if byID[mdID].VariantOf != origID || byID[mp4ID].VariantOf != origID {
		t.Fatalf("workspace VariantOf = (%q, %q), want %q", byID[mdID].VariantOf, byID[mp4ID].VariantOf, origID)
	}

	// 2. GetVariantGroup works from both primary and child variant IDs.
	p, vars, err := mgr.GetVariantGroup(mp4ID)
	if err != nil {
		t.Fatalf("GetVariantGroup: %v", err)
	}
	if p.ID != origID || len(vars) != 2 {
		t.Fatalf("GetVariantGroup = (%s, %d), want (%s, 2)", p.ID, len(vars), origID)
	}

	// 3. SaveVariantFromBytes works for arbitrary file types.
	vBytesItem, err := mgr.SaveVariantFromBytes(origID, []byte("{\"step\":1}"), "params.json", ImportItemOptions{Prompt: "seed 42"})
	if err != nil {
		t.Fatalf("SaveVariantFromBytes: %v", err)
	}
	if vBytesItem.Extension != "json" || vBytesItem.VariantOf != origID {
		t.Fatalf("SaveVariantFromBytes = ext:%q variantOf:%q, want json / %s", vBytesItem.Extension, vBytesItem.VariantOf, origID)
	}

	// 4. SetPrimaryVariant promotes mp4ID to primary.
	newPrimary, err := mgr.SetPrimaryVariant(origID, mp4ID)
	if err != nil {
		t.Fatalf("SetPrimaryVariant: %v", err)
	}
	if newPrimary.ID != mp4ID || newPrimary.VariantOf != "" || len(newPrimary.Variants) != 3 {
		t.Fatalf("newPrimary = id:%s variantOf:%q variants:%v", newPrimary.ID, newPrimary.VariantOf, newPrimary.Variants)
	}
	oldPrimary, err := mgr.GetItem(origID)
	if err != nil || oldPrimary.VariantOf != mp4ID || len(oldPrimary.Variants) != 0 {
		t.Fatalf("oldPrimary after promote = variantOf:%q variants:%v err:%v", oldPrimary.VariantOf, oldPrimary.Variants, err)
	}

	// 5. UnlinkVariant detaches mdID.
	afterUnlink, err := mgr.UnlinkVariant(mp4ID, mdID)
	if err != nil {
		t.Fatalf("UnlinkVariant: %v", err)
	}
	if len(afterUnlink.Variants) != 2 {
		t.Fatalf("afterUnlink.Variants = %v, want 2", afterUnlink.Variants)
	}
	unlinked, _ := mgr.GetItem(mdID)
	if unlinked.VariantOf != "" {
		t.Fatalf("unlinked.VariantOf = %q, want empty", unlinked.VariantOf)
	}

	// 6. DeleteItem cleans up variant references.
	if err := mgr.DeleteItem(vBytesItem.ID); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	afterDel, _ := mgr.GetItem(mp4ID)
	if len(afterDel.Variants) != 1 || afterDel.Variants[0] != origID {
		t.Fatalf("afterDel.Variants = %v, want [%s]", afterDel.Variants, origID)
	}
}

func TestAddVariantFromInternalItemFile(t *testing.T) {
	tmp := t.TempDir()
	mgr := &LibraryManager{}
	if _, err := mgr.CreateLibrary(tmp, "VariantInternalTest"); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	defer mgr.CloseLibrary()

	p1 := filepath.Join(tmp, "one.png")
	p2 := filepath.Join(tmp, "two.png")
	if err := os.WriteFile(p1, makeSolidPNG(t, 16, 16, color.RGBA{R: 200, G: 20, B: 20, A: 255}), 0o644); err != nil {
		t.Fatalf("write p1: %v", err)
	}
	if err := os.WriteFile(p2, makeSolidPNG(t, 16, 16, color.RGBA{R: 20, G: 20, B: 200, A: 255}), 0o644); err != nil {
		t.Fatalf("write p2: %v", err)
	}

	imported, err := mgr.ImportFiles([]string{p1, p2})
	if err != nil || len(imported) != 2 {
		t.Fatalf("ImportFiles = %d, %v", len(imported), err)
	}
	target := imported[0]
	dragged := imported[1]

	linked, err := mgr.AddVariantFromFile(target.ID, dragged.FilePath)
	if err != nil {
		t.Fatalf("AddVariantFromFile on internal item path: %v", err)
	}
	if linked == nil || linked.ID != dragged.ID || linked.VariantOf != target.ID {
		t.Fatalf("linked = %+v, want ID=%s VariantOf=%s", linked, dragged.ID, target.ID)
	}

	parent, err := mgr.GetItem(target.ID)
	if err != nil {
		t.Fatalf("GetItem(target): %v", err)
	}
	if len(parent.Variants) != 1 || parent.Variants[0] != dragged.ID {
		t.Fatalf("parent.Variants = %v, want [%s]", parent.Variants, dragged.ID)
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1", "2", true},
		{"2", "10", true},
		{"10", "2", false},
		{"page1.png", "page2.png", true},
		{"page2.png", "page10.png", true},
		{"page10.png", "page2.png", false},
		{"第1页", "第2页", true},
		{"第2页", "第10页", true},
		{"第10页", "第2页", false},
		{"01.jpg", "2.jpg", true},
		{"fileA", "fileb", true},
		{"fileB", "fileA", false},
		{"same", "same", false},
	}
	for _, tc := range cases {
		if got := NaturalLess(tc.a, tc.b); got != tc.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReorderVariants(t *testing.T) {
	tmp := t.TempDir()
	mgr := &LibraryManager{}
	if _, err := mgr.CreateLibrary(tmp, "ReorderTest"); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	defer mgr.CloseLibrary()

	paths := make([]string, 4)
	for i := 1; i <= 4; i++ {
		p := filepath.Join(tmp, fmt.Sprintf("page%d.png", i))
		if err := os.WriteFile(p, makeSolidPNG(t, 10, 10, color.RGBA{R: uint8(i * 40), A: 255}), 0o644); err != nil {
			t.Fatalf("write p%d: %v", i, err)
		}
		paths[i-1] = p
	}

	imported, err := mgr.ImportFiles(paths)
	if err != nil || len(imported) != 4 {
		t.Fatalf("ImportFiles: %v", err)
	}

	p1 := imported[0].ID
	p2 := imported[1].ID
	p3 := imported[2].ID
	p4 := imported[3].ID

	// Link p2, p3, p4 to p1
	root, err := mgr.LinkVariants(p1, []string{p2, p3, p4})
	if err != nil {
		t.Fatalf("LinkVariants: %v", err)
	}
	if len(root.Variants) != 3 {
		t.Fatalf("initial root.Variants = %v, want 3 items", root.Variants)
	}

	// 1. Reorder variants while keeping p1 as primary: [p1, p4, p2, p3]
	reordered, err := mgr.ReorderVariants(p1, []string{p1, p4, p2, p3})
	if err != nil {
		t.Fatalf("ReorderVariants: %v", err)
	}
	if reordered.ID != p1 {
		t.Fatalf("reordered.ID = %s, want %s", reordered.ID, p1)
	}
	if len(reordered.Variants) != 3 || reordered.Variants[0] != p4 || reordered.Variants[1] != p2 || reordered.Variants[2] != p3 {
		t.Fatalf("reordered.Variants = %v, want [%s, %s, %s]", reordered.Variants, p4, p2, p3)
	}

	// 2. Reorder with new primary: p3 becomes primary, order [p3, p1, p4, p2]
	newRoot, err := mgr.ReorderVariants(p2, []string{p3, p1, p4, p2})
	if err != nil {
		t.Fatalf("ReorderVariants with new primary: %v", err)
	}
	if newRoot.ID != p3 || newRoot.VariantOf != "" {
		t.Fatalf("newRoot = id:%s variantOf:%q, want %s / empty", newRoot.ID, newRoot.VariantOf, p3)
	}
	if len(newRoot.Variants) != 3 || newRoot.Variants[0] != p1 || newRoot.Variants[1] != p4 || newRoot.Variants[2] != p2 {
		t.Fatalf("newRoot.Variants = %v, want [%s, %s, %s]", newRoot.Variants, p1, p4, p2)
	}

	// Check old primary p1 is now a variant of p3
	oldP1, _ := mgr.GetItem(p1)
	if oldP1.VariantOf != p3 || len(oldP1.Variants) != 0 {
		t.Fatalf("oldP1: variantOf=%s variants=%v", oldP1.VariantOf, oldP1.Variants)
	}

	// 3. Omitting an item keeps it at the end: pass only [p4, p3] -> p4 is primary, then p3, then remaining p1, p2
	omitted, err := mgr.ReorderVariants(p3, []string{p4, p3})
	if err != nil {
		t.Fatalf("ReorderVariants with omitted: %v", err)
	}
	if omitted.ID != p4 {
		t.Fatalf("omitted.ID = %s, want %s", omitted.ID, p4)
	}
	if len(omitted.Variants) != 3 || omitted.Variants[0] != p3 || omitted.Variants[1] != p1 || omitted.Variants[2] != p2 {
		t.Fatalf("omitted.Variants = %v, want [%s, %s, %s]", omitted.Variants, p3, p1, p2)
	}
}
