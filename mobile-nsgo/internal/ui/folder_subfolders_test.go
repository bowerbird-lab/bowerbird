package ui

import (
	"testing"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
)

func TestCurrentUploadFolderIDMatchesDesktopLogic(t *testing.T) {
	a := &App{
		sidebarView:    SidebarFolder,
		activeFolderID: "folder-123",
	}
	if got := a.currentUploadFolderID(); got != "folder-123" {
		t.Fatalf("currentUploadFolderID in folder = %q, want folder-123", got)
	}

	for _, view := range []SidebarView{SidebarAll, SidebarUntagged, SidebarUncategorized, SidebarHistory, SidebarTrash} {
		a.sidebarView = view
		if got := a.currentUploadFolderID(); got != "" {
			t.Fatalf("currentUploadFolderID in %q = %q, want empty", view, got)
		}
	}
}

func TestBuildGalleryRowsIncludesAndCollapsesSubfolders(t *testing.T) {
	subs := []model.Folder{
		{ID: "c1", Name: "Sub 1", ItemCount: 3},
		{ID: "c2", Name: "Sub 2", ItemCount: 0},
		{ID: "c3", Name: "Sub 3", ItemCount: 5},
	}
	items := []model.ItemSummary{
		{ID: "i1", Name: "One"},
		{ID: "i2", Name: "Two"},
	}

	rows := BuildGalleryRows(subs, false, items, 2)
	if len(rows) != 4 {
		t.Fatalf("expanded rows len = %d, want 4", len(rows))
	}
	if rows[0].Kind != RowFolderHeader || rows[0].Title != "子文件夹 (3)" || rows[0].Collapsed {
		t.Fatalf("rows[0] = %+v, want expanded RowFolderHeader", rows[0])
	}
	if rows[1].Kind != RowFolderGroup || len(rows[1].Folders) != 2 || rows[1].Folders[0].ID != "c1" || rows[1].Folders[1].ID != "c2" {
		t.Fatalf("rows[1] = %+v, want 2 folders", rows[1])
	}
	if rows[2].Kind != RowFolderGroup || len(rows[2].Folders) != 1 || rows[2].Folders[0].ID != "c3" {
		t.Fatalf("rows[2] = %+v, want 1 folder", rows[2])
	}
	if rows[3].Kind != RowTileGroup || len(rows[3].Slots) != 2 {
		t.Fatalf("rows[3] = %+v, want 2 item slots", rows[3])
	}

	collapsedRows := BuildGalleryRows(subs, true, items, 2)
	if len(collapsedRows) != 2 {
		t.Fatalf("collapsed rows len = %d, want 2", len(collapsedRows))
	}
	if collapsedRows[0].Kind != RowFolderHeader || !collapsedRows[0].Collapsed {
		t.Fatalf("collapsedRows[0] = %+v, want collapsed RowFolderHeader", collapsedRows[0])
	}
	if collapsedRows[1].Kind != RowTileGroup {
		t.Fatalf("collapsedRows[1] = %+v, want RowTileGroup", collapsedRows[1])
	}
}

func TestBuildNativeSubfolderEntries(t *testing.T) {
	a := &App{
		sidebarView:    SidebarFolder,
		activeFolderID: "root",
		folders: []model.Folder{
			{
				ID:   "root",
				Name: "Root",
				Children: []model.Folder{
					{ID: "child-1", Name: "Child 1", ItemCount: 6},
					{ID: "child-2", Name: "Child 2", ItemCount: 0},
				},
			},
		},
		folderPreviews: map[string][]string{
			"child-1": {"u1", "u2", "u3", "u4", "u5"},
		},
	}

	entries := a.buildNativeSubfolderEntries()
	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3", len(entries))
	}
	if entries[0]["kind"] != "folder_header" || entries[0]["title"] != "子文件夹 (2)" || entries[0]["collapsed"] != false {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
	if entries[1]["kind"] != "folder" || entries[1]["id"] != "child-1" || entries[1]["count"] != 6 {
		t.Fatalf("entries[1] = %+v", entries[1])
	}
	thumbs, _ := entries[1]["thumbUrls"].([]string)
	if len(thumbs) != 4 {
		t.Fatalf("child-1 thumbUrls len = %d, want 4", len(thumbs))
	}

	a.subfoldersCollapsed = true
	collapsed := a.buildNativeSubfolderEntries()
	if len(collapsed) != 1 || collapsed[0]["collapsed"] != true {
		t.Fatalf("collapsed entries = %+v", collapsed)
	}
}
