package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"bowerbird/core"
)

func folderIDs(folders []core.Folder) []string {
	ids := make([]string, len(folders))
	for index, folder := range folders {
		ids[index] = folder.ID
	}
	return ids
}

func TestFolderIndexPreservesRelationshipsAndOrder(t *testing.T) {
	folders := []core.Folder{
		{ID: "root", Name: "Root", Children: []core.Folder{
			{ID: "second", Name: "Second", ParentID: "root", SortOrder: 0, ItemCount: 7},
			{ID: "first", Name: "First", ParentID: "root", SortOrder: 1, Children: []core.Folder{
				{ID: "grand", Name: "Grand", ParentID: "first"},
			}},
		}},
		{ID: "orphan", Name: "Orphan", ParentID: "missing"},
	}
	index := newFolderIndex(folders)
	if got := folderIDs(index.all); !reflect.DeepEqual(got, []string{"root", "second", "first", "grand", "orphan"}) {
		t.Fatalf("tree order = %v", got)
	}
	if got := folderIDs(index.roots); !reflect.DeepEqual(got, []string{"root", "orphan"}) {
		t.Fatalf("roots = %v", got)
	}
	if got := folderIDs(index.children["root"]); !reflect.DeepEqual(got, []string{"second", "first"}) {
		t.Fatalf("child order = %v", got)
	}
	if index.byID["second"].ItemCount != 7 {
		t.Fatal("folder item count was lost")
	}
	if _, found := index.byID["unknown"]; found {
		t.Fatal("unknown folder was indexed")
	}
	if got := index.scopeIDs("root", true); !reflect.DeepEqual(got, []string{"root", "second", "first"}) {
		t.Fatalf("one-level scope = %v", got)
	}
	if got := index.scopeIDs("root", false); !reflect.DeepEqual(got, []string{"root"}) {
		t.Fatalf("direct scope = %v", got)
	}
	if got := index.scopeIDs("", true); got != nil {
		t.Fatalf("empty scope = %v", got)
	}
	if got := index.previewGroups("root"); !reflect.DeepEqual(got, [][]string{{"second"}, {"first"}}) {
		t.Fatalf("direct folder preview groups = %v", got)
	}
	if index.canNest("root", "grand") || index.canNest("root", "root") || index.canNest("", "root") {
		t.Fatal("invalid folder move accepted")
	}
	if !index.canNest("grand", "root") || !index.canNest("first", "") {
		t.Fatal("valid folder move rejected")
	}
	wantOptions := []filterFolderOption{
		{"root", "Root"}, {"second", "Root / Second"}, {"first", "Root / First"},
		{"grand", "Root / First / Grand"}, {"orphan", "Orphan"},
	}
	if !reflect.DeepEqual(index.options, wantOptions) {
		t.Fatalf("filter paths = %v", index.options)
	}
}

func TestFolderIndexSupportsFlatSnapshotsAndCycles(t *testing.T) {
	index := newFolderIndex([]core.Folder{
		{ID: "child", ParentID: "root"},
		{ID: "root"},
		{ID: "cycle-a", ParentID: "cycle-b"},
		{ID: "cycle-b", ParentID: "cycle-a"},
	})
	if got := folderIDs(index.children["root"]); !reflect.DeepEqual(got, []string{"child"}) {
		t.Fatalf("flat child lookup = %v", got)
	}
	if got := index.scopeIDs("cycle-a", true); !reflect.DeepEqual(got, []string{"cycle-a", "cycle-b"}) {
		t.Fatalf("cyclic scope = %v", got)
	}
	if index.canNest("child", "cycle-a") {
		t.Fatal("move into a cyclic ancestry was accepted")
	}
}

func TestIncludeSubfoldersUsesOnlyOneLevel(t *testing.T) {
	app, service := newFolderOrderApp(t)
	parent, err := service.CreateFolder("Parent", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateFolder("Child", parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := service.CreateFolder("Grandchild", child.ID)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := service.CreateFolder("Unrelated", "")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 4)
	for position, folderID := range []string{parent.ID, child.ID, grandchild.ID, unrelated.ID} {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("item-%d.txt", position))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("content-%d", position)), 0600); err != nil {
			t.Fatal(err)
		}
		imported, err := service.ImportFiles([]string{path})
		if err != nil || len(imported) != 1 {
			t.Fatalf("import = %v err=%v", imported, err)
		}
		ids[position] = imported[0].ID
		if err := service.AddItemToFolder(ids[position], folderID); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.AddItemToFolder(ids[0], child.ID); err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, app, service)
	app.nav, app.folderID = NavFolder, parent.ID
	checkItems := func(expected ...string) {
		t.Helper()
		items, total, _, _, err := app.fetchWorkspace(workspaceRequest{scope: app.folderScope(), sort: "desc"})
		if err != nil {
			t.Fatal(err)
		}
		wanted := make(map[string]bool, len(expected))
		for _, id := range expected {
			wanted[id] = true
		}
		if total != len(expected) || len(items) != len(expected) {
			t.Fatalf("folder result total=%d items=%d want=%d", total, len(items), len(expected))
		}
		for _, item := range items {
			if !wanted[item.ID] {
				t.Fatalf("folder view included an out-of-scope item %s", item.ID)
			}
			delete(wanted, item.ID)
		}
		if len(wanted) != 0 {
			t.Fatalf("folder view is missing %v", wanted)
		}
	}
	app.prefs.IncludeSubs = false
	checkItems(ids[0])
	app.prefs.IncludeSubs = true
	if got := app.folderScope(); !reflect.DeepEqual(got, []string{parent.ID, child.ID}) {
		t.Fatalf("enabled scope = %v", got)
	}
	checkItems(ids[0], ids[1])
	previews := app.fetchFolderPreviews(app.folderIndex(), app.nav, app.folderID, "desc")
	if len(previews) != 1 || previews[child.ID].Total != 2 {
		t.Fatalf("direct child preview = %v", previews)
	}
	for _, item := range previews[child.ID].Items {
		if item.ID == ids[2] || item.ID == ids[3] {
			t.Fatal("child preview included a deeper or unrelated item")
		}
	}
	app.folderID = child.ID
	checkItems(ids[0], ids[1], ids[2])
	if app.folderCanNest(parent.ID, grandchild.ID) {
		t.Fatal("one-level item scope weakened the folder cycle check")
	}
	app.nav = NavAll
	if scope := app.folderScope(); scope != nil {
		t.Fatalf("All items inherited a folder scope: %v", scope)
	}
}

func TestFolderIndexReusesSnapshotWithoutAllocating(t *testing.T) {
	app := New(nil)
	app.folders = sidebar511Folders()
	snapshot := app.folderIndex()
	allocations := testing.AllocsPerRun(50, func() {
		total := len(app.folderRoots()) + len(app.filterFolderOptions())
		for _, folder := range snapshot.all {
			index := app.folderIndex()
			total += len(index.children[folder.ID])
			if _, found := index.byID[folder.ID]; !found {
				t.Fatal("indexed folder is missing")
			}
			if !index.canNest(folder.ID, "") {
				t.Fatal("valid root move rejected")
			}
		}
		if total != 1022 {
			t.Fatalf("snapshot lookup total = %d", total)
		}
	})
	if allocations != 0 {
		t.Fatalf("unchanged folder lookups allocate %g objects", allocations)
	}
	if app.folderIndex() != snapshot {
		t.Fatal("unchanged folder snapshot rebuilt its index")
	}
}

func TestFolderIndexRefreshesFromSQLiteChanges(t *testing.T) {
	app, service := newFolderOrderApp(t)
	parent, err := service.CreateFolder("Parent", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateFolder("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateFolder("Child", parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, app, service)
	before := app.folderIndex()
	if len(before.children[parent.ID]) != 1 {
		t.Fatal("SQLite parent reference was not indexed")
	}
	if err := service.RenameFolder(child.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, app, service)
	renamed := app.folderIndex()
	if renamed == before || renamed.byID[child.ID].Name != "Renamed" || before.byID[child.ID].Name != "Child" {
		t.Fatal("rename did not replace the immutable snapshot")
	}
	if err := service.MoveFolder(child.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, app, service)
	moved := app.folderIndex()
	if len(moved.children[parent.ID]) != 0 || len(moved.children[other.ID]) != 1 {
		t.Fatal("SQLite reparenting did not update the cache")
	}
	if err := service.SetFolderOrder("", []string{other.ID, parent.ID}); err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, app, service)
	if got := folderIDs(app.folderRoots()); !reflect.DeepEqual(got, []string{other.ID, parent.ID}) {
		t.Fatalf("SQLite root order = %v", got)
	}
	if err := service.DeleteFolder(child.ID); err != nil {
		t.Fatal(err)
	}
	loadFolderTree(t, app, service)
	if _, found := app.folderIndex().byID[child.ID]; found {
		t.Fatal("deleted folder remains cached")
	}
	app.folders = []core.Folder{{ID: parent.ID, Name: "Different library"}}
	if app.folderIndex().byID[parent.ID].Name != "Different library" || len(app.folderRoots()) != 1 {
		t.Fatal("new library reused the previous snapshot")
	}
	app.folders = nil
	if len(app.folderIndex().all) != 0 || len(app.folderRoots()) != 0 || len(app.filterFolderOptions()) != 0 {
		t.Fatal("closed library retains folder data")
	}
}

func sidebar511Folders() []core.Folder {
	folders := make([]core.Folder, 35)
	for rootIndex := range folders {
		folders[rootIndex] = core.Folder{ID: fmt.Sprintf("root-%d", rootIndex)}
		count := 13
		if rootIndex < 21 {
			count++
		}
		for childIndex := 0; childIndex < count; childIndex++ {
			folders[rootIndex].Children = append(folders[rootIndex].Children, core.Folder{
				ID: fmt.Sprintf("child-%d-%d", rootIndex, childIndex), ParentID: folders[rootIndex].ID,
			})
		}
	}
	return folders
}

var folderBenchmarkTotal int

func BenchmarkSidebarFolderLookup511(b *testing.B) {
	app := New(nil)
	app.folders = sidebar511Folders()
	snapshot := app.folderIndex()
	for _, scenario := range []struct {
		name    string
		visible []core.Folder
	}{
		{"collapsed", snapshot.roots},
		{"expanded", snapshot.all},
	} {
		b.Run(scenario.name+"/baseline", func(b *testing.B) {
			b.ReportAllocs()
			total := 0
			for range b.N {
				for _, folder := range scenario.visible {
					all := flattenFolders(app.folders)
					children := make([]core.Folder, 0, len(all))
					for _, candidate := range all {
						if candidate.ParentID == folder.ID {
							children = append(children, candidate)
						}
					}
					total += len(children)
				}
			}
			folderBenchmarkTotal = total
		})
		b.Run(scenario.name+"/cached", func(b *testing.B) {
			b.ReportAllocs()
			total := 0
			for range b.N {
				total += len(app.folderRoots())
				for _, folder := range scenario.visible {
					total += len(app.folderIndex().children[folder.ID])
				}
			}
			folderBenchmarkTotal = total
		})
	}
}
