package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
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

func TestFolderNavigationStackAndBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "Lib", PairingRequired: false, Capabilities: []string{"query"}})
		case "/api/v1/folders":
			_ = json.NewEncoder(w).Encode([]model.Folder{
				{
					ID: "folder-a", Name: "Folder A", ItemCount: 2,
					Children: []model.Folder{
						{ID: "folder-b", Name: "Folder B", ItemCount: 1},
					},
				},
			})
		case "/api/v1/query":
			folder := r.URL.Query().Get("folder")
			itemID := "root-item"
			switch folder {
			case "folder-a":
				itemID = "a-item"
			case "folder-b":
				itemID = "b-item"
			}
			_ = json.NewEncoder(w).Encode(model.ItemsPage{
				Total: 1,
				Items: []model.ItemSummary{{
					ID: itemID, Name: itemID, Filename: itemID + ".jpg",
					Extension: "jpg", MimeType: "image/jpeg", Size: 1024, Width: 400, Height: 400,
				}},
			})
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	local := storage.NewLocal(dir)
	if err := local.SaveSession(storage.Session{BaseURL: srv.URL, Token: "tok", LibraryID: "lib1", LibraryName: "Lib"}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	var appState *App
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		node := rootWithDir(cx, dir)
		appState = node.(*App)
		return node
	}, nsgotest.Size(420, 800))

	app.WaitFor("id:screen.gallery", 2*time.Second)
	app.WaitFor("id:gallery.tile", 2*time.Second)

	// Enter Folder A, wait for it to load, then enter subfolder Folder B.
	appState.scope.Post(func() {
		appState.selectFolder(appState.scope, "folder-a")
	})
	app.WaitFor("id:screen.folder.folder-a", 2*time.Second)
	app.Advance(50 * time.Millisecond)
	appState.scope.Post(func() {
		appState.selectFolder(appState.scope, "folder-b")
	})
	app.WaitFor("id:screen.folder.folder-b", 2*time.Second)
	app.Advance(50 * time.Millisecond)

	if got := len(appState.folderStack); got != 2 {
		t.Fatalf("folderStack len = %d, want 2", got)
	}
	if len(app.QueryMounted("id:screen.gallery")) == 0 || len(app.QueryMounted("id:screen.folder.folder-a")) == 0 {
		t.Fatal("lower screens should stay mounted behind top folder screen")
	}

	// Opening drawer while in Folder B: first Back closes drawer without popping Folder B.
	appState.scope.Post(func() {
		appState.setDrawerOpen(appState.scope, true)
	})
	app.Advance(300 * time.Millisecond)
	app.Back()
	app.Advance(300 * time.Millisecond)
	if appState.drawerOpen.Get() {
		t.Fatal("expected first Back to close drawer")
	}
	if got := len(appState.folderStack); got != 2 {
		t.Fatalf("folderStack len after closing drawer = %d, want 2", got)
	}

	// Next Back pops Folder B back to Folder A.
	app.Back()
	app.Advance(400 * time.Millisecond)
	if got := len(appState.folderStack); got != 1 {
		t.Fatalf("folderStack len after popping B = %d, want 1", got)
	}
	if appState.activeFolderID != "folder-a" || appState.sidebarView != SidebarFolder {
		t.Fatalf("after popping B: sidebarView=%q activeFolderID=%q", appState.sidebarView, appState.activeFolderID)
	}
	if len(app.QueryMounted("id:screen.folder.folder-b")) != 0 {
		t.Fatal("screen.folder.folder-b should unmount after pop")
	}

	// Next Back pops Folder A back to root gallery (without disconnecting).
	app.Back()
	app.Advance(400 * time.Millisecond)
	if got := len(appState.folderStack); got != 0 {
		t.Fatalf("folderStack len after popping A = %d, want 0", got)
	}
	if appState.screen.Get() != screenGallery {
		t.Fatalf("screen after popping A = %q, want %q", appState.screen.Get(), screenGallery)
	}
	if appState.sidebarView != SidebarAll || appState.activeFolderID != "" {
		t.Fatalf("after popping A: sidebarView=%q activeFolderID=%q", appState.sidebarView, appState.activeFolderID)
	}
	if app.Find("id:drawer.all") == nil || app.Find("text:Folder A") == nil {
		t.Fatal("sidebar drawer content should remain intact after popping back")
	}

	// Back at root gallery does not disconnect to devices screen.
	app.Back()
	app.Advance(200 * time.Millisecond)
	if appState.screen.Get() != screenGallery {
		t.Fatalf("screen after back at root = %q, want %q", appState.screen.Get(), screenGallery)
	}

	// Header back button also pops a pushed folder screen.
	appState.scope.Post(func() {
		appState.selectFolder(appState.scope, "folder-a")
	})
	app.WaitFor("id:screen.folder.folder-a", 2*time.Second)
	app.WaitFor("id:gallery.back", 2*time.Second)
	app.Press("id:gallery.back")
	app.Advance(400 * time.Millisecond)
	if got := len(appState.folderStack); got != 0 {
		t.Fatalf("folderStack len after pressing gallery.back = %d, want 0", got)
	}
}
