package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func TestPageGroupFoldingInRowsAndNativeEntries(t *testing.T) {
	items := []model.ItemSummary{
		{ID: "p1", Name: "Cover", Date: 1700000000000, Variants: []string{"v1", "v2"}},
		{ID: "v1", Name: "Page 2", Date: 1700000000000, VariantOf: "p1"},
		{ID: "v2", Name: "Page 3", Date: 1700000000000, VariantOf: "p1"},
		{ID: "solo", Name: "Solo", Date: 1700000000000},
		{ID: "orphan", Name: "Orphan Page", Date: 1700000000000, VariantOf: "missing"},
	}

	slots := visibleTileSlots(items)
	if len(slots) != 3 {
		t.Fatalf("visibleTileSlots len = %d, want 3", len(slots))
	}
	if slots[0].Item.ID != "p1" || slots[0].FlatIndex != 0 {
		t.Fatalf("slot[0] = %+v, want p1 at flatIndex 0", slots[0])
	}
	if slots[1].Item.ID != "solo" || slots[1].FlatIndex != 3 {
		t.Fatalf("slot[1] = %+v, want solo at flatIndex 3", slots[1])
	}
	if slots[2].Item.ID != "orphan" || slots[2].FlatIndex != 4 {
		t.Fatalf("slot[2] = %+v, want orphan at flatIndex 4", slots[2])
	}

	monthRows := BuildMonthRows(items, 3)
	if len(monthRows) != 2 || monthRows[0].Kind != RowMonthHeader || monthRows[0].Count != 3 {
		t.Fatalf("monthRows[0] = %+v, want header count 3", monthRows[0])
	}
	if len(monthRows[1].Slots) != 3 {
		t.Fatalf("monthRows[1].Slots len = %d, want 3", len(monthRows[1].Slots))
	}

	flatRows := BuildFlatRows(items, 2)
	if len(flatRows) != 2 || len(flatRows[0].Slots) != 2 || len(flatRows[1].Slots) != 1 {
		t.Fatalf("flatRows = %+v", flatRows)
	}

	pos, total, prevIdx, nextIdx := visibleGalleryPosition(items, 3)
	if pos != 1 || total != 3 || prevIdx != 0 || nextIdx != 4 {
		t.Fatalf("visibleGalleryPosition(3) = (%d, %d, %d, %d), want (1, 3, 0, 4)", pos, total, prevIdx, nextIdx)
	}

	a := &App{
		client:      &transport.Client{},
		selectedIDs: map[string]bool{},
	}
	entries := a.buildNativeGridEntries(items, true, screenSearch)
	if len(entries) != 4 {
		t.Fatalf("native entries len = %d, want 4 (1 header + 3 items)", len(entries))
	}
	if entries[0]["kind"] != "header" || entries[0]["count"] != 3 {
		t.Fatalf("entries[0] = %+v, want header count 3", entries[0])
	}
	if entries[1]["id"] != "p1" || entries[1]["pageCount"] != 3 || entries[1]["flatIndex"] != 0 {
		t.Fatalf("entries[1] = %+v, want p1 pageCount=3 flatIndex=0", entries[1])
	}
	if entries[2]["id"] != "solo" || entries[2]["pageCount"] != 0 || entries[2]["flatIndex"] != 3 {
		t.Fatalf("entries[2] = %+v, want solo pageCount=0 flatIndex=3", entries[2])
	}
	if entries[3]["id"] != "orphan" || entries[3]["isPageChild"] != true || entries[3]["flatIndex"] != 4 {
		t.Fatalf("entries[3] = %+v, want orphan isPageChild=true flatIndex=4", entries[3])
	}
}

func TestResolveDetailPagesFromGalleryAndCache(t *testing.T) {
	items := []model.ItemSummary{
		{ID: "p1", Name: "Cover", Variants: []string{"v1", "v2"}},
		{ID: "v1", Name: "Page 2", VariantOf: "p1"},
	}

	var pages []model.ItemSummary
	var pageIdx, clampedIdx int

	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := &App{
			scope:           cx,
			detailPageIndex: core.NewSignal(cx, 1),
			detailCache: map[string]model.ItemDetail{
				"p1": {ID: "p1", Name: "Cover", Variants: []string{"v1", "v2"}},
				"v1": {ID: "v1", Name: "Page 2", VariantOf: "p1"},
				"v2": {ID: "v2", Name: "Page 3 Cached", Extension: "png", VariantOf: "p1"},
			},
			detailLoading: map[string]bool{},
		}
		pages, pageIdx = a.resolveDetailPages(cx, items, 0)
		a.detailPageIndex.Set(99)
		_, clampedIdx = a.resolveDetailPages(cx, items, 0)
		return core.View{}
	}, nsgotest.Size(420, 800))

	if len(pages) != 3 {
		t.Fatalf("pages len = %d, want 3", len(pages))
	}
	if pages[0].ID != "p1" || pages[1].ID != "v1" || pages[2].ID != "v2" || pages[2].Name != "Page 3 Cached" {
		t.Fatalf("pages = %+v", pages)
	}
	if pageIdx != 1 {
		t.Fatalf("pageIdx = %d, want 1", pageIdx)
	}
	if clampedIdx != 2 {
		t.Fatalf("clampedIdx = %d, want 2", clampedIdx)
	}
}
