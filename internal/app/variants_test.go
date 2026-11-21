package app

import (
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
