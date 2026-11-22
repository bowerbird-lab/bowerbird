package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func diskWorkspaceSeed(tb testing.TB, count int) *LibraryManager {
	tb.Helper()
	path := tb.TempDir()
	db, err := openLibraryDB(filepath.Join(path, "index.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	manager := &LibraryManager{db: db, activeLib: &LibraryInfo{ID: "disk", Path: path}}
	if err := manager.initSchema(db); err != nil {
		tb.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT INTO items
		(id,hex,name,extension,filename,size,mime_type,width,height,has_thumbnail,tags,created_at,imported_at,
		note,prompt,url,rating,in_trash,trashed_at,trashed_folders,dominant_color,palette,meta_json)
		VALUES (?,?,?,'jpg','original.jpg',1024,'image/jpeg',1600,900,1,?,1,?,?,?,?,?,?,?,?,'#FF0000','#FF0000','{"orientation":"landscape"}')`)
	if err != nil {
		tb.Fatal(err)
	}
	defer insert.Close()
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("item-%06d", index)
		tag := ""
		if index%2 == 0 {
			tag = "reference"
		}
		trash := 0
		if index%7 == 0 {
			trash = 1
		}
		_, err := insert.Exec(id, "hex-"+id, id, tag, index, "note "+id, strings.Repeat("prompt ", 64), "https://example.com/"+id, index%6, trash, index, `["f1"]`)
		if err != nil {
			tb.Fatal(err)
		}
		if index%3 == 0 {
			if _, err := tx.Exec("INSERT INTO item_folders(item_id,folder_id,added_at) VALUES (?,'f1',1)", id); err != nil {
				tb.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	return manager
}

func TestDiskWorkspaceKeepsOnlySummariesAndRequestedCards(t *testing.T) {
	manager := diskWorkspaceSeed(t, 1000)
	index, err := manager.WorkspaceIndex(QuerySpec{}, "desc", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if index.Total != 857 || len(index.Items) != index.Total || index.TotalSize != int64(index.Total)*1024 {
		t.Fatalf("index totals = %+v", index)
	}
	for _, item := range index.Items {
		if item.FilePath != "" || item.Prompt != "" || item.Note != "" || item.Meta != nil || len(item.Folders) > 0 {
			t.Fatal("layout index contains full metadata")
		}
	}
	ids := itemIDs(index.Items[:96])
	cards, err := manager.workspaceItems(ids, true)
	if err != nil || len(cards) != 96 {
		t.Fatalf("cards=%d err=%v", len(cards), err)
	}
	for _, item := range cards {
		if item.FilePath == "" || item.Prompt != "" || item.Note != "" || item.Meta != nil {
			t.Fatal("gallery cards retained details or lost their media paths")
		}
	}
	full, err := manager.WorkspaceItems(ids[:1])
	if err != nil || len(full) != 1 || full[0].Prompt == "" || full[0].Note == "" {
		t.Fatalf("full metadata is not available on demand: err=%v", err)
	}
	if manager.cacheLoaded || len(manager.itemsByID) != 0 || len(manager.searchIndex) != 0 {
		t.Fatal("disk workspace warmed the full-library cache")
	}
	item, err := manager.GetItem(ids[0])
	if err != nil || item == nil || !item.HasThumbnail {
		t.Fatalf("on-demand item lost its thumbnail flag: item=%v err=%v", item, err)
	}
}

func TestDiskWorkspaceMatchesCachedQuerySemantics(t *testing.T) {
	manager := diskWorkspaceSeed(t, 40)
	queries := []string{
		"", "reference", "note item-000003", "prompt", "type:image", "tag:REFERENCE",
		"folder:f1", "folder:none", "color:#FF0000", "orientation:landscape",
		"size:small", "rating:3", "has:prompt", "no:note", "is:untagged", "is:uncategorized",
	}
	type queryCase struct {
		query   string
		sort    string
		scope   []string
		unrated bool
		page    QueryPage
	}
	var cases []queryCase
	for _, query := range queries {
		for _, order := range []string{"asc", "desc"} {
			for _, scope := range [][]string{nil, {"f1"}} {
				for _, unrated := range []bool{false, true} {
					spec, err := ParseQuery(query)
					if err != nil {
						t.Fatal(err)
					}
					page, err := manager.QueryPage(spec, order, 1, 5, scope, unrated)
					if err != nil {
						t.Fatal(err)
					}
					cases = append(cases, queryCase{query, order, scope, unrated, page})
				}
			}
		}
	}
	diskStats, err := manager.WorkspaceStats("desc")
	if err != nil {
		t.Fatal(err)
	}
	groups := [][]string{{"f1"}, {}, {"missing", "f1"}}
	diskPreviews, err := manager.FolderPreviews(groups, "desc")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ensureCacheLoaded(); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		spec, _ := ParseQuery(test.query)
		cached, err := manager.QueryPage(spec, test.sort, 1, 5, test.scope, test.unrated)
		if err != nil {
			t.Fatal(err)
		}
		if cached.Total != test.page.Total || cached.TotalSize != test.page.TotalSize || !equalStrings(itemIDs(cached.Items), itemIDs(test.page.Items)) {
			t.Fatalf("disk/cache mismatch for query=%q order=%s scope=%v unrated=%t: disk=%v cached=%v",
				test.query, test.sort, test.scope, test.unrated, itemIDs(test.page.Items), itemIDs(cached.Items))
		}
	}
	cachedStats, _ := manager.WorkspaceStats("desc")
	if !reflect.DeepEqual(diskStats, cachedStats) {
		t.Fatalf("stats mismatch: disk=%+v cached=%+v", diskStats, cachedStats)
	}
	cachedPreviews, _ := manager.FolderPreviews(groups, "desc")
	for index := range groups {
		if diskPreviews[index].Total != cachedPreviews[index].Total ||
			diskPreviews[index].TotalSize != cachedPreviews[index].TotalSize ||
			!equalStrings(itemIDs(diskPreviews[index].Items), itemIDs(cachedPreviews[index].Items)) {
			t.Fatalf("folder preview mismatch for %v", groups[index])
		}
	}
}

func TestDiskLegacyReadsDoNotRetainLibraryCache(t *testing.T) {
	manager := diskWorkspaceSeed(t, 20)
	items, err := manager.GetItems("", "desc")
	if err != nil || len(items) != 17 {
		t.Fatalf("active items=%d err=%v", len(items), err)
	}
	trash, err := manager.GetTrashItems("", "desc")
	if err != nil || len(trash) != 3 {
		t.Fatalf("trash=%d err=%v", len(trash), err)
	}
	if len(trash[0].Folders) != 1 || trash[0].Folders[0] != "f1" {
		t.Fatal("trash lost its saved folder associations")
	}
	if manager.cacheLoaded {
		t.Fatal("legacy read permanently loaded the library")
	}
}

func TestDiskWorkspaceCancelsSupersededQueries(t *testing.T) {
	manager := diskWorkspaceSeed(t, 20)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.WorkspaceIndexContext(ctx, QuerySpec{}, "desc", nil, false, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query returned %v", err)
	}
	if manager.cacheLoaded {
		t.Fatal("cancelled query loaded a permanent cache")
	}
}

func TestDiskFolderScopeUsesRelationIndexAndDeduplicates(t *testing.T) {
	manager := diskWorkspaceSeed(t, 7)
	for _, id := range []string{"item-000003", "item-000005"} {
		if _, err := manager.db.Exec("INSERT INTO item_folders(item_id,folder_id,added_at) VALUES (?,'f2',1)", id); err != nil {
			t.Fatal(err)
		}
	}
	scope := []string{"f1", "f2"}
	page, err := manager.WorkspaceIndex(QuerySpec{}, "desc", scope, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || page.TotalSize != 3072 ||
		!equalStrings(itemIDs(page.Items), []string{"item-000006", "item-000005", "item-000003"}) {
		t.Fatalf("folder scope duplicated items or included trash: ids=%v total=%d size=%d", itemIDs(page.Items), page.Total, page.TotalSize)
	}
	where, arguments := diskItemScope(false, scope)
	rows, err := manager.db.Query("EXPLAIN QUERY PLAN SELECT i.id FROM items i WHERE "+where, arguments...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var identifier, parent, unused int
		var detail string
		if err := rows.Scan(&identifier, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
		if strings.HasPrefix(strings.ToUpper(detail), "SCAN I") {
			t.Fatalf("folder query scans all items: %v", plan)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan, "\n"), "idx_item_folders_folder") {
		t.Fatalf("folder query did not use the relation index: %v", plan)
	}
}

func BenchmarkFolderScopeCount30000(b *testing.B) {
	manager := diskWorkspaceSeed(b, 30000)
	for index := 1; index <= 16; index++ {
		if _, err := manager.db.Exec("INSERT INTO item_folders(item_id,folder_id,added_at) VALUES (?,'sparse',1)", fmt.Sprintf("item-%06d", index)); err != nil {
			b.Fatal(err)
		}
	}
	where, arguments := diskItemScope(false, []string{"sparse"})
	for _, scenario := range []struct {
		name  string
		query string
	}{
		{"baseline", "SELECT COUNT(*),COALESCE(SUM(i.size),0) FROM items i WHERE COALESCE(i.in_trash,0)=0 AND EXISTS (SELECT 1 FROM item_folders f WHERE f.item_id=i.id AND f.folder_id IN (?))"},
		{"indexed", "SELECT COUNT(*),COALESCE(SUM(i.size),0) FROM items i WHERE " + where},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var total int
				var size int64
				if err := manager.db.QueryRow(scenario.query, arguments...).Scan(&total, &size); err != nil {
					b.Fatal(err)
				}
				if total != 14 || size != 14*1024 {
					b.Fatalf("folder count=%d size=%d", total, size)
				}
			}
		})
	}
}

func BenchmarkDiskWorkspace30000(b *testing.B) {
	manager := diskWorkspaceSeed(b, 30000)
	b.Run("layout_index", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := manager.WorkspaceIndex(QuerySpec{}, "desc", nil, false, false); err != nil {
				b.Fatal(err)
			}
		}
	})
	ids := make([]string, 96)
	for index := range ids {
		ids[index] = fmt.Sprintf("item-%06d", index)
	}
	b.Run("scroll_page_96", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := manager.workspaceItems(ids, true); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestWorkspaceSortByName(t *testing.T) {
	path := t.TempDir()
	db, err := openLibraryDB(filepath.Join(path, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	manager := &LibraryManager{db: db, activeLib: &LibraryInfo{ID: "sort-name", Path: path}}
	if err := manager.initSchema(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO folders (id, name, parent_id, created_at, sort_order) VALUES ('f1', 'Folder', '', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	seed := []struct {
		id         string
		name       string
		importedAt int64
		inTrash    int
	}{
		{"id1", "img10", 100, 0},
		{"id2", "Beta", 200, 0},
		{"id3", "img2", 300, 0},
		{"id4", "alpha", 400, 0},
		{"t1", "z_trash", 500, 1},
		{"t2", "a_trash", 600, 1},
	}
	for _, s := range seed {
		_, err := db.Exec(`INSERT INTO items
			(id,hex,name,extension,filename,size,mime_type,width,height,has_thumbnail,tags,created_at,imported_at,
			note,prompt,url,rating,in_trash,trashed_at,trashed_folders,dominant_color,palette,meta_json)
			VALUES (?,?,?,'jpg','original.jpg',100,'image/jpeg',100,100,1,'',1,?,'','','',0,?,?,?,'#FF0000','#FF0000','{}')`,
			s.id, "hex-"+s.id, s.name, s.importedAt, s.inTrash, s.importedAt, `["f1"]`)
		if err != nil {
			t.Fatal(err)
		}
		if s.inTrash == 0 {
			if _, err := db.Exec(`INSERT INTO item_folders(item_id,folder_id,added_at) VALUES (?,'f1',1)`, s.id); err != nil {
				t.Fatal(err)
			}
		}
	}

	wantAsc := []string{"alpha", "Beta", "img2", "img10"}
	wantDesc := []string{"img10", "img2", "Beta", "alpha"}

	names := func(items []Item) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.Name
		}
		return out
	}

	// Disk path: WorkspaceIndex
	idxAsc, err := manager.WorkspaceIndex(QuerySpec{}, "name_asc", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(idxAsc.Items); !reflect.DeepEqual(got, wantAsc) {
		t.Fatalf("WorkspaceIndex name_asc = %v, want %v", got, wantAsc)
	}
	idxDesc, err := manager.WorkspaceIndex(QuerySpec{}, "name_desc", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(idxDesc.Items); !reflect.DeepEqual(got, wantDesc) {
		t.Fatalf("WorkspaceIndex name_desc = %v, want %v", got, wantDesc)
	}

	// Disk path: Trash
	trashAsc, err := manager.WorkspaceIndex(QuerySpec{}, "name_asc", nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(trashAsc.Items); !reflect.DeepEqual(got, []string{"a_trash", "z_trash"}) {
		t.Fatalf("WorkspaceIndex trash name_asc = %v, want [a_trash z_trash]", got)
	}

	// Disk path: FolderPreviews
	prevDesc, err := manager.FolderPreviews([][]string{{"f1"}}, "name_desc")
	if err != nil {
		t.Fatal(err)
	}
	if len(prevDesc) != 1 || len(prevDesc[0].Items) != 4 {
		t.Fatalf("FolderPreviews disk unexpected: %+v", prevDesc)
	}

	// In-memory cache path
	if err := manager.ensureCacheLoaded(); err != nil {
		t.Fatal(err)
	}
	pageAsc, err := manager.QueryPage(QuerySpec{}, "name_asc", 0, 10, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(pageAsc.Items); !reflect.DeepEqual(got, wantAsc) {
		t.Fatalf("QueryPage name_asc = %v, want %v", got, wantAsc)
	}
	pageDesc, err := manager.QueryPage(QuerySpec{}, "name_desc", 1, 2, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(pageDesc.Items); !reflect.DeepEqual(got, []string{"img2", "Beta"}) {
		t.Fatalf("QueryPage name_desc offset=1 limit=2 = %v, want [img2 Beta]", got)
	}
	allAsc, err := manager.GetItems("", "name_asc")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(allAsc); !reflect.DeepEqual(got, wantAsc) {
		t.Fatalf("GetItems name_asc = %v, want %v", got, wantAsc)
	}
	trashMemDesc, err := manager.GetTrashItems("", "name_desc")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(trashMemDesc); !reflect.DeepEqual(got, []string{"z_trash", "a_trash"}) {
		t.Fatalf("GetTrashItems name_desc = %v, want [z_trash a_trash]", got)
	}
	memPrevAsc, err := manager.FolderPreviews([][]string{{"f1"}}, "name_asc")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(memPrevAsc[0].Items); !reflect.DeepEqual(got, wantAsc) {
		t.Fatalf("FolderPreviews mem name_asc = %v, want %v", got, wantAsc)
	}
}
