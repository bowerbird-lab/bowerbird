package core

import (
	"context"
	"encoding/json"
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
