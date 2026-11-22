package app

import (
	"os"
	"path/filepath"
	"testing"

	"bowerbird/core"
)

func TestApplyVariantGrouping(t *testing.T) {
	items := []core.Item{
		{ID: "p1", Name: "original.png", Size: 100, Variants: []string{"v1", "v2"}},
		{ID: "v1", Name: "ai-edit.png", Size: 200, VariantOf: "p1"},
		{ID: "v2", Name: "ai-video.mp4", Size: 500, VariantOf: "p1"},
		{ID: "solo", Name: "standalone.md", Size: 50},
		{ID: "orphan-var", Name: "filtered-match.txt", Size: 30, VariantOf: "missing-parent"},
	}

	// Default collapsed ("stacked"): v1 and v2 are collapsed under p1; orphan-var stays visible.
	collapsed, collapsedSize := applyVariantGrouping(items, nil, "stacked")
	if len(collapsed) != 3 {
		t.Fatalf("collapsed len = %d, want 3", len(collapsed))
	}
	if collapsed[0].ID != "p1" || collapsed[1].ID != "solo" || collapsed[2].ID != "orphan-var" {
		t.Fatalf("unexpected collapsed order: %v, %v, %v", collapsed[0].ID, collapsed[1].ID, collapsed[2].ID)
	}
	if collapsedSize != 180 {
		t.Fatalf("collapsedSize = %d, want 180", collapsedSize)
	}

	// Expanded p1 in stacked mode: v1 and v2 appear immediately after p1.
	expanded, expandedSize := applyVariantGrouping(items, map[string]bool{"p1": true}, "stacked")
	if len(expanded) != 5 {
		t.Fatalf("expanded len = %d, want 5", len(expanded))
	}
	if expanded[0].ID != "p1" || expanded[1].ID != "v1" || expanded[2].ID != "v2" || expanded[3].ID != "solo" || expanded[4].ID != "orphan-var" {
		t.Fatalf("unexpected expanded order: %+v", []string{expanded[0].ID, expanded[1].ID, expanded[2].ID, expanded[3].ID, expanded[4].ID})
	}
	if expandedSize != 880 {
		t.Fatalf("expandedSize = %d, want 880", expandedSize)
	}

	// Flat mode: all items are shown side-by-side without collapsing.
	flat, flatSize := applyVariantGrouping(items, nil, "flat")
	if len(flat) != 5 || flatSize != 880 {
		t.Fatalf("flat = (%d, %d), want (5, 880)", len(flat), flatSize)
	}

	// workspaceSummary preserves Variants and VariantOf.
	sum := workspaceSummary(items[0])
	if len(sum.Variants) != 2 || sum.Variants[0] != "v1" || sum.Variants[1] != "v2" {
		t.Fatalf("workspaceSummary Variants = %v", sum.Variants)
	}
	sumChild := workspaceSummary(items[1])
	if sumChild.VariantOf != "p1" {
		t.Fatalf("workspaceSummary VariantOf = %q, want p1", sumChild.VariantOf)
	}
}

func TestDetectVariantGroupSort(t *testing.T) {
	// 1. Single item -> manual
	single := []core.Item{{ID: "1", Name: "only.png"}}
	if mode, desc := detectVariantGroupSort(single); mode != "manual" || desc {
		t.Errorf("detectVariantGroupSort(single) = (%s, %v), want (manual, false)", mode, desc)
	}

	// 2. Natural ascending order
	asc := []core.Item{
		{ID: "1", Name: "01_intro.png"},
		{ID: "2", Name: "2_body.png"},
		{ID: "3", Name: "10_outro.png"},
	}
	if mode, desc := detectVariantGroupSort(asc); mode != "name" || desc {
		t.Errorf("detectVariantGroupSort(asc) = (%s, %v), want (name, false)", mode, desc)
	}

	// 3. Natural descending order
	descItems := []core.Item{
		{ID: "3", Name: "10_outro.png"},
		{ID: "2", Name: "2_body.png"},
		{ID: "1", Name: "01_intro.png"},
	}
	if mode, desc := detectVariantGroupSort(descItems); mode != "name" || !desc {
		t.Errorf("detectVariantGroupSort(desc) = (%s, %v), want (name, true)", mode, desc)
	}

	// 4. Manual / unordered
	manual := []core.Item{
		{ID: "2", Name: "2_body.png"},
		{ID: "1", Name: "01_intro.png"},
		{ID: "3", Name: "10_outro.png"},
	}
	if mode, desc := detectVariantGroupSort(manual); mode != "manual" || desc {
		t.Errorf("detectVariantGroupSort(manual) = (%s, %v), want (manual, false)", mode, desc)
	}
}

func TestVariantPageReorderingFlow(t *testing.T) {
	tmp := t.TempDir()
	mgr := &core.LibraryManager{}
	svc := core.NewService(mgr)
	defer mgr.CloseLibrary()

	if _, err := svc.CreateLibrary(tmp, "AppReorderTest"); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	app := &App{svc: svc}

	// Create 3 items with distinct file contents so they are not deduped by content hash.
	p1 := filepath.Join(tmp, "10_last.txt")
	p2 := filepath.Join(tmp, "01_first.txt")
	p3 := filepath.Join(tmp, "02_middle.txt")
	_ = os.WriteFile(p1, []byte("content 1"), 0o644)
	_ = os.WriteFile(p2, []byte("content 2"), 0o644)
	_ = os.WriteFile(p3, []byte("content 3"), 0o644)

	importedList, err := svc.ImportFiles([]string{p1, p2, p3})
	if err != nil || len(importedList) != 3 {
		t.Fatalf("ImportFiles: %v (len=%d)", err, len(importedList))
	}
	it1 := importedList[0].ID
	it2 := importedList[1].ID
	it3 := importedList[2].ID

	// Link it2 and it3 to it1. Group initial order: [it1 (10_last), it2 (01_first), it3 (02_middle)]
	root, err := svc.LinkVariants(it1, []string{it2, it3})
	if err != nil {
		t.Fatalf("LinkVariants: %v", err)
	}
	if len(root.Variants) != 2 {
		t.Fatalf("root.Variants len = %d, want 2", len(root.Variants))
	}

	// 1. Sort Alphanumeric Ascending: should become [it2 (01_first), it3 (02_middle), it1 (10_last)]
	app.sortVariantGroupAlphanumeric(it1, false)

	// Since sortVariantGroupAlphanumeric dispatches reorderVariantPages in a goroutine,
	// test ReorderVariants synchronously through svc as well:
	ordered, err := svc.ReorderVariants(it1, []string{it2, it3, it1})
	if err != nil {
		t.Fatalf("ReorderVariants: %v", err)
	}
	if ordered.ID != it2 {
		t.Fatalf("ordered.ID = %s, want %s (01_first promoted to Cover)", ordered.ID, it2)
	}
	if len(ordered.Variants) != 2 || ordered.Variants[0] != it3 || ordered.Variants[1] != it1 {
		t.Fatalf("ordered.Variants = %v, want [%s, %s]", ordered.Variants, it3, it1)
	}

	// 2. Reverse order: should become [it1 (10_last), it3 (02_middle), it2 (01_first)]
	reversed, err := svc.ReorderVariants(it2, []string{it1, it3, it2})
	if err != nil {
		t.Fatalf("ReorderVariants reverse: %v", err)
	}
	if reversed.ID != it1 {
		t.Fatalf("reversed.ID = %s, want %s", reversed.ID, it1)
	}
	if len(reversed.Variants) != 2 || reversed.Variants[0] != it3 || reversed.Variants[1] != it2 {
		t.Fatalf("reversed.Variants = %v, want [%s, %s]", reversed.Variants, it3, it2)
	}
}

func TestVariantSortModeSwitchBackToManual(t *testing.T) {
	a := New(nil)
	items := []core.Item{
		{ID: "p1", Name: "01.png"},
		{ID: "p2", Name: "02.png"},
		{ID: "p3", Name: "03.png"},
	}

	// Default mode is manual even if items happen to be named in ascending order.
	if mode, desc := a.variantGroupSortStateFor("p1", items); mode != "manual" || desc {
		t.Fatalf("initial state = (%s, %v), want (manual, false)", mode, desc)
	}

	// Switching to name sort records the mode and manual snapshot.
	a.reorderVariantPagesWithSort("p1", []string{"p3", "p2", "p1"}, variantGroupSortState{
		Mode:        "name",
		Desc:        true,
		ManualOrder: []string{"p1", "p2", "p3"},
	})
	if mode, desc := a.variantGroupSortStateFor("p1", items); mode != "name" || !desc {
		t.Fatalf("after name desc = (%s, %v), want (name, true)", mode, desc)
	}

	// Switching back to manual resets mode to manual.
	a.switchVariantGroupToManual("p1")
	if mode, desc := a.variantGroupSortStateFor("p1", items); mode != "manual" || desc {
		t.Fatalf("after switch back to manual = (%s, %v), want (manual, false)", mode, desc)
	}
}
