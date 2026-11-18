package core_test

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"bowerbird/core"
)

// newRaceLibrary returns a manager with an empty library and its own user data
// directory, so the test never touches the real user profile.
func newRaceLibrary(t *testing.T, name string) *core.LibraryManager {
	t.Helper()

	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	mgr := core.NewLibraryManager()
	if _, err := mgr.CreateLibrary(t.TempDir(), name); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	t.Cleanup(func() { _ = mgr.CloseLibrary() })
	return mgr
}

// An OS file drop reaches the app twice - once through the webview drop handler
// (ImportFromBase64) and once through the native drop handler (ImportFiles) - and
// they race for the same content-addressed item. The loser used to delete the
// shared item directory, leaving a row whose payload was gone: a card that could
// not be previewed and a file that could never be re-imported.
func TestConcurrentImportOfSameContentKeepsPayload(t *testing.T) {
	mgr := newRaceLibrary(t, "Race")

	// Large enough that both imports pass the deduplication check before either
	// finishes writing.
	payload := make([]byte, 16<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}
	src := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := mgr.ImportFiles([]string{src}); err != nil {
				t.Errorf("ImportFiles: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	items, err := mgr.GetItems("", "desc")
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (content addressed, so the second import must reuse it)", len(items))
	}
	if _, err := os.Stat(items[0].FilePath); err != nil {
		t.Fatalf("item %s has a row but no payload: %v", items[0].ID, err)
	}
}

// Re-importing the same bytes repairs an item whose payload is missing, which is
// the state a row is left in when its directory was lost.
func TestReimportRestoresMissingPayload(t *testing.T) {
	mgr := newRaceLibrary(t, "Repair")

	src := filepath.Join(t.TempDir(), "asset.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if _, err := mgr.ImportFiles([]string{src}); err != nil {
		t.Fatalf("ImportFiles: %v", err)
	}
	items, err := mgr.GetItems("", "desc")
	if err != nil || len(items) != 1 {
		t.Fatalf("GetItems = %d items, err=%v", len(items), err)
	}

	// Simulate the damaged state: row present, payload gone.
	if err := os.RemoveAll(items[0].ItemPath); err != nil {
		t.Fatalf("remove item dir: %v", err)
	}
	if _, err := os.Stat(items[0].FilePath); err == nil {
		t.Fatal("payload still present after removal, the fixture is not testing anything")
	}

	if _, err := mgr.ImportFiles([]string{src}); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	items, err = mgr.GetItems("", "desc")
	if err != nil {
		t.Fatalf("GetItems after re-import: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items after re-import, want 1", len(items))
	}
	if _, err := os.Stat(items[0].FilePath); err != nil {
		t.Fatalf("re-import did not restore the payload: %v", err)
	}
}

func TestConcurrentFolderImportAndMillisecondSearch(t *testing.T) {
	mgr := newRaceLibrary(t, "BulkPerf")
	rootDir := filepath.Join(t.TempDir(), "BulkFolder")

	const subFolders = 5
	const filesPerFolder = 40
	const totalFiles = subFolders * filesPerFolder

	for s := range subFolders {
		dir := filepath.Join(rootDir, fmt.Sprintf("category_%d", s))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		for f := range filesPerFolder {
			name := fmt.Sprintf("asset_%d_%d.txt", s, f)
			content := fmt.Sprintf("unique-payload-%d-%d", s, f)
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
		}
	}

	var progressCalls atomic.Int32
	var lastTotal atomic.Int32
	startImport := time.Now()
	folder, err := mgr.ImportFolderRecursivelyWithProgress(rootDir, "", func(current, total int, filename string) {
		progressCalls.Add(1)
		lastTotal.Store(int32(total))
	})
	importElapsed := time.Since(startImport)
	if err != nil {
		t.Fatalf("ImportFolderRecursivelyWithProgress: %v", err)
	}
	if folder == nil {
		t.Fatal("expected non-nil root folder")
	}
	if progressCalls.Load() == 0 || int(lastTotal.Load()) != totalFiles {
		t.Fatalf("expected progress callbacks with total=%d, got calls=%d total=%d", totalFiles, progressCalls.Load(), lastTotal.Load())
	}

	// Verify all items are indexed and searchable in sub-millisecond latency
	startQuery := time.Now()
	allItems, err := mgr.GetItems("", "desc")
	queryElapsed := time.Since(startQuery)
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(allItems) != totalFiles {
		t.Fatalf("expected %d imported items, got %d", totalFiles, len(allItems))
	}

	startSearch := time.Now()
	matched, err := mgr.GetItems("asset_3_17", "desc")
	searchElapsed := time.Since(startSearch)
	if err != nil {
		t.Fatalf("GetItems search: %v", err)
	}
	if len(matched) != 1 || matched[0].Name != "asset_3_17" {
		t.Fatalf("expected 1 match for asset_3_17, got %d", len(matched))
	}
	if searchElapsed > 10*time.Millisecond {
		t.Errorf("expected millisecond-level search latency, got %v", searchElapsed)
	}
	t.Logf("Imported %d files across %d folders in %v; GetItems=%v; Search=%v", totalFiles, subFolders, importElapsed, queryElapsed, searchElapsed)
}

// Importing a file that is already the active library's own payload is a
// no-op: no hashing, no copy, and no duplicate row.
func TestImportSkipsActiveLibraryPayload(t *testing.T) {
	mgr := newRaceLibrary(t, "SelfDrop")

	src := filepath.Join(t.TempDir(), "asset.txt")
	if err := os.WriteFile(src, []byte("unique-self-drop-payload"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	imported, err := mgr.ImportFiles([]string{src})
	if err != nil || len(imported) != 1 {
		t.Fatalf("ImportFiles = %d items, err=%v", len(imported), err)
	}

	again, err := mgr.ImportFiles([]string{imported[0].FilePath})
	if err != nil {
		t.Fatalf("re-import own payload: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("own payload imported again: %d items", len(again))
	}
	items, err := mgr.GetItems("", "desc")
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items after self-drop, want 1", len(items))
	}
}

// A native drop of an item path inside the app routes as an internal item
// (or skips it), never as an external import that shows another import pass.
func TestNativeDropSkipsCurrentLibraryItem(t *testing.T) {
	mgr := newRaceLibrary(t, "NativeSelfDrop")
	svc := core.NewService(mgr)

	src := filepath.Join(t.TempDir(), "asset.txt")
	if err := os.WriteFile(src, []byte("native-self-drop-payload"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	imported, err := mgr.ImportFiles([]string{src})
	if err != nil || len(imported) != 1 {
		t.Fatalf("ImportFiles = %d items, err=%v", len(imported), err)
	}

	for _, dropped := range []string{imported[0].FilePath, imported[0].FileURL} {
		result, err := svc.HandleNativeDropWithResult([]string{dropped}, "", "")
		if err != nil {
			t.Fatalf("HandleNativeDrop(%q): %v", dropped, err)
		}
		if result.Importing || result.External != 0 {
			t.Fatalf("self-drop %q started an import: %+v", dropped, result)
		}
	}
	items, err := mgr.GetItems("", "desc")
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items after native self-drop, want 1", len(items))
	}
}

// Items from another library are external files for the active library. They
// must be routed through the import pipeline and deduplicated against the
// active library only.
func TestNativeDropImportsItemFromAnotherLibrary(t *testing.T) {
	mgr := newRaceLibrary(t, "CrossLibraryDrop")
	svc := core.NewService(mgr)

	otherItemDir := filepath.Join(t.TempDir(), "Other.library", "items", strings.Repeat("a", 64))
	if err := os.MkdirAll(otherItemDir, 0o755); err != nil {
		t.Fatalf("mkdir other library item: %v", err)
	}
	src := filepath.Join(otherItemDir, "asset.txt")
	if err := os.WriteFile(src, []byte("item from another library"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	result, err := svc.HandleNativeDropWithResult([]string{src}, "", "")
	if err != nil {
		t.Fatalf("HandleNativeDrop: %v", err)
	}
	if result.Internal != 0 || result.External != 1 || !result.Importing {
		t.Fatalf("cross-library drop was not treated as external: %+v", result)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		items, err := mgr.GetItems("", "desc")
		if err != nil {
			t.Fatalf("GetItems: %v", err)
		}
		if len(items) == 1 {
			if items[0].Name != "asset" {
				t.Fatalf("imported item name = %q, want asset", items[0].Name)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("cross-library import produced %d items, want 1", len(items))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Loose files inside the active library are also its own data and are skipped
// before the import pipeline starts.
func TestNativeDropSkipsLooseFileInActiveLibrary(t *testing.T) {
	mgr := newRaceLibrary(t, "LooseSelfDrop")
	svc := core.NewService(mgr)
	lib := mgr.GetActiveLibrary()
	if lib == nil {
		t.Fatal("GetActiveLibrary: no active library")
	}

	loose := filepath.Join(lib.Path, "notes.txt")
	if err := os.WriteFile(loose, []byte("library bookkeeping"), 0o644); err != nil {
		t.Fatalf("write loose file: %v", err)
	}
	result, err := svc.HandleNativeDropWithResult([]string{loose}, "", "")
	if err != nil {
		t.Fatalf("HandleNativeDrop: %v", err)
	}
	if result.External != 0 || result.Importing || result.Skipped != 1 {
		t.Fatalf("loose library file was not skipped: %+v", result)
	}
}

type dropImportHost struct {
	core.Host
	finished chan error
}

func (h *dropImportHost) Emit(name string, payload any) {
	switch name {
	case "import-completed":
		h.finished <- nil
	case "import-error":
		h.finished <- fmt.Errorf("%v", payload)
	}
}

func TestNativeDropImportsIntoFolder(t *testing.T) {
	mgr := newRaceLibrary(t, "FolderDrop")
	svc := core.NewService(mgr)
	target, err := mgr.CreateFolder("Target", "")
	if err != nil {
		t.Fatal(err)
	}
	host := &dropImportHost{finished: make(chan error, 1)}
	core.AttachHost(svc, host)

	dir := t.TempDir()
	file := filepath.Join(dir, "asset.txt")
	subdir := filepath.Join(dir, "Nested")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		file:                                "file dropped into the open folder",
		filepath.Join(subdir, "nested.txt"): "file inside a dropped directory",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.HandleNativeDropWithResult([]string{file, subdir}, "subfolder", target.ID)
	if err != nil || !result.Importing {
		t.Fatalf("drop = %+v, err=%v", result, err)
	}
	select {
	case err := <-host.finished:
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("folder import did not finish")
	}

	items, err := mgr.GetItems("", "desc")
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %v, err=%v", items, err)
	}
	folders, err := mgr.GetFolders()
	if err != nil {
		t.Fatal(err)
	}
	var nestedID string
	for _, folder := range folders {
		if folder.ID == target.ID {
			for _, child := range folder.Children {
				if child.Name == "Nested" {
					nestedID = child.ID
				}
			}
		}
	}
	if nestedID == "" {
		t.Fatal("dropped directory was not created under the target folder")
	}
	for _, item := range items {
		want := target.ID
		if item.Name == "nested" {
			want = nestedID
		}
		if len(item.Folders) != 1 || item.Folders[0] != want {
			t.Fatalf("item %q folders = %v, want [%s]", item.Name, item.Folders, want)
		}
	}
}

func TestImportModeCopyKeepsSourceAndMoveDeletesSource(t *testing.T) {
	mgr := newRaceLibrary(t, "ImportMode")
	svc := core.NewService(mgr)
	if got := svc.GetImportMode(); got != core.ImportModeCopy {
		t.Fatalf("default import mode = %q, want %q", got, core.ImportModeCopy)
	}

	srcDir := t.TempDir()
	copySrc := filepath.Join(srcDir, "keep-me.txt")
	if err := os.WriteFile(copySrc, []byte("copy mode content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Copy mode (default): source remains after import.
	copied, err := mgr.ImportFiles([]string{copySrc})
	if err != nil || len(copied) != 1 {
		t.Fatalf("ImportFiles(copy) = %d, err=%v", len(copied), err)
	}
	if _, err := os.Stat(copySrc); err != nil {
		t.Fatalf("copy mode removed source file: %v", err)
	}

	// Switch to move mode.
	if err := svc.SetImportMode(core.ImportModeMove); err != nil {
		t.Fatalf("SetImportMode(move): %v", err)
	}

	// Single file import in move mode deletes source.
	moveSingle := filepath.Join(srcDir, "move-single.txt")
	if err := os.WriteFile(moveSingle, []byte("move single content"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved1, err := mgr.ImportFiles([]string{moveSingle})
	if err != nil || len(moved1) != 1 {
		t.Fatalf("ImportFiles(move single) = %d, err=%v", len(moved1), err)
	}
	if _, err := os.Stat(moveSingle); !os.IsNotExist(err) {
		t.Fatalf("move mode kept single source file %s (err=%v)", moveSingle, err)
	}
	if _, err := os.Stat(moved1[0].FilePath); err != nil {
		t.Fatalf("moved payload missing in library: %v", err)
	}

	// Batch import in move mode deletes all imported sources.
	moveBatchA := filepath.Join(srcDir, "move-a.txt")
	moveBatchB := filepath.Join(srcDir, "move-b.txt")
	if err := os.WriteFile(moveBatchA, []byte("move batch A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(moveBatchB, []byte("move batch B"), 0o644); err != nil {
		t.Fatal(err)
	}
	movedBatch, err := mgr.ImportFiles([]string{moveBatchA, moveBatchB})
	if err != nil || len(movedBatch) != 2 {
		t.Fatalf("ImportFiles(move batch) = %d, err=%v", len(movedBatch), err)
	}
	for _, p := range []string{moveBatchA, moveBatchB} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("move mode kept batch source file %s (err=%v)", p, err)
		}
	}

	// Duplicate content in move mode also removes the source file.
	moveDup := filepath.Join(srcDir, "move-dup.txt")
	if err := os.WriteFile(moveDup, []byte("move single content"), 0o644); err != nil {
		t.Fatal(err)
	}
	dupItems, err := mgr.ImportFiles([]string{moveDup})
	if err != nil || len(dupItems) != 1 {
		t.Fatalf("ImportFiles(move dup) = %d, err=%v", len(dupItems), err)
	}
	if _, err := os.Stat(moveDup); !os.IsNotExist(err) {
		t.Fatalf("move mode kept duplicate source file %s (err=%v)", moveDup, err)
	}

	// Folder import in move mode removes imported files in the folder.
	folderSrc := filepath.Join(srcDir, "folder-to-move")
	if err := os.Mkdir(folderSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	moveInFolder := filepath.Join(folderSrc, "inside.txt")
	if err := os.WriteFile(moveInFolder, []byte("folder move content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ImportFolderRecursively(folderSrc, ""); err != nil {
		t.Fatalf("ImportFolderRecursively(move): %v", err)
	}
	if _, err := os.Stat(moveInFolder); !os.IsNotExist(err) {
		t.Fatalf("move mode kept folder file %s (err=%v)", moveInFolder, err)
	}

	// Re-importing an active library payload in move mode must never delete it.
	if _, err := mgr.ImportFiles([]string{moved1[0].FilePath}); err != nil {
		t.Fatalf("ImportFiles(active payload): %v", err)
	}
	if _, err := os.Stat(moved1[0].FilePath); err != nil {
		t.Fatalf("active library payload was deleted: %v", err)
	}
}

func isUppercaseAlnum(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func TestRandomItemDirAndModifiedFileReimport(t *testing.T) {
	mgr := newRaceLibrary(t, "ModifiedReimport")

	srcDir := t.TempDir()
	v1 := []byte("original psd v1 content")
	v2 := []byte("edited psd v2 content in photoshop")
	psd1 := filepath.Join(srcDir, "psd1.txt")
	psd2 := filepath.Join(srcDir, "psd2.txt")
	if err := os.WriteFile(psd1, v1, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(psd2, v1, 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Import psd1: directory name must be random uppercase alphanumeric, not 64-char hex.
	imp1, err := mgr.ImportFiles([]string{psd1})
	if err != nil || len(imp1) != 1 {
		t.Fatalf("ImportFiles(psd1) = %d, err=%v", len(imp1), err)
	}
	item1 := imp1[0]
	if !isUppercaseAlnum(item1.ID) || item1.ID == item1.Hex || len(item1.ID) == 64 {
		t.Fatalf("expected random uppercase alphanumeric ID distinct from hex, got id=%q hex=%q", item1.ID, item1.Hex)
	}
	if filepath.Base(item1.ItemPath) != item1.ID {
		t.Fatalf("itemDir %q does not match id %q", item1.ItemPath, item1.ID)
	}

	// 2. Edit psd1 in-place inside the library (simulating external editor save).
	if err := os.WriteFile(item1.FilePath, v2, 0o644); err != nil {
		t.Fatalf("edit item1 in-place: %v", err)
	}

	// 3. Import psd2 (which still has the original v1 hash).
	// It must NOT be ignored; item1's hex must hot-update to v2, and psd2 must import cleanly.
	imp2, err := mgr.ImportFiles([]string{psd2})
	if err != nil || len(imp2) != 1 {
		t.Fatalf("ImportFiles(psd2) = %d, err=%v", len(imp2), err)
	}
	item2 := imp2[0]
	if item2.ID == item1.ID {
		t.Fatalf("psd2 reused item1's ID %q", item1.ID)
	}
	if !isUppercaseAlnum(item2.ID) || item2.ID == item2.Hex {
		t.Fatalf("expected random uppercase alphanumeric ID for psd2, got id=%q hex=%q", item2.ID, item2.Hex)
	}

	items, err := mgr.GetItems("", "desc")
	if err != nil || len(items) != 2 {
		t.Fatalf("GetItems = %d, err=%v; want 2", len(items), err)
	}

	got1, err := mgr.GetItem(item1.ID)
	if err != nil {
		t.Fatalf("GetItem(item1): %v", err)
	}
	if got1.Hex != core.ComputeBytesSHA256(v2) {
		t.Fatalf("item1 hex not hot-updated: got %s, want %s", got1.Hex, core.ComputeBytesSHA256(v2))
	}
	if item2.Hex != core.ComputeBytesSHA256(v1) {
		t.Fatalf("item2 hex = %s, want %s", item2.Hex, core.ComputeBytesSHA256(v1))
	}

	// 4. Importing psd2 again while unmodified must deduplicate.
	imp2Again, err := mgr.ImportFiles([]string{psd2})
	if err != nil || len(imp2Again) != 1 || imp2Again[0].ID != item2.ID {
		t.Fatalf("re-import unmodified psd2: got %+v, err=%v", imp2Again, err)
	}
	items, _ = mgr.GetItems("", "desc")
	if len(items) != 2 {
		t.Fatalf("expected 2 items after duplicate import, got %d", len(items))
	}

	// 5. Deleting item1 must not delete item2.
	if err := mgr.DeleteItem(item1.ID); err != nil {
		t.Fatalf("DeleteItem(item1): %v", err)
	}
	if _, err := mgr.GetItem(item2.ID); err != nil {
		t.Fatalf("item2 disappeared after deleting item1: %v", err)
	}
}

func TestLegacyLibraryHexIndexInitAndCompat(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	mgr := core.NewLibraryManager()
	t.Cleanup(func() { _ = mgr.CloseLibrary() })

	libRoot := t.TempDir()
	info, err := mgr.CreateLibrary(libRoot, "LegacyLib")
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	libPath := info.Path

	// Build a legacy 64-char hex item directory and insert a row with empty hex,
	// clearing hex_index_initialized to simulate opening an existing pre-migration library.
	legacyPayload := []byte("legacy file payload v1")
	legacyHex := core.ComputeBytesSHA256(legacyPayload)
	legacyDir := filepath.Join(libPath, "items", legacyHex)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacyDir, "legacy.txt")
	if err := os.WriteFile(legacyFile, legacyPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mgr.CloseLibrary(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", filepath.Join(libPath, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO items (id, hex, name, extension, filename, size, mime_type, created_at, imported_at) VALUES (?, '', 'legacy', 'txt', 'legacy.txt', ?, 'text/plain', ?, ?)`,
		legacyHex, len(legacyPayload), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM library_meta WHERE key = 'hex_index_initialized'`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Re-open the legacy library: OpenLibrary must initialize the hex index once.
	if _, err := mgr.OpenLibrary(libPath); err != nil {
		t.Fatalf("OpenLibrary: %v", err)
	}

	gotLegacy, err := mgr.GetItem(legacyHex)
	if err != nil {
		t.Fatalf("GetItem(legacyHex): %v", err)
	}
	if gotLegacy.Hex != legacyHex {
		t.Fatalf("legacy item hex = %q, want %q", gotLegacy.Hex, legacyHex)
	}

	// Importing a copy of the legacy file must deduplicate against the legacy item.
	dupSrc := filepath.Join(t.TempDir(), "copy.txt")
	if err := os.WriteFile(dupSrc, legacyPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	impDup, err := mgr.ImportFiles([]string{dupSrc})
	if err != nil || len(impDup) != 1 || impDup[0].ID != legacyHex {
		t.Fatalf("expected deduplication against legacy item %s, got %+v, err=%v", legacyHex, impDup, err)
	}

	// Modify legacy file in-place, then import dupSrc again:
	// legacy item's hex hot-updates, and dupSrc is imported into a new random directory!
	editedLegacy := []byte("legacy file payload v2 edited")
	if err := os.WriteFile(legacyFile, editedLegacy, 0o644); err != nil {
		t.Fatal(err)
	}
	impNew, err := mgr.ImportFiles([]string{dupSrc})
	if err != nil || len(impNew) != 1 {
		t.Fatalf("ImportFiles(dupSrc after legacy edit) = %d, err=%v", len(impNew), err)
	}
	if impNew[0].ID == legacyHex || !isUppercaseAlnum(impNew[0].ID) {
		t.Fatalf("expected new random uppercase alphanumeric ID, got %q", impNew[0].ID)
	}

	// Deleting the legacy item (whose id == legacyHex) must NOT delete the newly imported item (whose hex == legacyHex).
	if err := mgr.DeleteItem(legacyHex); err != nil {
		t.Fatalf("DeleteItem(legacyHex): %v", err)
	}
	if _, err := mgr.GetItem(impNew[0].ID); err != nil {
		t.Fatalf("new item disappeared after deleting legacy item: %v", err)
	}
}
