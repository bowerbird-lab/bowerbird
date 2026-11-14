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
