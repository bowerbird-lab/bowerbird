package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"bowerbird/core"
	"bowerbird/internal/plugin"
	"bowerbird/internal/theme"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func seededApp(t *testing.T) *App {
	t.Helper()
	a := New(nil)
	a.prefs = defaultPrefs()
	a.prefs.InspectorOpen = true
	a.prefs.SidebarOpen = true
	a.sidebarWidth = defaultSidebarWidth
	a.inspectorWidth = defaultInspectorWidth
	a.rowHeight = int(a.prefs.RowHeight)
	a.sortOrder = "desc"
	a.library = &core.LibraryInfo{ID: "lib", Name: "Library", Path: t.TempDir()}
	a.stats = core.WorkspaceStats{Total: 3, UntaggedCount: 1, TotalSize: 4096}
	a.folders = []core.Folder{{ID: "f1", Name: "Inspiration", ItemCount: 2}}
	a.tags = []core.Tag{{Name: "reference", ItemCount: 2}}
	a.items = []core.Item{
		{ID: "a", Name: "Alpha", Extension: "jpg", MimeType: "image/jpeg", Width: 1600, Height: 900, FilePath: "/tmp/a.jpg", Rating: 3, Tags: []string{"reference"}},
		{ID: "b", Name: "Beta", Extension: "png", MimeType: "image/png", Width: 900, Height: 1200, FilePath: "/tmp/b.png"},
		{ID: "c", Name: "Gamma", Extension: "mp3", MimeType: "audio/mpeg", Width: 0, Height: 0, FilePath: "/tmp/c.mp3"},
	}
	a.total = len(a.items)
	a.activeItem = &a.items[0]
	a.activeItemID = "a"
	a.selected = map[string]bool{"a": true}
	a.OnWriteFiles = func(paths []string) error { return nil }
	a.rebuildRows()
	return a
}

func TestMainViewRendersAndSettles(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	if tester.FramePending() {
		t.Fatal("idle main view keeps asking for frames")
	}
	for _, want := range []string{"Alpha", "Inspiration", "reference"} {
		if !tester.HasText(want) {
			t.Fatalf("main view is missing %q", want)
		}
	}
}

func TestSelectingItemUpdatesInspector(t *testing.T) {
	a := seededApp(t)
	a.chooseItem(1, 0)
	if a.activeItemID != "b" {
		t.Fatalf("active item = %q, want b", a.activeItemID)
	}
	if !a.selected["b"] || len(a.selected) != 1 {
		t.Fatalf("selection = %v", a.selected)
	}
	tester := ui.NewTester(a.viewMain, 1280, 800)
	if tester.FramePending() {
		t.Fatal("selection view keeps animating")
	}
	if !tester.HasText("Beta") {
		t.Fatal("selected item not shown")
	}
	// Direct click always single-selects.
	a.chooseItem(2, 0)
	if !a.selected["c"] || len(a.selected) != 1 {
		t.Fatalf("click selection = %v, want only c", a.selected)
	}
}
func TestInspectorShowsLibraryInfoWhenSelectionEmpty(t *testing.T) {
	a := seededApp(t)
	a.library.Name = "ArchiveLib"
	a.activeItem = nil
	a.activeItemID = ""
	a.selected = map[string]bool{}
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 800)
	if tester.FramePending() {
		t.Fatal("library inspector keeps animating")
	}
	for _, want := range []string{"ArchiveLib", "3", "4.0 KB"} {
		if !tester.HasText(want) {
			t.Fatalf("library inspector is missing %q", want)
		}
	}
	// The name is the header now; the "current library" caption is gone and
	// the path is a hover tooltip on the location row rather than a block.
	if tester.HasText(a.t("settings.storage.library")) {
		t.Fatal("library inspector still repeats the current library caption")
	}
	if tester.HasText(a.library.Path) {
		t.Fatal("library inspector shows the path without hovering")
	}
	r, ok := tester.Find(a.t("settings.storage.open"))
	if !ok {
		t.Fatal("library inspector has no open-in-file-manager action")
	}
	tester.Move(r.X+r.W/2, r.Y+r.H/2)
	tester.Advance(2 * time.Second)
	tester.Frame()
	if !tester.HasText(a.library.Path) {
		t.Fatal("hovering the location row does not show the library path")
	}
}

// TestInspectorHasCompactTopStrip keeps the inspector from reserving a full
// header row it has no controls for, while still leaving room for the window
// buttons on platforms that draw them on the right.
func TestInspectorHasCompactTopStrip(t *testing.T) {
	a := seededApp(t)
	a.library.Name = "ArchiveLib"
	a.activeItem = nil
	a.activeItemID = ""
	a.selected = map[string]bool{}
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 800)
	tester.SetTitleBar(ui.TitleBar{Height: 52, Left: 80})
	tester.Frame()
	mac, ok := tester.Find("ArchiveLib")
	if !ok {
		t.Fatal("library inspector is missing its title")
	}
	if mac.Y > headerHeight+20 {
		t.Fatalf("inspector content starts too low: y=%v", mac.Y)
	}

	// Windows-style controls reserve room on the right; the content must stay
	// below them instead of sliding under the buttons.
	tester.SetTitleBar(ui.TitleBar{Height: 52, Right: 138})
	tester.Frame()
	win, ok := tester.Find("ArchiveLib")
	if !ok {
		t.Fatal("library inspector is missing its title")
	}
	if win.Y <= mac.Y {
		t.Fatalf("inspector content did not move below the window buttons: mac y=%v win y=%v", mac.Y, win.Y)
	}
}

func TestArrowKeySelectionNavigation(t *testing.T) {
	a := seededApp(t)
	a.chooseItem(0, 0)
	if a.selectedAt != 0 {
		t.Fatalf("initial selectedAt = %d, want 0", a.selectedAt)
	}
	// Move Right in Justified layout
	a.moveGallerySelection(1, 0)
	if a.selectedAt != 1 {
		t.Fatalf("after right selectedAt = %d, want 1", a.selectedAt)
	}
	// Move Left
	a.moveGallerySelection(-1, 0)
	if a.selectedAt != 0 {
		t.Fatalf("after left selectedAt = %d, want 0", a.selectedAt)
	}
	// Test List mode navigation
	a.prefs.LayoutMode = "list"
	a.rebuildRows()
	a.moveGallerySelection(0, 1)
	if a.selectedAt != 1 {
		t.Fatalf("list down selectedAt = %d, want 1", a.selectedAt)
	}
	a.moveGallerySelection(0, -1)
	if a.selectedAt != 0 {
		t.Fatalf("list up selectedAt = %d, want 0", a.selectedAt)
	}
}

func TestSidebarModesRender(t *testing.T) {
	a := seededApp(t)
	for _, nav := range []Nav{NavAll, NavTags, NavTrash} {
		a.nav = nav
		tester := ui.NewTester(a.viewMain, 1100, 760)
		tester.Frame()
		if tester.FramePending() {
			t.Fatalf("%s view does not settle", nav)
		}
	}
}

func TestBlankMainViewWithoutLibrary(t *testing.T) {
	a := New(nil)
	if a.library != nil {
		t.Fatal("new app has a library")
	}
	tester := ui.NewTester(a.viewMain, 900, 600)
	if tester.FramePending() {
		t.Fatal("empty view keeps animating")
	}
}

func TestMainViewIsNotBlack(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	img := tester.Image()
	if img == nil {
		t.Fatal("no frame")
	}
	bright := 0
	for y := 0; y < img.Bounds().Dy(); y += 4 {
		for x := 0; x < img.Bounds().Dx(); x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8+g>>8+b>>8 > 60 {
				bright++
			}
		}
	}
	if bright < 1000 {
		t.Fatalf("frame looks black: only %d sampled pixels are visible", bright)
	}
}

func TestHeaderSitsAtTopAndGalleryScrolls(t *testing.T) {
	a := seededApp(t)
	a.items = nil
	for i := 0; i < 80; i++ {
		a.items = append(a.items, core.Item{ID: fmt.Sprintf("i%d", i), Name: fmt.Sprintf("Item %d", i), Extension: "jpg", MimeType: "image/jpeg", Width: 1600, Height: 1000})
	}
	a.total = len(a.items)
	a.activeItem = nil
	a.selected = map[string]bool{}
	a.rebuildRows()
	tester := ui.NewTester(a.viewMain, 1100, 760)
	rect, ok := tester.Find("Newest")
	if !ok {
		t.Fatal("header is missing")
	}
	if rect.Y > float32(headerHeight+42) {
		t.Fatalf("header is not at the top: y=%v", rect.Y)
	}
	if !tester.HasText("Item 0") {
		t.Fatal("first item is not visible")
	}
	tester.Scroll(650, 400, 0, 2400)
	tester.Frame()
	if tester.HasText("Item 0") {
		t.Fatal("gallery did not scroll: the first item is still visible")
	}
}
func TestLargeGalleryScrollsToItsRealEnd(t *testing.T) {
	a := seededApp(t)
	a.items = nil
	for i := 0; i < 30000; i++ {
		a.items = append(a.items, core.Item{
			ID: fmt.Sprintf("large-%d", i), Name: fmt.Sprintf("Large Item %d", i),
			Extension: "jpg", MimeType: "image/jpeg", Width: 1600, Height: 1000,
		})
	}
	a.total = len(a.items)
	a.activeItem = nil
	a.selected = map[string]bool{}
	a.rebuildRows()
	if len(a.rows) != len(a.items) {
		t.Fatalf("rows = %d, want one row per item (%d)", len(a.rows), len(a.items))
	}

	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()
	if a.galleryScroll.MaxY <= 0 {
		t.Fatal("gallery has no scroll range")
	}
	a.galleryScroll.Y = math.MaxFloat32
	tester.Frame()
	if a.galleryScroll.Y < a.galleryScroll.MaxY-1 {
		t.Fatalf("gallery did not reach its end: y=%v maxY=%v", a.galleryScroll.Y, a.galleryScroll.MaxY)
	}
	if !tester.HasText("Large Item 29999") {
		t.Fatal("the last item is not visible at the end of a large gallery")
	}
}

func TestLayoutModesSwitchAndRender(t *testing.T) {
	a := seededApp(t)
	for _, mode := range []string{"justified", "grid", "list"} {
		a.prefs.LayoutMode = mode
		a.rebuildRows()
		if len(a.rows) == 0 {
			t.Fatalf("layout %s produced no rows", mode)
		}
		tester := ui.NewTester(a.viewMain, 1280, 800)
		tester.Frame()
		if !tester.HasText("Alpha") {
			t.Fatalf("layout %s does not show Alpha", mode)
		}
	}
}

func TestRubberBandMarqueeSelect(t *testing.T) {
	a := seededApp(t)
	a.selected = map[string]bool{}
	a.rebuildRows()

	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	if !tester.HasText("Alpha") {
		t.Fatal("Alpha card not found")
	}
	// Start marquee from empty space below or beside the gallery cards and drag up
	tester.Press(800, 600)
	tester.Move(100, 50)
	tester.Release(100, 50)
	tester.Frame()
	if len(a.selected) == 0 {
		t.Fatal("marquee selection did not select any items")
	}
}

func TestDoubleclickOpensInlinePreviewAndHeaderChanges(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// Double-click triggers inline preview
	a.openPreview("a", "inline")
	tester.Frame()

	if !a.previewOpen || a.previewMode != "inline" {
		t.Fatalf("preview should be open in inline mode, got open=%v mode=%v", a.previewOpen, a.previewMode)
	}
	// In inline preview, sidebar and inspector remain, header is replaced with preview toolbar
	if !tester.HasText("Alpha") {
		t.Fatal("inline preview toolbar should show item title")
	}
	if !tester.HasText("Inspiration") {
		t.Fatal("sidebar should still be visible in inline preview mode")
	}
}

func TestWorkspaceHeaderToggleAlwaysShows(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.SetTitleBar(ui.TitleBar{Height: headerHeight, Left: 80})
	tester.Frame()

	// With the sidebar open, the shared header still offers its toggle at the
	// start of the workspace column.
	x := a.sidebarWidth + panelDividerWidth + gallerySidePadding + 15
	tester.ClickAt(x, headerHeight/2)
	tester.Frame()
	if a.prefs.SidebarOpen {
		t.Fatal("header toggle should collapse the open sidebar")
	}

	// Closed, the same button moves to the window edge past the traffic lights
	// and reopens it.
	tester.ClickAt(80+gallerySidePadding+15, headerHeight/2)
	tester.Frame()
	if !a.prefs.SidebarOpen {
		t.Fatal("header toggle should restore the collapsed sidebar")
	}
}

func TestInlinePreviewReusesWorkspaceHeader(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.SetTitleBar(ui.TitleBar{Height: headerHeight, Left: 80})
	tester.Frame()
	a.openPreview("a", "inline")
	tester.Frame()

	// The inline preview row is the shared header, so its sidebar toggle still
	// collapses the panel without leaving the preview.
	x := a.sidebarWidth + panelDividerWidth + gallerySidePadding + 15
	tester.ClickAt(x, headerHeight/2)
	tester.Frame()
	if a.prefs.SidebarOpen {
		t.Fatal("inline preview header lost its sidebar toggle")
	}
	if !a.previewOpen {
		t.Fatal("toggling the sidebar must keep the preview open")
	}

	// Close is the last control of the row, at the workspace's right edge.
	right := float32(1280) - a.inspectorWidth - panelDividerWidth
	tester.ClickAt(right-gallerySidePadding-15, headerHeight/2)
	tester.Frame()
	if a.previewOpen {
		t.Fatal("close button at the end of the header did not close the preview")
	}
}

func TestPreviewZoomSliderMapsLogarithmically(t *testing.T) {
	for _, zoom := range []float32{previewMinZoom, 0.5, 1, 2, previewMaxZoom} {
		if got := previewZoomFromTrack(previewZoomTrack(zoom)); math.Abs(float64(got-zoom)) > 0.001 {
			t.Fatalf("zoom %v maps to %v on the track and back to %v", zoom, previewZoomTrack(zoom), got)
		}
	}
	// 100% sits near the middle of the track instead of at its far left.
	if track := previewZoomTrack(1); track < 25 || track > 75 {
		t.Fatalf("100%% sits at %v on the track, want it near the middle", track)
	}
}

func TestSpaceKeyOpensFullWindowPreview(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// Space key triggers full-window preview
	tester.Key(0, ui.KeySpace)
	tester.Frame()

	if !a.previewOpen || a.previewMode != "full" {
		t.Fatalf("preview should be open in full mode after Space key, got open=%v mode=%v", a.previewOpen, a.previewMode)
	}

	// Pressing space again cancels/closes preview
	tester.Key(0, ui.KeySpace)
	tester.Frame()
	if a.previewOpen {
		t.Fatal("pressing Space again should close the preview")
	}
}

func TestGalleryClickAndDoubleClickCards(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// Gamma is not the active item, so its label is unique to the gallery.
	r, ok := tester.Find("Gamma")
	if !ok {
		t.Fatal("Gamma card not found")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2

	tester.ClickAt(x, y)
	tester.Frame()
	if !a.selected["c"] || len(a.selected) != 1 || a.activeItemID != "c" {
		t.Fatalf("click selected %v, active %q; want only c", a.selected, a.activeItemID)
	}
	if a.previewOpen {
		t.Fatal("single click must not open the preview")
	}

	tester.ClickAt(x, y)
	tester.Frame()
	if !a.previewOpen || a.previewMode != "inline" {
		t.Fatalf("double click should open inline preview, got open=%v mode=%v", a.previewOpen, a.previewMode)
	}
}

func TestItemDragDataFollowsSelection(t *testing.T) {
	a := seededApp(t)
	a.items[0].FileURL = "file:///tmp/a.jpg"
	a.items[1].FileURL = "file:///tmp/b.png"
	a.items[2].FileURL = "file:///tmp/c.mp3"

	readList := func(item core.Item) string {
		data := a.itemDragData(item)
		b, err := data.Read(transfer.FileList)
		if err != nil {
			t.Fatalf("read file list: %v", err)
		}
		return string(b)
	}

	// A single selection drags only the pressed item.
	if got := readList(a.items[0]); got != "file:///tmp/a.jpg" {
		t.Fatalf("single drag = %q, want a.jpg", got)
	}

	// Dragging a member of a multi-selection drags the whole selection.
	a.selected = map[string]bool{"a": true, "b": true}
	a.selectedOrder = []string{"a", "b"}
	if got := readList(a.items[1]); got != "file:///tmp/a.jpg\r\nfile:///tmp/b.png" {
		t.Fatalf("multi drag = %q, want a then b", got)
	}

	// Dragging an item outside the selection carries only that item.
	if got := readList(a.items[2]); got != "file:///tmp/c.mp3" {
		t.Fatalf("outside drag = %q, want c.mp3", got)
	}
}

func TestItemDragIDsFollowSelection(t *testing.T) {
	a := seededApp(t)

	// A single selection drags only the pressed item.
	if got := fmt.Sprint(a.itemDragIDs(a.items[0])); got != "[a]" {
		t.Fatalf("single drag IDs = %s, want [a]", got)
	}

	// Dragging a member of a multi-selection carries the whole selection in
	// gallery order.
	a.selected = map[string]bool{"a": true, "b": true}
	if got := fmt.Sprint(a.itemDragIDs(a.items[1])); got != "[a b]" {
		t.Fatalf("multi drag IDs = %s, want [a b]", got)
	}

	// Dragging an item outside the selection carries only that item.
	if got := fmt.Sprint(a.itemDragIDs(a.items[2])); got != "[c]" {
		t.Fatalf("outside drag IDs = %s, want [c]", got)
	}
}

func TestGalleryContextMenuTogglesSidebarAndInspector(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// Right-click the empty part of the gallery.
	tester.RightClickAt(600, 700)
	tester.Frame()
	if !tester.HasText(a.t("contextMenu.showSidebar")) && !contains(tester.Menu(), a.t("contextMenu.showSidebar")) {
		t.Fatalf("context menu is missing sidebar toggle: %v", tester.Menu())
	}
	if err := tester.ChooseMenuItem(a.t("contextMenu.showSidebar")); err != nil {
		t.Fatalf("choose sidebar toggle: %v", err)
	}
	if a.prefs.SidebarOpen {
		t.Fatal("sidebar toggle did not hide the sidebar")
	}
	tester.Frame()

	tester.RightClickAt(600, 700)
	tester.Frame()
	if err := tester.ChooseMenuItem(a.t("contextMenu.showInspector")); err != nil {
		t.Fatalf("choose inspector toggle: %v", err)
	}
	if a.prefs.InspectorOpen {
		t.Fatal("inspector toggle did not hide the inspector")
	}
}

func TestGalleryContextMenuCreatesSubfolderOnlyInsideFolders(t *testing.T) {
	smart := seededApp(t)
	smartTester := ui.NewTester(smart.viewMain, 1280, 800)
	smartTester.Frame()
	smartTester.RightClickAt(600, 700)
	smartTester.Frame()
	if contains(smartTester.Menu(), smart.t("contextMenu.newSubfolder")) {
		t.Fatalf("smart view context menu must not offer a subfolder: %v", smartTester.Menu())
	}

	a := seededApp(t)
	a.nav = NavFolder
	a.folderID = "f1"
	a.rebuildRows()
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()
	tester.RightClickAt(600, 700)
	tester.Frame()
	if !contains(tester.Menu(), a.t("contextMenu.newSubfolder")) {
		t.Fatalf("folder context menu is missing new subfolder: %v", tester.Menu())
	}
	if err := tester.ChooseMenuItem(a.t("contextMenu.newSubfolder")); err != nil {
		t.Fatalf("choose new subfolder: %v", err)
	}
	tester.Frame()
	if a.dialog != dialogNewFolder {
		t.Fatalf("dialog = %v, want new folder", a.dialog)
	}
	if a.newFolderParentID != "f1" {
		t.Fatalf("new folder parent = %q, want f1", a.newFolderParentID)
	}
}

func TestGalleryFolderCardUsesTheSidebarFolderContextMenu(t *testing.T) {
	a := seededApp(t)
	a.prefs.SidebarOpen = false
	a.folders = []core.Folder{
		{ID: "root", Name: "Root"},
		{ID: "child", Name: "Child", ParentID: "root"},
	}
	a.nav = NavFolder
	a.folderID = "root"
	a.rebuildRows()
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	if err := tester.RightClick("Child"); err != nil {
		t.Fatalf("right click folder card: %v", err)
	}
	tester.Frame()
	want := []string{
		a.t("common.rename"),
		a.t("sidebar.newSubfolderMenu"),
		"-",
		a.t("common.delete"),
	}
	if got := tester.Menu(); !slices.Equal(got, want) {
		t.Fatalf("folder card menu = %v, want the sidebar menu %v", got, want)
	}
	if err := tester.ChooseMenuItem(a.t("sidebar.newSubfolderMenu")); err != nil {
		t.Fatalf("choose new subfolder: %v", err)
	}
	tester.Frame()
	if a.dialog != dialogNewFolder {
		t.Fatalf("dialog = %v, want new folder", a.dialog)
	}
	if a.newFolderParentID != "child" {
		t.Fatalf("new folder parent = %q, want child", a.newFolderParentID)
	}
}

func TestGalleryFolderCardRenameKeepsTheCurrentFolder(t *testing.T) {
	a := seededApp(t)
	a.prefs.SidebarOpen = false
	a.folders = []core.Folder{
		{ID: "root", Name: "Root"},
		{ID: "child", Name: "Child", ParentID: "root"},
	}
	a.nav = NavFolder
	a.folderID = "root"
	a.rebuildRows()
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	if err := tester.RightClick("Child"); err != nil {
		t.Fatalf("right click folder card: %v", err)
	}
	tester.Frame()
	if err := tester.ChooseMenuItem(a.t("common.rename")); err != nil {
		t.Fatalf("choose rename: %v", err)
	}
	tester.Frame()

	if a.dialog != dialogRenameFolder {
		t.Fatalf("dialog = %v, want rename folder", a.dialog)
	}
	if a.nav != NavFolder || a.folderID != "root" {
		t.Fatalf("rename moved the workspace to nav=%q folder=%q, want root", a.nav, a.folderID)
	}
	if a.folderDialogID != "child" {
		t.Fatalf("rename target = %q, want child", a.folderDialogID)
	}
	if a.dialogValue != "Child" {
		t.Fatalf("rename value = %q, want Child", a.dialogValue)
	}
}

func TestEmptyGalleryKeepsDefaultContextMenu(t *testing.T) {
	a := seededApp(t)
	a.items = nil
	a.folders = nil
	a.total = 0
	a.totalSize = 0
	a.activeItem = nil
	a.activeItemID = ""
	a.selected = map[string]bool{}
	a.rebuildRows()

	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()
	tester.RightClickAt(600, 650)
	tester.Frame()
	if !contains(tester.Menu(), a.t("workspace.includeSubfolders")) {
		t.Fatalf("empty gallery context menu is missing subfolder toggle: %v", tester.Menu())
	}
	if err := tester.ChooseMenuItem(a.t("workspace.includeSubfolders")); err != nil {
		t.Fatalf("choose subfolder toggle: %v", err)
	}
	if !a.prefs.IncludeSubs {
		t.Fatal("subfolder toggle did not update the preference")
	}
}

func TestFolderContextMenuCreatesSubfolderUnderFolder(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()

	if err := tester.RightClick("Inspiration"); err != nil {
		t.Fatalf("right click folder: %v", err)
	}
	tester.Frame()
	if !contains(tester.Menu(), a.t("sidebar.newSubfolderMenu")) {
		t.Fatalf("folder context menu is missing new subfolder: %v", tester.Menu())
	}
	if err := tester.ChooseMenuItem(a.t("sidebar.newSubfolderMenu")); err != nil {
		t.Fatalf("choose new subfolder: %v", err)
	}
	tester.Frame()

	if a.dialog != dialogNewFolder {
		t.Fatalf("dialog = %v, want new folder", a.dialog)
	}
	if a.newFolderParentID != "f1" {
		t.Fatalf("new folder parent = %q, want f1", a.newFolderParentID)
	}
	if !tester.HasText(a.t("sidebar.newSubfolder")) {
		t.Fatalf("dialog is missing its subfolder title: %v", tester.Texts())
	}
}

func newFolderOrderApp(t *testing.T) (*App, *core.Service) {
	t.Helper()
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	mgr := core.NewLibraryManager()
	t.Cleanup(func() { _ = mgr.CloseLibrary() })
	if _, err := mgr.CreateLibrary(t.TempDir(), "FolderOrderLib"); err != nil {
		t.Fatalf("create library: %v", err)
	}
	svc := core.NewService(mgr)
	lib, err := svc.GetActiveLibrary()
	if err != nil || lib == nil {
		t.Fatalf("active library = %v, err=%v", lib, err)
	}
	a := New(svc)
	a.library = lib
	a.prefs.SidebarOpen = true
	a.prefs.InspectorOpen = false
	a.stats = core.WorkspaceStats{}
	a.tags = nil
	a.trashCount = 0
	return a, svc
}

func loadFolderTree(t *testing.T, a *App, svc *core.Service) []core.Folder {
	t.Helper()
	folders, err := svc.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	a.folders = folders
	return folders
}

func TestDraggingFolderReordersSiblings(t *testing.T) {
	a, svc := newFolderOrderApp(t)
	first, err := svc.CreateFolder("AAA", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateFolder("BBB", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateFolder("Child", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, a, svc)
	tester := ui.NewTester(a.viewMain, 900, 700)
	tester.Frame()

	src, ok := tester.Find("AAA")
	if !ok {
		t.Fatal("source folder row missing")
	}
	dst, ok := tester.Find("BBB")
	if !ok {
		t.Fatal("target folder row missing")
	}
	tester.Press(src.X+src.W/2, src.Y+src.H/2)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tester.Move(src.X+src.W/2+(dst.X-src.X)*f, src.Y+src.H/2+(dst.Y+dst.H+3-(src.Y+src.H/2))*f)
	}
	// The bottom edge of the target row reorders instead of nesting.
	tester.Release(dst.X+dst.W/2, dst.Y+dst.H+3)

	folders := loadFolderTree(t, a, svc)
	if len(folders) != 2 || folders[0].ID != second.ID || folders[1].ID != first.ID {
		t.Fatalf("root order after drag = %v, want BBB,AAA", folderNameList(folders))
	}
	if len(folders[1].Children) != 1 || folders[1].Children[0].ID != child.ID {
		t.Fatalf("child folder moved with the wrong parent: %v", folderNameList(folders[1].Children))
	}
}

func TestDraggingFolderNestsUnderAnotherFolder(t *testing.T) {
	a, svc := newFolderOrderApp(t)
	parent, err := svc.CreateFolder("Parent", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateFolder("Child", parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	moving, err := svc.CreateFolder("Moving", "")
	if err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, a, svc)
	tester := ui.NewTester(a.viewMain, 900, 700)
	tester.Frame()

	src, ok := tester.Find("Moving")
	if !ok {
		t.Fatal("source folder row missing")
	}
	dst, ok := tester.Find("Parent")
	if !ok {
		t.Fatal("target folder row missing")
	}
	tester.Press(src.X+src.W/2, src.Y+src.H/2)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tester.Move(src.X+src.W/2+(dst.X-src.X)*f, src.Y+src.H/2+(dst.Y+dst.H/2-(src.Y+src.H/2))*f)
	}
	// The middle of a sibling row nests the dragged folder under it.
	tester.Release(dst.X+dst.W/2, dst.Y+dst.H/2)

	folders := loadFolderTree(t, a, svc)
	if len(folders) != 1 || folders[0].ID != parent.ID {
		t.Fatalf("roots after nesting = %v, want only Parent", folderNameList(folders))
	}
	if len(folders[0].Children) != 2 {
		t.Fatalf("parent children = %v, want Child,Moving", folderNameList(folders[0].Children))
	}
	if folders[0].Children[0].ID != child.ID || folders[0].Children[1].ID != moving.ID {
		t.Fatalf("nested folder order = %v", folderNameList(folders[0].Children))
	}
	if !a.expanded[parent.ID] {
		t.Fatal("new parent was not expanded")
	}
}

func TestDraggingNestedFolderToFoldersHeaderReturnsToRoot(t *testing.T) {
	a, svc := newFolderOrderApp(t)
	parent, err := svc.CreateFolder("Parent", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateFolder("Child", parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	moving, err := svc.CreateFolder("Moving", parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, a, svc)
	a.expanded = map[string]bool{parent.ID: true}
	tester := ui.NewTester(a.viewMain, 900, 700)
	tester.Frame()

	src, ok := tester.Find("Moving")
	if !ok {
		t.Fatal("nested folder row missing")
	}
	header, ok := tester.Find(strings.ToUpper(a.t("sidebar.folders")))
	if !ok {
		t.Fatal("folders header missing")
	}
	tester.Press(src.X+src.W/2, src.Y+src.H/2)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tester.Move(src.X+src.W/2+(header.X+header.W/2-(src.X+src.W/2))*f, src.Y+src.H/2+(header.Y+header.H/2-(src.Y+src.H/2))*f)
	}
	tester.Release(header.X+header.W/2, header.Y+header.H/2)

	folders := loadFolderTree(t, a, svc)
	if len(folders) != 2 || folders[0].ID != parent.ID || folders[1].ID != moving.ID {
		t.Fatalf("roots after returning to root = %v, want Parent,Moving", folderNameList(folders))
	}
	if len(folders[0].Children) != 1 || folders[0].Children[0].ID != child.ID {
		t.Fatalf("old parent children = %v, want only Child", folderNameList(folders[0].Children))
	}
}

func folderNameList(folders []core.Folder) []string {
	names := make([]string, len(folders))
	for i := range folders {
		names[i] = folders[i].Name
	}
	return names
}

func TestClearWorkspaceSelectionResetsInspector(t *testing.T) {
	a := seededApp(t)
	a.activeItem = &a.items[0]
	a.activeItemID = a.items[0].ID
	a.selected = map[string]bool{a.items[0].ID: true}
	a.selectedOrder = []string{a.items[0].ID}
	a.inspectSections = []plugin.InspectSection{{Title: "stale"}}
	a.previewOpen = true

	a.clearWorkspaceSelection()
	if a.activeItem != nil || a.activeItemID != "" || len(a.selected) != 0 || len(a.selectedOrder) != 0 {
		t.Fatalf("selection was not cleared: item=%v id=%q selected=%v order=%v", a.activeItem, a.activeItemID, a.selected, a.selectedOrder)
	}
	if len(a.inspectSections) != 0 || a.previewOpen {
		t.Fatal("inspector or preview state survived a library switch")
	}
}

func TestFilterBarPinnedAndActiveCriteria(t *testing.T) {
	a := seededApp(t)

	// The old web default keeps type, color, folder and orientation pinned.
	want := []string{filterCriterionType, filterCriterionColor, filterCriterionFolder, filterCriterionOrientation}
	if got := a.visibleFilterCriteria(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("default visible filters = %v, want %v", got, want)
	}

	// An active unpinned criterion stays visible so it can be changed/cleared.
	a.filters.Sizes = []string{"small"}
	want = append(want, filterCriterionSize)
	if got := a.visibleFilterCriteria(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("visible filters with active size = %v, want %v", got, want)
	}

	// Unpinning removes the criterion unless it is active.
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a.toggleFilterPin(filterCriterionType)
	if got := a.visibleFilterCriteria(); contains(got, filterCriterionType) {
		t.Fatalf("unpinned inactive type is still visible: %v", got)
	}
}

func TestFilterQueryStringIncludesAllCriteria(t *testing.T) {
	a := seededApp(t)
	a.query = "sunset"
	a.filters = Filters{
		Types:        []string{"image"},
		Colors:       []string{"#EF4444"},
		Folders:      []string{"f1"},
		Ratings:      []int{5},
		Orientations: []string{"landscape"},
		Sizes:        []string{"small"},
		Has:          []string{"url"},
		No:           []string{"note"},
	}
	want := "sunset type:image color:#EF4444 folder:f1 rating:5 orientation:landscape size:small has:url no:note"
	if got := a.queryString(); got != want {
		t.Fatalf("queryString = %q, want %q", got, want)
	}

	// Unrated is an additive query flag, not a rating:0 token.
	a.filters.Ratings = nil
	a.filters.Unrated = true
	if got := a.queryString(); contains([]string{got}, "rating:") {
		t.Fatalf("unrated must not emit a rating token: %q", got)
	}
}

func TestSmartViewNavScopesQuery(t *testing.T) {
	a := seededApp(t)
	a.query = "sunset"
	cases := []struct {
		nav  Nav
		want string
	}{
		{NavAll, "sunset"},
		{NavUntagged, "sunset is:untagged"},
		{NavUncategorized, "sunset is:uncategorized"},
	}
	for _, tc := range cases {
		a.nav = tc.nav
		if got := a.queryString(); got != tc.want {
			t.Fatalf("nav %s queryString = %q, want %q", tc.nav, got, tc.want)
		}
	}
}

func TestSidebarFolderTreeRendersChildrenAfterParent(t *testing.T) {
	a := seededApp(t)
	a.folders = []core.Folder{{
		ID: "root-folder", Name: "RootFolder", ItemCount: 4,
		Children: []core.Folder{{
			ID: "child-folder", Name: "ChildFolder", ParentID: "root-folder", ItemCount: 2,
			Children: []core.Folder{
				{ID: "grand-folder", Name: "GrandFolder", ParentID: "child-folder"},
			},
		}},
	}}
	a.expanded = map[string]bool{"root-folder": true, "child-folder": true}
	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()
	root, okRoot := tester.Find("RootFolder")
	child, okChild := tester.Find("ChildFolder")
	grand, okGrand := tester.Find("GrandFolder")
	if !okRoot || !okChild || !okGrand {
		t.Fatalf("folder tree rows missing: root=%v child=%v grand=%v", okRoot, okChild, okGrand)
	}
	if !(root.Y < child.Y && child.Y < grand.Y) {
		t.Fatalf("folder tree order root=%v child=%v grand=%v", root.Y, child.Y, grand.Y)
	}
	if !(root.X < child.X && child.X < grand.X) {
		t.Fatalf("folder tree indentation root=%v child=%v grand=%v", root.X, child.X, grand.X)
	}

	a.expanded = map[string]bool{"root-folder": true}
	tester.Frame()
	if tester.HasText("GrandFolder") {
		t.Fatal("collapsed child folder still renders its descendants")
	}
}

func TestGalleryShowsSubfoldersBeforeItems(t *testing.T) {
	a := seededApp(t)
	a.nav = NavFolder
	a.folderID = "parent-folder"
	a.folders = []core.Folder{{
		ID:   "parent-folder",
		Name: "ParentFolder",
		Children: []core.Folder{
			{ID: "child-folder", Name: "ChildFolder", ParentID: "parent-folder", ItemCount: 7},
		},
	}}
	a.folderPreviews = map[string]core.FolderPreview{
		"child-folder": {Total: 7},
	}
	a.rebuildRows()
	if len(a.rows) < 2 || !a.rows[0][0].folderHeader || a.rows[1][0].folderID != "child-folder" {
		t.Fatalf("folder section was not placed first: %+v", a.rows[:min(len(a.rows), 2)])
	}

	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()
	folderRect, okFolder := tester.Find("ChildFolder")
	itemRect, okItem := tester.Find("Alpha")
	if !okFolder || !okItem {
		t.Fatalf("gallery missing folder or item: folder=%v item=%v", okFolder, okItem)
	}
	if folderRect.Y >= itemRect.Y {
		t.Fatalf("folder card Y=%v is not above item Y=%v", folderRect.Y, itemRect.Y)
	}

	a.subfoldersCollapsed = true
	a.rebuildRows()
	tester.Frame()
	if tester.HasText("ChildFolder") {
		t.Fatal("collapsed subfolder section still shows folder cards")
	}
}

func TestLibraryPickerKeepsStableOrderAndFilters(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := seededApp(t)
	a.recent = []core.LibraryRef{
		{ID: "b", Path: "/tmp/Blue.library"},
		{ID: "lib", Path: a.library.Path},
		{ID: "c", Path: "/tmp/Charcoal.library"},
	}
	a.prefs.LibraryOrder = []string{"c", "b", "lib"}

	got := a.libraryRefsInOrder()
	var ids []string
	for _, ref := range got {
		ids = append(ids, ref.ID)
	}
	if strings.Join(ids, ",") != "c,b,lib" {
		t.Fatalf("library order = %v, want c,b,lib", ids)
	}

	a.librarySearch = "blue"
	filtered := a.filteredLibraryRefs(got)
	if len(filtered) != 2 || filtered[0].ID != "b" || filtered[1].ID != "lib" {
		t.Fatalf("filtered libraries = %+v, want Blue and active library", filtered)
	}

	a.reorderLibraries([]int{0}, 3)
	if strings.Join(a.prefs.LibraryOrder, ",") != "b,lib,c" {
		t.Fatalf("reordered libraries = %v, want b,lib,c", a.prefs.LibraryOrder)
	}
}

func TestLibraryPickerRendersSearchAndActions(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := seededApp(t)
	a.recent = []core.LibraryRef{
		{ID: "lib", Path: a.library.Path},
		{ID: "blue", Path: "/tmp/Blue.library"},
	}
	a.libraryPickerOpen = true
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()
	if !tester.HasText(a.t("sidebar.createLibrary.action")) || !tester.HasText(a.t("sidebar.openLibrary")) {
		t.Fatal("library picker is missing its actions")
	}
	if !tester.HasText("Blue") {
		t.Fatal("library picker is missing a recent library")
	}
}

func TestSettingsSectionLabelsAreTranslated(t *testing.T) {
	a := seededApp(t)
	for _, label := range a.settingsSectionLabels() {
		if strings.HasPrefix(label, "settings.") {
			t.Fatalf("settings navigation shows a raw translation key: %q", label)
		}
	}
	for _, title := range []string{
		a.t("settings.vibrancy.title"),
		a.t("settings.vibrancy.enable"),
		a.t("settings.shortcuts.title"),
		a.t("settings.language.title"),
	} {
		if strings.HasPrefix(title, "settings.") {
			t.Fatalf("settings shows a raw translation key: %q", title)
		}
	}
}

func TestVibrancyDetailsFollowMasterSwitch(t *testing.T) {
	a := seededApp(t)
	a.settingsSection = 1 // Appearance
	tester := ui.NewTester(a.viewSettings, 1000, 700)
	tester.SetVibrancy(true)
	a.vibrancyValue = false
	tester.Frame()
	if tester.HasText(a.t("settings.vibrancy.sidebar")) || tester.HasText(a.t("settings.vibrancy.titlebar")) {
		t.Fatal("detailed blur options are shown while the material is off")
	}
	a.vibrancyValue = true
	tester.Frame()
	if !tester.HasText(a.t("settings.vibrancy.sidebar")) || !tester.HasText(a.t("settings.vibrancy.titlebar")) {
		t.Fatal("detailed blur options are missing while the material is on")
	}
}

func TestVibrancyMaterialRequiresMasterSwitch(t *testing.T) {
	a := seededApp(t)
	shows := false
	tester := ui.NewTester(func(c *ui.Context) { shows = a.vibrancyShows(c) }, 100, 100)
	tester.SetVibrancy(true)
	a.vibrancyValue = true
	tester.Frame()
	if !shows {
		t.Fatal("material does not show while the master switch is on")
	}
	a.vibrancyValue = false
	tester.Frame()
	if shows {
		t.Fatal("material still shows after the master switch was turned off")
	}
}

func TestHeaderBlurStartsWhenContentReachesHeader(t *testing.T) {
	a := seededApp(t)
	a.galleryScroll.Y = 0
	if a.headerHasContentUnder() {
		t.Fatal("header blurred before the gallery reached it")
	}
	a.galleryScroll.Y = galleryTopPadding
	if a.headerHasContentUnder() {
		t.Fatal("header blurred while content only reached its bottom edge")
	}
	a.galleryScroll.Y = galleryTopPadding + 1
	if !a.headerHasContentUnder() {
		t.Fatal("header did not blur once content scrolled under it")
	}
}

func TestSettingsHeaderBlurStartsWhenContentReachesHeader(t *testing.T) {
	a := seededApp(t)
	a.settingsScroll.Y = 0
	if a.settingsHeaderHasContentUnder() {
		t.Fatal("settings header blurred before content reached it")
	}
	a.settingsScroll.Y = settingsContentTopPadding
	if a.settingsHeaderHasContentUnder() {
		t.Fatal("settings header blurred while content only reached its bottom edge")
	}
	a.settingsScroll.Y = settingsContentTopPadding + 1
	if !a.settingsHeaderHasContentUnder() {
		t.Fatal("settings header did not blur once content scrolled under it")
	}
}

func TestThemesSettingsPageRenders(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := seededApp(t)
	a.settingsSection = 2
	tester := ui.NewTester(a.viewSettings, 1000, 800)
	tester.Frame()
	for _, want := range []string{
		a.t("settings.themes.selection"),
		a.t("settings.themes.light"),
		a.t("settings.themes.dark"),
		a.t("settings.themes.edit"),
	} {
		if !tester.HasText(want) {
			t.Fatalf("theme page is missing %q", want)
		}
	}
	if tester.FramePending() {
		t.Fatal("theme page keeps asking for frames")
	}
}

func TestShippedThemesKeepTheCurrentPalette(t *testing.T) {
	builtins, err := theme.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*theme.Theme{}
	for _, th := range builtins {
		byID[th.ID] = th
	}
	light := paletteFor(byID["default-light"])
	dark := paletteFor(byID["default-dark"])
	checks := []struct {
		name string
		got  ui.Color
		want ui.Color
	}{
		{"light bg", light.bg, ui.Hex("#ffffff")},
		{"light sidebar", light.sidebar, ui.Hex("#f5f5f7")},
		{"light text", light.text, ui.Hex("#18181b")},
		{"dark bg", dark.bg, ui.Hex("#141416")},
		{"dark sidebar", dark.sidebar, ui.Hex("#1b1b1e")},
		{"dark text", dark.text, ui.Hex("#f4f4f5")},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Fatalf("%s = %s, want %s", c.name, theme.FormatColor(c.got), theme.FormatColor(c.want))
		}
	}
}

func TestPaletteFollowsSystemAppearance(t *testing.T) {
	a := seededApp(t)
	var got palette
	tester := ui.NewTester(func(c *ui.Context) { got = a.palette(c) }, 100, 100)
	tester.SetDark(true)
	tester.Frame()
	if got.bg != ui.Hex("#141416") {
		t.Fatalf("system dark bg = %s", theme.FormatColor(got.bg))
	}
	tester.SetDark(false)
	tester.Frame()
	if got.bg != ui.Hex("#ffffff") {
		t.Fatalf("system light bg = %s", theme.FormatColor(got.bg))
	}
}

func TestAccentTextPicksContrast(t *testing.T) {
	if got := accentText(ui.RGB(255, 255, 0)); got != ui.RGB(0, 0, 0) {
		t.Fatalf("text on a light accent = %s, want black", theme.FormatColor(got))
	}
	if got := accentText(ui.RGB(20, 20, 60)); got != ui.RGB(255, 255, 255) {
		t.Fatalf("text on a dark accent = %s, want white", theme.FormatColor(got))
	}
}

func TestSystemAccentOverridesThemeAccent(t *testing.T) {
	a := seededApp(t)
	var got palette
	tester := ui.NewTester(func(c *ui.Context) {
		a.syncSystemAccent(c)
		got = a.palette(c)
	}, 100, 100)
	tester.SetPreferences(ui.Preferences{Accent: ui.RGB(255, 0, 0)})
	tester.Frame()
	if got.accent != ui.RGB(255, 0, 0) {
		t.Fatalf("accent = %s, want the system red", theme.FormatColor(got.accent))
	}
	if got.onAccent != ui.RGB(255, 255, 255) {
		t.Fatalf("text on the system accent = %s, want white", theme.FormatColor(got.onAccent))
	}

	a.prefs.SystemAccent = false
	tester.Frame()
	if got.accent == ui.RGB(255, 0, 0) {
		t.Fatal("accent still uses the system colour after the switch was turned off")
	}
}

func TestUserThemeSelectionAppliesPalette(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := seededApp(t)
	base := *a.themeIndex["default-light"]
	src := &theme.Theme{ID: "mine-light", Appearance: theme.Light, Colors: base.Colors, Material: base.Material}
	src.Colors.Accent = "#ff0000"
	src.Builtin = false
	if err := a.writeThemeFile(src); err != nil {
		t.Fatal(err)
	}
	a.loadThemes()
	if a.themeIndex["mine-light"] == nil {
		t.Fatal("user theme was not loaded")
	}
	a.prefs.LightTheme = "mine-light"
	a.applyThemeSelection()
	var got palette
	tester := ui.NewTester(func(c *ui.Context) { got = a.palette(c) }, 100, 100)
	tester.Frame()
	if got.accent != ui.Hex("#ff0000") {
		t.Fatalf("accent = %s, want #ff0000", theme.FormatColor(got.accent))
	}
	if got.bg != ui.Hex("#ffffff") {
		t.Fatalf("missing tokens did not fall back: bg = %s", theme.FormatColor(got.bg))
	}
}

func TestDuplicateBuiltinThemeCreatesEditableCopy(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := seededApp(t)
	builtin := a.themeIndex["default-dark"]
	if builtin == nil {
		t.Fatal("default-dark is missing")
	}
	a.duplicateTheme(builtin)
	if a.prefs.DarkTheme == "default-dark" {
		t.Fatal("duplicate did not select the new dark theme")
	}
	if _, err := os.Stat(filepath.Join(a.themesDir(), a.prefs.DarkTheme+".json")); err != nil {
		t.Fatalf("duplicate file: %v", err)
	}
	if a.themeIndex["default-dark"] == nil {
		t.Fatal("built-in default-dark disappeared")
	}
}

func TestThemeSelectionPersistsAcrossRestart(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := New(nil)
	base := *a.themeIndex["default-light"]
	custom := &theme.Theme{ID: "mine-light", Appearance: theme.Light, Colors: base.Colors, Material: base.Material}
	custom.Colors.Accent = "#ff0000"
	if err := a.writeThemeFile(custom); err != nil {
		t.Fatal(err)
	}
	a.loadThemes()
	a.prefs.LightTheme = "mine-light"
	a.prefs.DarkTheme = "midnight-dark"
	a.applyThemeSelection()

	// Simulate a fresh process: the selection and the user theme both have to
	// come back from disk.
	b := New(nil)
	if b.prefs.LightTheme != "mine-light" {
		t.Fatalf("light theme after restart = %q, want mine-light", b.prefs.LightTheme)
	}
	if b.prefs.DarkTheme != "midnight-dark" {
		t.Fatalf("dark theme after restart = %q, want midnight-dark", b.prefs.DarkTheme)
	}
	if got := b.themeByID("mine-light", theme.Light); got.Colors.Accent != "#ff0000" {
		t.Fatalf("restored theme accent = %q, want #ff0000", got.Colors.Accent)
	}
}

func TestMissingThemeSelectionFallsBackToDefault(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	a := New(nil)
	base := *a.themeIndex["default-light"]
	custom := &theme.Theme{ID: "mine-light", Appearance: theme.Light, Colors: base.Colors, Material: base.Material}
	if err := a.writeThemeFile(custom); err != nil {
		t.Fatal(err)
	}
	a.loadThemes()
	a.prefs.LightTheme = "mine-light"
	a.applyThemeSelection()

	// The theme file is deleted (or becomes invalid) between launches.
	if err := os.Remove(filepath.Join(a.themesDir(), "mine-light.json")); err != nil {
		t.Fatal(err)
	}
	b := New(nil)
	if b.prefs.LightTheme != "default-light" {
		t.Fatalf("light theme after losing the file = %q, want default-light", b.prefs.LightTheme)
	}
	if got := b.themeByID(b.prefs.LightTheme, theme.Light).ID; got != "default-light" {
		t.Fatalf("painted theme = %q, want default-light", got)
	}
}

func TestCorruptPrefsFallBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWERBIRD_USER_DATA_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "native-ui.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(nil)
	if a.prefs.LightTheme != "default-light" || a.prefs.DarkTheme != "default-dark" {
		t.Fatalf("themes after corrupt prefs = %q/%q, want the defaults", a.prefs.LightTheme, a.prefs.DarkTheme)
	}
	if mode := a.themeMode(); mode != core.ThemeSystem {
		t.Fatalf("theme mode after corrupt prefs = %q, want system", mode)
	}
}

func TestNormalizeDroppedURLs(t *testing.T) {
	raw := []string{
		"https://www.pinterest.com/pin/123/\r\nhttps://pin.it/AbCdEf",
		"pin.it/NoScheme",
		"not a url",
		"file:///tmp/a.jpg",
		"https://www.pinterest.com/pin/123/",
	}
	got := normalizeDroppedURLs(raw)
	want := []string{
		"https://www.pinterest.com/pin/123/",
		"https://pin.it/AbCdEf",
		"https://pin.it/NoScheme",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("normalizeDroppedURLs = %v, want %v", got, want)
	}
	urls, files := splitDroppedPaths([]string{"https://pin.it/x", "/tmp/photo.jpg"})
	if len(urls) != 1 || len(files) != 1 {
		t.Fatalf("splitDroppedPaths urls=%v files=%v", urls, files)
	}
}

func TestHeaderRightPaddingOnInspectorCollapsed(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.SetTitleBar(ui.TitleBar{Height: 52, Right: 138})

	// Collapse inspector. Main header now reaches the right edge of the window.
	a.prefs.InspectorOpen = false
	tester.Frame()

	if tester.FramePending() {
		t.Fatal("unexpected pending frame after rendering header with collapsed inspector")
	}
}

func TestSidebarSeamDragsItsWidth(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	start := a.sidebarWidth
	seam := start + 0.5
	tester.Press(seam, 300)
	tester.Move(seam+48, 300)
	tester.Release(seam+48, 300)
	tester.Frame()

	if got, want := a.sidebarWidth, start+48; math.Abs(float64(got-want)) > 1 {
		t.Fatalf("sidebar width after a 48px drag = %v, want %v", got, want)
	}
	if a.prefs.SidebarWidth != float64(a.sidebarWidth) {
		t.Fatalf("sidebar pref = %v, want the dragged %v", a.prefs.SidebarWidth, a.sidebarWidth)
	}

	// A drag past the minimum stops there instead of hiding the sidebar.
	seam = a.sidebarWidth + 0.5
	tester.Press(seam, 300)
	tester.Move(seam-600, 300)
	tester.Release(seam-600, 300)
	tester.Frame()
	if a.sidebarWidth != minSidebarWidth {
		t.Fatalf("sidebar width after a drag past the minimum = %v, want %v", a.sidebarWidth, minSidebarWidth)
	}

	// Dragging right stops at the panel's own maximum.
	seam = a.sidebarWidth + 0.5
	tester.Press(seam, 300)
	tester.Move(1279, 300)
	tester.Release(1279, 300)
	tester.Frame()
	if a.sidebarWidth != maxSidebarWidth {
		t.Fatalf("sidebar width after a drag past the maximum = %v, want %v", a.sidebarWidth, maxSidebarWidth)
	}
	if got := a.workspaceWidth(1280); got < mainMinWidth-1 {
		t.Fatalf("workspace width after the sidebar drag = %v, want at least %v", got, mainMinWidth)
	}
}

func TestInspectorSeamDragsItsWidth(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	start := a.inspectorWidth
	seam := 1280 - start - 0.5
	tester.Press(seam, 300)
	tester.Move(seam-40, 300)
	tester.Release(seam-40, 300)
	tester.Frame()

	if got, want := a.inspectorWidth, start+40; math.Abs(float64(got-want)) > 1 {
		t.Fatalf("inspector width after dragging its seam left = %v, want %v", got, want)
	}
	if a.prefs.InspectorWidth != float64(a.inspectorWidth) {
		t.Fatalf("inspector pref = %v, want the dragged %v", a.prefs.InspectorWidth, a.inspectorWidth)
	}

	// Dragging left stops where the workspace meets its own minimum.
	seam = 1280 - a.inspectorWidth - 0.5
	tester.Press(seam, 300)
	tester.Move(1, 300)
	tester.Release(1, 300)
	tester.Frame()
	if got := a.workspaceWidth(1280); math.Abs(float64(got-mainMinWidth)) > 1 {
		t.Fatalf("workspace width after a drag past its minimum = %v, want %v", got, mainMinWidth)
	}
}

func TestNarrowWindowTrimsPanelsForWorkspace(t *testing.T) {
	a := seededApp(t)
	a.sidebarWidth = maxSidebarWidth
	a.inspectorWidth = maxInspectorWidth
	a.prefs.SidebarWidth = float64(a.sidebarWidth)
	a.prefs.InspectorWidth = float64(a.inspectorWidth)

	tester := ui.NewTester(a.viewMain, 1000, 800)
	tester.Frame()

	if got := a.workspaceWidth(1000); got < mainMinWidth-1 {
		t.Fatalf("workspace width in a 1000px window = %v, want at least %v", got, mainMinWidth)
	}
	if a.inspectorWidth != minInspectorWidth {
		t.Fatalf("inspector width = %v, want the minimum %v after giving way", a.inspectorWidth, minInspectorWidth)
	}
}

func TestGalleryCtrlWheelZoom(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	initHeight := a.rowHeight

	// Without modifier, it does not zoom.
	taken := a.handleGalleryZoomInput(ui.InputEvent{Kind: ui.InputScroll, DY: -80})
	if taken || a.rowHeight != initHeight {
		t.Fatalf("rowHeight changed without modifier: taken=%v, height=%d", taken, a.rowHeight)
	}

	zoomMod := ui.Ctrl
	if runtime.GOOS == "darwin" {
		zoomMod = ui.Super
	}

	// Wheel with modifier zooms in (DY < 0 increases row height).
	taken = a.handleGalleryZoomInput(ui.InputEvent{Kind: ui.InputScroll, DY: -80, Mods: zoomMod})
	if !taken || a.rowHeight <= initHeight {
		t.Fatalf("rowHeight did not increase on modifier+Wheel Up: taken=%v got=%d, want > %d", taken, a.rowHeight, initHeight)
	}

	zoomedIn := a.rowHeight
	// Wheel with modifier zooms out (DY > 0 decreases row height).
	taken = a.handleGalleryZoomInput(ui.InputEvent{Kind: ui.InputScroll, DY: 80, Mods: zoomMod})
	if !taken || a.rowHeight >= zoomedIn {
		t.Fatalf("rowHeight did not decrease on modifier+Wheel Down: taken=%v got=%d, want < %d", taken, a.rowHeight, zoomedIn)
	}

	// Pinch gesture through Tester also triggers zoom.
	beforePinch := a.rowHeight
	tester.Pinch(400, 300, 1.25)
	tester.Frame()
	if a.rowHeight <= beforePinch {
		t.Fatalf("Pinch gesture did not zoom: got %d, want > %d", a.rowHeight, beforePinch)
	}
}

func TestCopyItemsShortcutAndAction(t *testing.T) {
	a := seededApp(t)
	var copiedPaths []string
	a.OnWriteFiles = func(paths []string) error {
		copiedPaths = paths
		return nil
	}
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// 1. Single selection copy (item "a")
	a.selected = map[string]bool{"a": true}
	a.activeItemID = "a"
	a.copySelected()
	if a.toast != a.t("common.copied") {
		t.Fatalf("toast = %q, want %q", a.toast, a.t("common.copied"))
	}
	if len(copiedPaths) != 1 || !strings.HasSuffix(copiedPaths[0], "a.jpg") {
		t.Fatalf("copiedPaths = %v, want [ .../a.jpg ]", copiedPaths)
	}

	// 2. Multi selection copy (items "a" and "b")
	a.selected = map[string]bool{"a": true, "b": true}
	a.toast = ""
	copiedPaths = nil
	a.copySelected()
	if a.toast != a.t("common.copied") {
		t.Fatalf("multi-item copy toast = %q, want %q", a.toast, a.t("common.copied"))
	}
	if len(copiedPaths) != 2 {
		t.Fatalf("copiedPaths count = %d, want 2: %v", len(copiedPaths), copiedPaths)
	}

	// 3. Trigger via KeyC shortcut through Tester
	a.toast = ""
	copiedPaths = nil
	tester.Key(ui.Cmd, ui.KeyC)
	tester.Frame()
	if a.toast != a.t("common.copied") {
		t.Fatalf("Cmd+C shortcut toast = %q, want %q", a.toast, a.t("common.copied"))
	}
	if len(copiedPaths) != 2 {
		t.Fatalf("shortcut copiedPaths count = %d, want 2: %v", len(copiedPaths), copiedPaths)
	}
}

func TestShiftAndCtrlMultiSelectionAcrossLayouts(t *testing.T) {
	a := seededApp(t)

	// --- 1. Test Ctrl / Cmd toggle selection ---
	a.chooseItem(0, 0)
	if !a.selected["a"] || len(a.selected) != 1 {
		t.Fatalf("expected only 'a' selected, got %v", a.selected)
	}
	// Ctrl+Click 'b': add 'b'
	a.chooseItem(1, ui.Ctrl)
	if !a.selected["a"] || !a.selected["b"] || len(a.selected) != 2 {
		t.Fatalf("expected 'a' and 'b' selected, got %v", a.selected)
	}
	// Ctrl+Click 'a': toggle 'a' off
	a.chooseItem(0, ui.Ctrl)
	if a.selected["a"] || !a.selected["b"] || len(a.selected) != 1 {
		t.Fatalf("expected only 'b' selected after toggling 'a', got %v", a.selected)
	}
	// Ctrl+Click 'a': toggle 'a' back on
	a.chooseItem(0, ui.Ctrl)
	if !a.selected["a"] || !a.selected["b"] || len(a.selected) != 2 {
		t.Fatalf("expected 'a' and 'b' selected after toggling 'a' back, got %v", a.selected)
	}

	// --- 2. Test List layout Shift range selection (1D consecutive) ---
	a.prefs.LayoutMode = "list"
	a.rebuildRows()
	// Click 'a' (index 0) without modifiers -> sets anchor to 0
	a.chooseItem(0, 0)
	// Shift+Click 'c' (index 2) -> selects [0, 1, 2]
	a.chooseItem(2, ui.Shift)
	if len(a.selected) != 3 || !a.selected["a"] || !a.selected["b"] || !a.selected["c"] {
		t.Fatalf("list shift selection expected 3 items (a, b, c), got %v", a.selected)
	}

	// --- 3. Test Grid layout Shift range selection (2D matrix block) ---
	a.prefs.LayoutMode = "grid"
	// Setup 6 items: 2 rows of 3 columns
	a.items = []core.Item{
		{ID: "0", Name: "Item 0"}, {ID: "1", Name: "Item 1"}, {ID: "2", Name: "Item 2"},
		{ID: "3", Name: "Item 3"}, {ID: "4", Name: "Item 4"}, {ID: "5", Name: "Item 5"},
	}
	a.rows = [][]galleryCell{
		{{index: 0, width: 100, height: 100}, {index: 1, width: 100, height: 100}, {index: 2, width: 100, height: 100}},
		{{index: 3, width: 100, height: 100}, {index: 4, width: 100, height: 100}, {index: 5, width: 100, height: 100}},
	}
	// Click (row 0, col 0) index 0 -> anchor = 0
	a.chooseItem(0, 0)
	// Shift+Click (row 1, col 1) index 4 -> 2D block [row 0..1] x [col 0..1] = {0, 1, 3, 4}
	a.chooseItem(4, ui.Shift)
	if len(a.selected) != 4 || !a.selected["0"] || !a.selected["1"] || !a.selected["3"] || !a.selected["4"] {
		t.Fatalf("grid 2D shift block selection expected {0, 1, 3, 4}, got %v", a.selected)
	}
	if a.selected["2"] || a.selected["5"] {
		t.Fatalf("items in column 2 should not be selected in [0..1]x[0..1] grid box")
	}

	// --- 4. Test Justified layout Shift range selection ---
	a.prefs.LayoutMode = "justified"
	a.rows = [][]galleryCell{
		{{index: 0, width: 120, height: 100}, {index: 1, width: 180, height: 100}, {index: 2, width: 140, height: 100}},
		{{index: 3, width: 200, height: 100}, {index: 4, width: 140, height: 100}, {index: 5, width: 100, height: 100}},
	}
	a.chooseItem(0, 0)
	a.chooseItem(4, ui.Shift)
	if len(a.selected) < 2 || !a.selected["0"] || !a.selected["4"] {
		t.Fatalf("justified shift selection must at least include endpoints 0 and 4, got %v", a.selected)
	}
}

// TestGalleryTakesFocusForSelectionKeys keeps the gallery keys working: Delete
// and Backspace act only while the focus is inside the gallery, so keyboard
// selection has to move the focus there (a text field keeps them otherwise).
func TestGalleryTakesFocusForSelectionKeys(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	tester.Key(0, ui.KeyRight)
	tester.Frame()
	tester.Frame()

	if active, _ := tester.TextInputActive(); active {
		t.Fatal("arrow keys left the focus in a text field")
	}
	if !tester.Focused("Alpha") {
		t.Fatal("arrow keys did not move the focus into the gallery")
	}
}

// TestPreviewArrowKeysStepThroughItems keeps the film strip keyboard
// navigable: a modal preview overlay makes the window's shortcuts wait, so the
// full preview registers the arrow keys on the overlay itself. The inline
// preview is no overlay and keeps using the window shortcuts.
func TestPreviewArrowKeysStepThroughItems(t *testing.T) {
	t.Run("full", func(t *testing.T) {
		a := seededApp(t)
		tester := ui.NewTester(a.viewMain, 1280, 800)
		tester.Frame()

		tester.Key(0, ui.KeySpace)
		tester.Frame()
		if !a.previewOpen || a.previewMode != "full" {
			t.Fatalf("preview = (open=%v, mode=%s), want the full overlay", a.previewOpen, a.previewMode)
		}

		tester.Key(0, ui.KeyRight)
		tester.Frame()
		if a.previewIndex != 1 || a.activeItemID != "b" {
			t.Fatalf("Right moved to (index=%d, active=%s), want b", a.previewIndex, a.activeItemID)
		}

		tester.Key(0, ui.KeyLeft)
		tester.Frame()
		if a.previewIndex != 0 || a.activeItemID != "a" {
			t.Fatalf("Left moved to (index=%d, active=%s), want a", a.previewIndex, a.activeItemID)
		}

		// The strip wraps around at the ends.
		tester.Key(0, ui.KeyLeft)
		tester.Frame()
		if a.previewIndex != len(a.items)-1 {
			t.Fatalf("Left from the first item landed on %d, want %d", a.previewIndex, len(a.items)-1)
		}
	})

	t.Run("inline", func(t *testing.T) {
		a := seededApp(t)
		tester := ui.NewTester(a.viewMain, 1280, 800)
		tester.Frame()

		a.openPreview("a", "inline")
		tester.Frame()

		tester.Key(0, ui.KeyRight)
		tester.Frame()
		if a.previewIndex != 1 || a.activeItemID != "b" {
			t.Fatalf("inline Right moved to (index=%d, active=%s), want b", a.previewIndex, a.activeItemID)
		}
	})
}

func TestPreviewUpDownKeysStepThroughPages(t *testing.T) {
	setupMultiPageApp := func(t *testing.T) *App {
		t.Helper()
		a := seededApp(t)
		p1 := a.items[0]
		p1.Variants = []string{"v1", "v2"}
		a.items[0] = p1
		a.activeItem = &a.items[0]
		a.variantGroupCacheID = p1.ID
		a.variantGroupPrimary = &p1
		a.variantGroupItems = []core.Item{
			{ID: "v1", Name: "Page 2", VariantOf: p1.ID, Extension: "png"},
			{ID: "v2", Name: "Page 3", VariantOf: p1.ID, Extension: "png"},
		}
		return a
	}

	t.Run("full", func(t *testing.T) {
		a := setupMultiPageApp(t)
		tester := ui.NewTester(a.viewMain, 1280, 800)
		tester.Frame()

		tester.Key(0, ui.KeySpace)
		tester.Frame()

		tester.Key(0, ui.KeyDown)
		tester.Frame()
		if a.activeItemID != "v1" || a.previewVariantID != "v1" {
			t.Fatalf("Down moved to (active=%s, variant=%s), want v1", a.activeItemID, a.previewVariantID)
		}

		tester.Key(0, ui.KeyDown)
		tester.Frame()
		if a.activeItemID != "v2" || a.previewVariantID != "v2" {
			t.Fatalf("second Down moved to (active=%s, variant=%s), want v2", a.activeItemID, a.previewVariantID)
		}

		// Down from the last page wraps to the primary page.
		tester.Key(0, ui.KeyDown)
		tester.Frame()
		if a.activeItemID != "a" || a.previewVariantID != "a" {
			t.Fatalf("wrap Down moved to (active=%s, variant=%s), want a", a.activeItemID, a.previewVariantID)
		}

		// Up from the primary page wraps to the last page.
		tester.Key(0, ui.KeyUp)
		tester.Frame()
		if a.activeItemID != "v2" || a.previewVariantID != "v2" {
			t.Fatalf("Up from primary moved to (active=%s, variant=%s), want v2", a.activeItemID, a.previewVariantID)
		}

		// Left/Right still moves across workspace items and clears variant selection.
		tester.Key(0, ui.KeyRight)
		tester.Frame()
		if a.previewIndex != 1 || a.activeItemID != "b" || a.previewVariantID != "" {
			t.Fatalf("Right moved to (index=%d, active=%s, variant=%s), want (1, b, '')", a.previewIndex, a.activeItemID, a.previewVariantID)
		}

		// Up/Down on a single-page item is a no-op.
		tester.Key(0, ui.KeyDown)
		tester.Frame()
		if a.previewIndex != 1 || a.activeItemID != "b" || a.previewVariantID != "" {
			t.Fatalf("Down on single-page item changed state to (index=%d, active=%s, variant=%s)", a.previewIndex, a.activeItemID, a.previewVariantID)
		}
	})

	t.Run("inline", func(t *testing.T) {
		a := setupMultiPageApp(t)
		tester := ui.NewTester(a.viewMain, 1280, 800)
		tester.Frame()

		a.openPreview("a", "inline")
		tester.Frame()

		tester.Key(0, ui.KeyDown)
		tester.Frame()
		if a.activeItemID != "v1" || a.previewVariantID != "v1" {
			t.Fatalf("inline Down moved to (active=%s, variant=%s), want v1", a.activeItemID, a.previewVariantID)
		}

		tester.Key(0, ui.KeyUp)
		tester.Frame()
		if a.activeItemID != "a" || a.previewVariantID != "a" {
			t.Fatalf("inline Up moved to (active=%s, variant=%s), want a", a.activeItemID, a.previewVariantID)
		}
	})
}

func TestStripFitCountAdaptsToWidth(t *testing.T) {
	cases := []struct {
		width float32
		want  int
	}{
		{0, 1},
		{100, 1},
		{300, 4},
		{1280, 21},
		{2400, 41},
	}
	for _, tc := range cases {
		if got := stripFitCount(tc.width); got != tc.want {
			t.Errorf("stripFitCount(%v) = %d, want %d", tc.width, got, tc.want)
		}
	}
}

func TestStripWindowCentresAndClamps(t *testing.T) {
	cases := []struct {
		index, total, fit int
		start, end        int
	}{
		{0, 3, 15, 0, 3},    // fewer items than cells: all shown
		{0, 100, 10, 0, 10}, // at the start: no empty cells
		{50, 100, 10, 45, 55},
		{99, 100, 10, 90, 100}, // at the end
		{5, 10, 0, 5, 6},       // a degenerate width still shows the item
	}
	for _, tc := range cases {
		start, end := stripWindow(tc.index, tc.total, tc.fit)
		if start != tc.start || end != tc.end {
			t.Errorf("stripWindow(%d, %d, %d) = (%d, %d), want (%d, %d)",
				tc.index, tc.total, tc.fit, start, end, tc.start, tc.end)
		}
		if tc.index < start || tc.index >= end {
			t.Errorf("stripWindow(%d, %d, %d) leaves the current item out", tc.index, tc.total, tc.fit)
		}
	}
}

// TestShortcutRecorder covers the settings recorder: keys show as they are
// pressed, Save applies the combination, Esc cancels, and No shortcut clears
// the binding. Conflicts keep the recorder open with the reason.
func TestShortcutRecorder(t *testing.T) {
	newSettings := func(t *testing.T) (*App, *ui.Tester) {
		t.Helper()
		a := seededApp(t)
		a.settingsSection = 2
		a.shortcuts = core.DefaultShortcutSettings()
		a.rebuildShortcutPairs()
		return a, ui.NewTester(a.viewSettings, 1000, 700)
	}
	click := func(t *testing.T, tester *ui.Tester, label string) {
		t.Helper()
		r, ok := tester.Find(label)
		if !ok {
			t.Fatalf("no %q button", label)
		}
		tester.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		tester.Frame()
	}

	t.Run("save", func(t *testing.T) {
		a, tester := newSettings(t)
		a.openShortcutDialog(core.ShortcutOpenSettings)
		tester.Frame()

		tester.Key(ui.Cmd, ui.KeyK)
		tester.Frame()
		if a.shortcutDraft != "CmdOrCtrl+K" {
			t.Fatalf("draft = %q, want CmdOrCtrl+K", a.shortcutDraft)
		}

		click(t, tester, a.t("common.save"))
		if a.shortcutEditID != "" {
			t.Fatal("recorder stayed open after saving")
		}
		if got := a.shortcuts.Accelerator(core.ShortcutOpenSettings); got != "CmdOrCtrl+K" {
			t.Fatalf("saved accelerator = %q, want CmdOrCtrl+K", got)
		}
	})

	t.Run("escape cancels", func(t *testing.T) {
		a, tester := newSettings(t)
		a.openShortcutDialog(core.ShortcutOpenSettings)
		tester.Frame()

		tester.Key(ui.Cmd, ui.KeyK)
		tester.Frame()
		tester.Key(0, ui.KeyEscape)
		tester.Frame()

		if a.shortcutEditID != "" {
			t.Fatal("Esc left the recorder open")
		}
		if got := a.shortcuts.Accelerator(core.ShortcutOpenSettings); got == "CmdOrCtrl+K" {
			t.Fatal("Esc saved the recorded combination")
		}
	})

	t.Run("no shortcut", func(t *testing.T) {
		a, tester := newSettings(t)
		a.openShortcutDialog(core.ShortcutLogs)
		tester.Frame()

		click(t, tester, a.t("settings.shortcuts.unsetAction"))
		if a.shortcutEditID != "" {
			t.Fatal("recorder stayed open after clearing")
		}
		if got := a.shortcuts.Accelerator(core.ShortcutLogs); got != "" {
			t.Fatalf("accelerator = %q, want it unset", got)
		}
		// Saving the same map again keeps it unset.
		if err := core.ValidateShortcutSettings(a.shortcuts); err != nil {
			t.Fatalf("unset shortcuts must not conflict: %v", err)
		}
	})

	t.Run("conflict warns while recording", func(t *testing.T) {
		a, tester := newSettings(t)
		a.openShortcutDialog(core.ShortcutOpenSettings)
		tester.Frame()

		// CmdOrCtrl+L already opens the logs: the warning shows on the key
		// press, before Save is anywhere near.
		tester.Key(ui.Cmd, ui.KeyL)
		tester.Frame()
		if a.shortcutEditError == "" {
			t.Fatal("a conflict was not reported as the keys came in")
		}
		if a.shortcutDraft != "CmdOrCtrl+L" {
			t.Fatalf("draft = %q, want the pressed combination", a.shortcutDraft)
		}

		// Saving keeps the recorder open with the warning and changes nothing.
		click(t, tester, a.t("common.save"))
		if a.shortcutEditID == "" {
			t.Fatal("a conflicting combination closed the recorder")
		}
		if got := a.shortcuts.Accelerator(core.ShortcutOpenSettings); got == "CmdOrCtrl+L" {
			t.Fatal("a conflicting combination was saved")
		}

		// A free combination clears the warning right away.
		tester.Key(ui.Cmd, ui.KeyK)
		tester.Frame()
		if a.shortcutEditError != "" {
			t.Fatalf("warning stayed after a free combination: %q", a.shortcutEditError)
		}
	})
}

func TestSyncLanguageAppliesPersistedPreference(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWERBIRD_USER_DATA_DIR", dir)
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	if err := os.WriteFile(filepath.Join(dir, "user_data.json"), []byte(`{"language":"zh"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := core.NewLibraryManager()
	t.Cleanup(func() { _ = mgr.CloseLibrary() })

	// Mirror the desktop startup order: the app is constructed before the
	// manager is attached, then the language is re-read in WhenReady.
	svc := core.NewService(nil)
	a := New(svc)
	if got := a.Translator().Language(); got != "en" {
		t.Fatalf("pre-attach language = %q, want en (system default)", got)
	}
	core.AttachManager(svc, mgr)
	a.SyncLanguage()
	if got := a.Translator().Language(); got != "zh" {
		t.Fatalf("language after attach = %q, want zh", got)
	}
	if got := a.Translator().T("settings.languageSystem"); got != "跟随系统" {
		t.Fatalf("translated label = %q, want 跟随系统", got)
	}
}

func TestSettingsLanguageRowPutsDropdownRightAndHelpInTooltip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWERBIRD_USER_DATA_DIR", dir)
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	mgr := core.NewLibraryManager()
	t.Cleanup(func() { _ = mgr.CloseLibrary() })

	svc := core.NewService(nil)
	core.AttachManager(svc, mgr)
	a := New(svc)
	a.settingsSection = 0
	a.refreshSettings()

	tester := ui.NewTester(a.viewSettings, 900, 700)
	tester.Frame()

	label, ok := tester.Find(a.t("settings.language.title"))
	if !ok {
		t.Fatalf("language label %q is missing", a.t("settings.language.title"))
	}
	value, ok := tester.Find(a.t("settings.language.auto"))
	if !ok {
		t.Fatalf("language dropdown %q is missing", a.t("settings.language.auto"))
	}
	if value.X <= label.X {
		t.Fatalf("dropdown starts at %.1f, label at %.1f: control is not on the right", value.X, label.X)
	}
	if diff := value.Y + value.H/2 - (label.Y + label.H/2); diff < -12 || diff > 12 {
		t.Fatalf("dropdown and label are not on one row (y offset %.1f)", diff)
	}
	for _, key := range []string{
		"settings.language.description",
		"settings.lightweight.description",
		"settings.recordURL.description",
	} {
		if tester.HasText(a.t(key)) {
			t.Fatalf("%s is rendered inline instead of a tooltip", key)
		}
	}
}

// TestInspectorListsOnlyTheItemFolders keeps the details panel to the folders
// the item actually lives in: the full tree belongs to the picker behind the
// plus, not to the panel.
func TestInspectorListsOnlyTheItemFolders(t *testing.T) {
	a := seededApp(t)
	a.folders = []core.Folder{
		{ID: "f1", Name: "Inspiration", ItemCount: 2},
		{ID: "f2", Name: "Client Work", ItemCount: 4},
	}
	a.activeItem.Folders = []string{"f1"}
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 1200)
	if !tester.HasText("Inspiration") {
		t.Fatalf("inspector is missing the folder the item lives in: %q", tester.Texts())
	}
	if tester.HasText("Client Work") {
		t.Fatal("inspector lists a folder the item is not in")
	}
}

// TestFolderPickerTreeSearchesAndAdds covers the shared picker: it draws the
// folder tree, its search keeps a match with the parent that places it, and a
// click adds the target and closes it.
func TestFolderPickerTreeSearchesAndAdds(t *testing.T) {
	a := seededApp(t)
	a.folders = []core.Folder{
		{ID: "f1", Name: "Inspiration", ItemCount: 2},
		{ID: "f2", Name: "Client Work", ItemCount: 4},
		{ID: "f3", Name: "Work 2024", ParentID: "f2"},
	}
	a.activeItem.Folders = []string{"f1"}
	a.openFolderPicker([]core.Item{*a.activeItem})
	if a.dialog != dialogFolderPicker {
		t.Fatalf("the picker did not open: dialog=%v", a.dialog)
	}
	open := true
	tester := ui.NewTester(func(c *ui.Context) { a.viewFolderPickerDialog(c, &open) }, 420, 700)
	for _, want := range []string{"Inspiration", "Client Work"} {
		if !tester.HasText(want) {
			t.Fatalf("picker is missing %q: %q", want, tester.Texts())
		}
	}

	// The search keeps a match and the ancestors that place it in the tree.
	a.folderPickerQuery = "2024"
	tester.Frame()
	if !tester.HasText("Work 2024") || !tester.HasText("Client Work") {
		t.Fatalf("search dropped the match or its parent: %q", tester.Texts())
	}
	if tester.HasText("Inspiration") {
		t.Fatalf("search kept a folder that does not match: %q", tester.Texts())
	}

	row, ok := tester.Find("Work 2024")
	if !ok {
		t.Fatalf("the match has no row: %q", tester.Texts())
	}
	tester.Press(row.X+row.W/2, row.Y+row.H/2)
	tester.Release(row.X+row.W/2, row.Y+row.H/2)
	tester.Frame()
	if a.dialog != dialogNone || len(a.folderPickerItems) != 0 {
		t.Fatalf("the picker stayed open after a folder was chosen: dialog=%v items=%d", a.dialog, len(a.folderPickerItems))
	}
}

// TestFolderPickerExpansionIsIndependentOfTheSidebar keeps the picker's tree
// and the sidebar's from sharing one expansion map: opening the picker starts
// from its own state, and folding a folder there leaves the sidebar as it was.
func TestFolderPickerExpansionIsIndependentOfTheSidebar(t *testing.T) {
	a := seededApp(t)
	a.folders = []core.Folder{
		{ID: "f1", Name: "Inspiration", ItemCount: 2},
		{ID: "f2", Name: "Client Work", ItemCount: 4},
		{ID: "f3", Name: "Work 2024", ParentID: "f2"},
	}
	// The sidebar has f2 open; the picker has never expanded it.
	a.expanded["f2"] = true
	a.openFolderPicker([]core.Item{*a.activeItem})
	open := true
	tester := ui.NewTester(func(c *ui.Context) { a.viewFolderPickerDialog(c, &open) }, 420, 700)
	if tester.HasText("Work 2024") {
		t.Fatal("the picker opened with the sidebar's expansion instead of its own")
	}

	// The arrow on the left of the parent row opens it in the picker only.
	// Find gives the name's box: the row pads 6, the arrow is 16 wide, then a
	// 6 gap, the 13 icon and another 6 gap put the name 33 past the arrow's
	// middle.
	row, ok := tester.Find("Client Work")
	if !ok {
		t.Fatalf("the parent row is missing: %q", tester.Texts())
	}
	tester.Press(row.X-33, row.Y+row.H/2)
	tester.Release(row.X-33, row.Y+row.H/2)
	tester.Frame()
	if !tester.HasText("Work 2024") {
		t.Fatalf("the picker's own arrow did not expand the folder: %q", tester.Texts())
	}
	if !a.expanded["f2"] {
		t.Fatal("the picker's expansion leaked into the sidebar's state")
	}
}

// TestTagPickerShowsAllTagsAndCreatesTheTypedOne covers the shared tag picker:
// it lists every tag by default, narrows with the search, offers the typed name
// when no tag has it, and adds the clicked tag.
func TestTagPickerShowsAllTagsAndCreatesTheTypedOne(t *testing.T) {
	a := seededApp(t)
	a.tags = []core.Tag{
		{Name: "reference", ItemCount: 2},
		{Name: "mood", ItemCount: 5},
	}
	a.activeItem.Tags = []string{"reference"}
	a.openTagPicker([]core.Item{*a.activeItem})
	if a.dialog != dialogTagPicker {
		t.Fatalf("the tag picker did not open: dialog=%v", a.dialog)
	}
	open := true
	tester := ui.NewTester(func(c *ui.Context) { a.viewTagPickerDialog(c, &open) }, 420, 700)
	for _, want := range []string{"reference", "mood"} {
		if !tester.HasText(want) {
			t.Fatalf("the picker does not list %q by default: %q", want, tester.Texts())
		}
	}

	// The search narrows the list, and offers the typed name as a new tag.
	a.tagPickerQuery = "mo"
	tester.Frame()
	if !tester.HasText("mood") {
		t.Fatalf("the search dropped the matching tag: %q", tester.Texts())
	}
	if tester.HasText("reference") {
		t.Fatalf("the search kept a tag that does not match: %q", tester.Texts())
	}
	create := a.tf("inspector.createTag", map[string]any{"name": "mo"})
	if !tester.HasText(create) {
		t.Fatalf("no row creates the typed tag %q: %q", create, tester.Texts())
	}

	// A name that is already a tag needs no create row.
	a.tagPickerQuery = "reference"
	tester.Frame()
	if tester.HasText(a.tf("inspector.createTag", map[string]any{"name": "reference"})) {
		t.Fatal("the picker offers to create a tag that already exists")
	}

	// Clicking a tag applies it and closes the picker.
	a.tagPickerQuery = ""
	tester.Frame()
	row, ok := tester.Find("mood")
	if !ok {
		t.Fatalf("the tag has no row: %q", tester.Texts())
	}
	tester.Press(row.X+row.W/2, row.Y+row.H/2)
	tester.Release(row.X+row.W/2, row.Y+row.H/2)
	tester.Frame()
	if a.dialog != dialogNone || len(a.tagPickerItems) != 0 {
		t.Fatalf("the picker stayed open after a tag was chosen: dialog=%v items=%d", a.dialog, len(a.tagPickerItems))
	}
}

func TestSharedRating(t *testing.T) {
	if got := sharedRating(nil); got != 0 {
		t.Fatalf("rating of nothing = %d, want 0", got)
	}
	if got := sharedRating([]core.Item{{Rating: 3}}); got != 3 {
		t.Fatalf("rating of one item = %d, want 3", got)
	}
	if got := sharedRating([]core.Item{{Rating: 3}, {Rating: 3}}); got != 3 {
		t.Fatalf("rating all items share = %d, want 3", got)
	}
	if got := sharedRating([]core.Item{{Rating: 3}, {Rating: 4}}); got != 0 {
		t.Fatalf("rating of a mixed selection = %d, want 0", got)
	}
}

// TestSelectionInspectorReusesTheRatingStars keeps one rating control: the
// selection shows the same stars as an item, never a row of "N ★" buttons, and
// only offers Clear when every item shares a rating.
func TestSelectionInspectorReusesTheRatingStars(t *testing.T) {
	a := seededApp(t)
	a.selected = map[string]bool{"a": true, "b": true}
	a.selectedOrder = []string{"a", "b"}
	a.items[0].Rating, a.items[1].Rating = 4, 4
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 1400)
	// Section labels are drawn upper case.
	if !tester.HasText(strings.ToUpper(a.t("inspector.rating"))) {
		t.Fatalf("the selection has no rating section: %q", tester.Texts())
	}
	if !tester.HasText(a.t("inspector.clear")) {
		t.Fatal("a rating the selection shares offers no Clear")
	}
	for _, text := range []string{"1 ★", "2 ★", "3 ★", "4 ★", "5 ★"} {
		if tester.HasText(text) {
			t.Fatalf("the selection still draws its own rating button %q", text)
		}
	}

	// Mixed ratings show no shared value, so no Clear either.
	a.items[1].Rating = 2
	tester.Frame()
	if tester.HasText(a.t("inspector.clear")) {
		t.Fatal("a mixed selection offers to clear a rating it does not share")
	}
}

// TestItemInspectorOrder opens with the palette, then the link and the note, and
// keeps the rating below the metadata: what the item looks like, what it is
// for, and what it is.
func TestItemInspectorOrder(t *testing.T) {
	a := seededApp(t)
	a.activeItem.URL = "https://example.com/source"
	a.activeItem.Palette = []string{"#EF4444", "#3B82F6"}
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 2000)
	label := func(key string) ui.Rect {
		t.Helper()
		r, ok := tester.Find(strings.ToUpper(a.t(key)))
		if !ok {
			t.Fatalf("%s has no section label in %q", key, tester.Texts())
		}
		return r
	}
	palette := label("inspector.colorPalette")
	link := label("inspector.sourceUrl")
	note := label("inspector.note")
	metadata := label("inspector.metadata")
	// The rating is a metadata row now, so its label is not upper cased like a
	// section title and sits inside the metadata block.
	rating, ok := tester.Find(a.t("inspector.rating"))
	if !ok {
		t.Fatalf("the metadata block has no rating row: %q", tester.Texts())
	}
	if palette.Y > link.Y {
		t.Errorf("palette (y=%.1f) does not come before the link (y=%.1f)", palette.Y, link.Y)
	}
	if link.Y > note.Y {
		t.Errorf("link (y=%.1f) does not come before the note (y=%.1f)", link.Y, note.Y)
	}
	if note.Y > metadata.Y {
		t.Errorf("note (y=%.1f) is not above the metadata (y=%.1f)", note.Y, metadata.Y)
	}
	if rating.Y < metadata.Y {
		t.Errorf("rating (y=%.1f) is not inside the metadata (y=%.1f)", rating.Y, metadata.Y)
	}
}

func TestRatingPreviewFollowsTheHover(t *testing.T) {
	if got := ratingPreview(0, 0); got != 0 {
		t.Fatalf("preview with nothing to show = %d, want 0", got)
	}
	if got := ratingPreview(3, 0); got != 3 {
		t.Fatalf("preview off the stars = %d, want the stored 3", got)
	}
	if got := ratingPreview(3, 5); got != 5 {
		t.Fatalf("preview under the fifth star = %d, want 5", got)
	}
	if got := ratingPreview(0, 2); got != 2 {
		t.Fatalf("preview under the second star = %d, want 2", got)
	}
}

func TestColorWheelPicksHueAndSaturation(t *testing.T) {
	const size = float32(100)
	cases := []struct {
		name    string
		x, y    float32
		wantHue float32
		wantSat float32
	}{
		{name: "top is red", x: 50, y: 0, wantHue: 0, wantSat: 1},
		{name: "right is a quarter turn", x: 100, y: 50, wantHue: 90, wantSat: 1},
		{name: "bottom is half a turn", x: 50, y: 100, wantHue: 180, wantSat: 1},
		{name: "left is three quarters", x: 0, y: 50, wantHue: 270, wantSat: 1},
		{name: "the middle is gray", x: 50, y: 50, wantHue: 0, wantSat: 0},
		{name: "half way out is half saturated", x: 75, y: 50, wantHue: 90, wantSat: 0.5},
	}
	for _, tc := range cases {
		hue, sat := wheelPick(tc.x, tc.y, size)
		if math.Abs(float64(hue-tc.wantHue)) > 0.01 || math.Abs(float64(sat-tc.wantSat)) > 0.01 {
			t.Errorf("%s: wheelPick(%v, %v) = (%v, %v), want (%v, %v)", tc.name, tc.x, tc.y, hue, sat, tc.wantHue, tc.wantSat)
		}
	}
}

func TestWheelColorRoundTrip(t *testing.T) {
	cases := []struct {
		hue     float32
		r, g, b uint8
	}{
		{hue: 0, r: 255, g: 0, b: 0},
		{hue: 120, r: 0, g: 255, b: 0},
		{hue: 240, r: 0, g: 0, b: 255},
		{hue: 60, r: 255, g: 255, b: 0},
	}
	for _, tc := range cases {
		col := hsvColor(tc.hue, 1)
		if col.R != tc.r || col.G != tc.g || col.B != tc.b {
			t.Fatalf("hsvColor(%v, 1) = %v, want #%02X%02X%02X", tc.hue, col, tc.r, tc.g, tc.b)
		}
		if got := hexOfColor(col); got != fmt.Sprintf("#%02X%02X%02X", tc.r, tc.g, tc.b) {
			t.Fatalf("hex of hue %v = %q", tc.hue, got)
		}
		hue, sat := colorHS(col)
		if math.Abs(float64(hue-tc.hue)) > 0.5 || math.Abs(float64(sat-1)) > 0.01 {
			t.Fatalf("colorHS(%v) = (%v, %v), want (%v, 1)", col, hue, sat, tc.hue)
		}
	}
	// Gray has no hue and no saturation: its knob belongs in the middle.
	if hue, sat := colorHS(ui.RGB(128, 128, 128)); hue != 0 || sat != 0 {
		t.Fatalf("colorHS(gray) = (%v, %v), want (0, 0)", hue, sat)
	}
}

// TestColorFilterOpensSwatchesAndWheel keeps the color criterion off the menu:
// a click opens the picker with its swatches, its wheel and the way back to
// every color, and that last one closes it again.
func TestColorFilterOpensSwatchesAndWheel(t *testing.T) {
	a := seededApp(t)
	a.stats.TopColors = []string{"#EF4444", "#3B82F6"}
	tester := ui.NewTester(a.viewMain, 1280, 800)
	chip, ok := tester.Find(a.t("filters.color"))
	if !ok {
		t.Fatalf("no color criterion: %q", tester.Texts())
	}
	tester.Press(chip.X+chip.W/2, chip.Y+chip.H/2)
	tester.Release(chip.X+chip.W/2, chip.Y+chip.H/2)
	tester.Frame()
	if !a.filterColorOpen {
		t.Fatalf("the color picker did not open: %q", tester.Texts())
	}
	if !tester.HasText(a.t("filters.allColors")) {
		t.Fatalf("the picker shows no way back to every color: %q", tester.Texts())
	}
	all, ok := tester.Find(a.t("filters.allColors"))
	if !ok {
		t.Fatalf("no all-colors button: %q", tester.Texts())
	}
	tester.Press(all.X+all.W/2, all.Y+all.H/2)
	tester.Release(all.X+all.W/2, all.Y+all.H/2)
	tester.Frame()
	if a.filterColorOpen || len(a.filters.Colors) != 0 {
		t.Fatalf("choosing every color left the picker open=%v colors=%v", a.filterColorOpen, a.filters.Colors)
	}
}

// colorPickerWheelRect finds the wheel of the open color picker from its
// footer: the panel is as wide as a row of swatches plus its padding, and the
// wheel sits a gap above the hex row.
func colorPickerWheelRect(t *testing.T, tester *ui.Tester, a *App) ui.Rect {
	t.Helper()
	all, ok := tester.Find(a.t("filters.allColors"))
	if !ok {
		t.Fatalf("the color picker is closed: %q", tester.Texts())
	}
	panelW := colorSwatchSize*colorSwatchesInRow + 5*4 + 28
	panelRight := all.X + all.W + 7 + 12
	return ui.Rect{
		X: panelRight - panelW + 12,
		Y: all.Y + all.H/2 - 11 - 10 - colorWheelSize,
		W: colorWheelSize,
		H: colorWheelSize,
	}
}

// TestColorWheelRendersAndAppliesHues checks the wheel itself: its middle is
// unsaturated, its right side carries the green a quarter turn gives, and a
// press there filters by a green.
func TestColorWheelRendersAndAppliesHues(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	chip, ok := tester.Find(a.t("filters.color"))
	if !ok {
		t.Fatalf("no color criterion: %q", tester.Texts())
	}
	tester.Press(chip.X+chip.W/2, chip.Y+chip.H/2)
	tester.Release(chip.X+chip.W/2, chip.Y+chip.H/2)
	tester.Frame()

	wheel := colorPickerWheelRect(t, tester, a)
	img := tester.Image()
	pixel := func(x, y float32) (uint8, uint8, uint8) {
		i := int(y)*img.Stride + int(x)*4
		return img.Pix[i], img.Pix[i+1], img.Pix[i+2]
	}
	if r, g, b := pixel(wheel.X+wheel.W/2, wheel.Y+wheel.H/2); r < 220 || g < 220 || b < 220 {
		t.Fatalf("the wheel's middle is not unsaturated white: (%d,%d,%d)", r, g, b)
	}
	rightX := wheel.X + wheel.W - 8
	if r, g, b := pixel(rightX, wheel.Y+wheel.H/2); g < 180 || g < r+40 || g < b+40 {
		t.Fatalf("the wheel's right side is not green: (%d,%d,%d)", r, g, b)
	}

	// A press on the right side filters by the hue there.
	tester.Press(rightX, wheel.Y+wheel.H/2)
	tester.Frame()
	tester.Release(rightX, wheel.Y+wheel.H/2)
	tester.Frame()
	if len(a.filters.Colors) != 1 {
		t.Fatalf("the wheel applied no color: %v", a.filters.Colors)
	}
	col, err := theme.ParseColor(a.filters.Colors[0])
	if err != nil {
		t.Fatalf("the wheel applied %q: %v", a.filters.Colors[0], err)
	}
	if col.G <= col.R || col.G <= col.B {
		t.Fatalf("the wheel applied %q, which is not green", a.filters.Colors[0])
	}
}

func TestLargeJustifiedGalleryMarqueeAndClickAfterScroll(t *testing.T) {
	a := seededApp(t)
	a.prefs.LayoutMode = "justified"
	a.prefs.ShowNames = false
	a.items = nil
	dims := [][2]int{{800, 1600}, {1600, 800}, {1000, 1000}, {900, 1400}, {1800, 900}}
	for i := 0; i < 2000; i++ {
		d := dims[i%len(dims)]
		a.items = append(a.items, core.Item{
			ID: fmt.Sprintf("item-%d", i), Name: fmt.Sprintf("Item %d", i),
			Extension: "jpg", MimeType: "image/jpeg", Width: d[0], Height: d[1],
		})
	}
	a.total = len(a.items)
	a.activeItem = nil
	a.selected = map[string]bool{}
	a.rebuildRows()

	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()
	if a.galleryScroll.MaxY <= 0 {
		t.Fatal("gallery has no scroll range")
	}

	// Jump to ~50% of the gallery so unmeasured rows above have varying heights.
	a.galleryScroll.Y = a.galleryScroll.MaxY * 0.5
	tester.Frame()
	if !a.galleryLayoutSampleOK {
		t.Fatal("gallery paint sample was not recorded")
	}

	firstRow, lastRow := a.galleryList.Visible()
	if lastRow <= firstRow+2 {
		t.Fatalf("expected multiple visible rows, got first=%d last=%d", firstRow, lastRow)
	}
	top := float32(mainHeaderHeight + galleryTopPadding)
	targetRow := -1
	var rowLocalY float32
	for r := firstRow; r <= lastRow; r++ {
		if len(a.rows[r]) < 2 {
			continue
		}
		y := a.galleryRowsOriginY(top) + a.galleryRowTopOffset(r)
		if y >= top && y+float32(a.rows[r][0].height) <= a.galleryBoxRect.H {
			targetRow = r
			rowLocalY = y
			break
		}
	}
	if targetRow < 0 {
		t.Fatalf("no fully visible row found in [%d, %d]", firstRow, lastRow)
	}
	row := a.rows[targetRow]
	if len(row) < 2 {
		t.Fatalf("target row %d has fewer than 2 cells (%d)", targetRow, len(row))
	}

	rowH := float32(row[0].height)
	midY := a.galleryBoxRect.Y + rowLocalY + rowH*0.5

	// 1. Clicking a card in targetRow must select that exact card.
	firstCell := row[0]
	clickX := a.galleryBoxRect.X + float32(gallerySidePadding) + float32(firstCell.width)*0.5
	tester.Press(clickX, midY)
	tester.Release(clickX, midY)
	tester.Frame()
	wantID := a.items[firstCell.index].ID
	if !a.selected[wantID] || len(a.selected) != 1 {
		t.Fatalf("click on row %d selected %v, want only %s", targetRow, a.selected, wantID)
	}

	// 2. Marquee-selecting across targetRow from the left margin must select
	//    only items in targetRow, not the row below it.
	a.clearSelection()
	startX := a.galleryBoxRect.X + 4
	endX := a.galleryBoxRect.X + float32(gallerySidePadding) + float32(firstCell.width) + a.galleryGap() + float32(row[1].width)*0.5
	tester.Press(startX, a.galleryBoxRect.Y+rowLocalY+4)
	tester.Move(endX, a.galleryBoxRect.Y+rowLocalY+rowH-4)
	tester.Release(endX, a.galleryBoxRect.Y+rowLocalY+rowH-4)
	tester.Frame()

	for id := range a.selected {
		inTargetRow := false
		for _, cell := range row {
			if a.items[cell.index].ID == id {
				inTargetRow = true
				break
			}
		}
		if !inTargetRow {
			t.Fatalf("marquee on row %d selected item %s from another row (selected=%v)", targetRow, id, a.selected)
		}
	}
	if !a.selected[a.items[row[0].index].ID] || !a.selected[a.items[row[1].index].ID] {
		t.Fatalf("marquee on row %d missed: y=%v h=%v sx=%v ex=%v sel=%v", targetRow, rowLocalY, rowH, startX, endX, a.selected)
	}
}

func TestDoubleClickInlinePreviewEscClosesAndHidesNavButtons(t *testing.T) {
	a := seededApp(t)
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// Double-click Gamma card to open inline preview.
	r, ok := tester.Find("Gamma")
	if !ok {
		t.Fatal("Gamma card not found")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tester.ClickAt(x, y)
	tester.Frame()
	tester.ClickAt(x, y)
	tester.Frame()

	if !a.previewOpen || a.previewMode != "inline" {
		t.Fatalf("double click should open inline preview, got open=%v mode=%q", a.previewOpen, a.previewMode)
	}

	// Inline preview header must not show previous/next navigation buttons.
	if _, hasPrev := tester.Access(a.t("preview.previous")); hasPrev {
		t.Fatal("inline preview header should hide preview.previous button")
	}
	if _, hasNext := tester.Access(a.t("preview.next")); hasNext {
		t.Fatal("inline preview header should hide preview.next button")
	}

	// Pressing Escape must exit inline preview.
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if a.previewOpen {
		t.Fatal("Escape should close inline preview opened by double-click")
	}
}

func TestHistoryBackAndForwardAcrossFolders(t *testing.T) {
	a := seededApp(t)
	a.recordHistory() // Initial NavAll stop

	// Enter Folder A, then switch to Folder B.
	a.navigate(NavFolder, "f1")
	a.navigate(NavFolder, "f2")

	if len(a.history) != 3 || a.historyAt != 2 {
		t.Fatalf("history = (len=%d, at=%d), want (3, 2)", len(a.history), a.historyAt)
	}
	if !a.canGoBack() || a.canGoForward() {
		t.Fatalf("canGoBack=%v canGoForward=%v, want true/false", a.canGoBack(), a.canGoForward())
	}

	// First Back should return to Folder A (f1), not NavAll.
	a.goHistory(-1)
	if a.nav != NavFolder || a.folderID != "f1" {
		t.Fatalf("first back = (%s, %s), want (folder, f1)", a.nav, a.folderID)
	}
	// Simulate async refresh completing after goHistory; must not truncate forward history.
	a.recordHistory()
	if !a.canGoBack() || !a.canGoForward() {
		t.Fatalf("after back: canGoBack=%v canGoForward=%v, want true/true", a.canGoBack(), a.canGoForward())
	}

	// Second Back should return to NavAll.
	a.goHistory(-1)
	if a.nav != NavAll || a.folderID != "" {
		t.Fatalf("second back = (%s, %s), want (all, \"\")", a.nav, a.folderID)
	}
	if a.canGoBack() || !a.canGoForward() {
		t.Fatalf("at root: canGoBack=%v canGoForward=%v, want false/true", a.canGoBack(), a.canGoForward())
	}

	// Forward should walk back to Folder A, then Folder B.
	a.goHistory(1)
	if a.nav != NavFolder || a.folderID != "f1" {
		t.Fatalf("first forward = (%s, %s), want (folder, f1)", a.nav, a.folderID)
	}
	a.goHistory(1)
	if a.nav != NavFolder || a.folderID != "f2" {
		t.Fatalf("second forward = (%s, %s), want (folder, f2)", a.nav, a.folderID)
	}
}

func TestSpaceTogglePreviewDoesNotScrollGallery(t *testing.T) {
	a := seededApp(t)
	var items []core.Item
	for i := 0; i < 60; i++ {
		items = append(items, core.Item{
			ID:     fmt.Sprintf("item-%d", i),
			Name:   fmt.Sprintf("Item %d", i),
			Width:  400,
			Height: 300,
		})
	}
	a.setWorkspaceItems(items, len(items), 0)
	a.activeItemID = "item-0"
	tester := ui.NewTester(a.viewMain, 1280, 800)
	tester.Frame()

	// Press Space to open full preview: gallery scroll must remain 0.
	tester.Key(0, ui.KeySpace)
	tester.Frame()
	if !a.previewOpen || a.previewMode != "full" {
		t.Fatalf("Space should open full preview, got open=%v mode=%q", a.previewOpen, a.previewMode)
	}
	if a.galleryScroll.Y != 0 {
		t.Fatalf("galleryScroll.Y = %v after opening preview with Space, want 0", a.galleryScroll.Y)
	}

	// Press Space again to close full preview: gallery scroll must still remain 0.
	tester.Key(0, ui.KeySpace)
	tester.Frame()
	if a.previewOpen {
		t.Fatal("Space should close full preview")
	}
	if a.galleryScroll.Y != 0 {
		t.Fatalf("galleryScroll.Y = %v after closing preview with Space, want 0", a.galleryScroll.Y)
	}
}

func TestClearHistoryOnLibrarySwitch(t *testing.T) {
	a := seededApp(t)
	a.recordHistory()
	a.navigate(NavFolder, "f1")
	a.navigate(NavFolder, "f2")
	if !a.canGoBack() {
		t.Fatal("expected history to allow back before library switch")
	}

	a.clearHistory()
	if len(a.history) != 0 || a.historyAt != -1 || a.canGoBack() || a.canGoForward() {
		t.Fatalf("history not cleared: len=%d at=%d back=%v forward=%v", len(a.history), a.historyAt, a.canGoBack(), a.canGoForward())
	}
}




