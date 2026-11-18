package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAtomicSaveAndLoadDoesNotPersistTokenInJSON(t *testing.T) {
	dir := t.TempDir()
	l := NewLocal(dir)
	if err := l.SaveSession(Session{BaseURL: "http://host:17878", Token: "secret", LibraryID: "lib-1", LibraryName: "Main"}); err != nil {
		t.Fatal(err)
	}
	rawSession, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawSession), "secret") {
		t.Fatalf("session.json must not contain plaintext token: %s", rawSession)
	}
	rawConn, err := os.ReadFile(filepath.Join(dir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawConn), "secret") {
		t.Fatalf("connections.json must not contain plaintext token: %s", rawConn)
	}
	s, err := l.LoadSession()
	if err != nil || s == nil || s.BaseURL != "http://host:17878" || s.Version != SessionVersion {
		t.Fatalf("session = %+v, %v", s, err)
	}
	info, err := os.Stat(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode = %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestCorruptDocumentIsMovedAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := NewLocal(dir)
	if _, err := l.LoadSession(); err == nil {
		t.Fatal("expected a decode error")
	}
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "corrupt-") {
			found = true
		}
	}
	if !found {
		t.Fatal("corrupt document was not preserved")
	}
}

func TestConnectionHistoryIsCappedAndDeduped(t *testing.T) {
	l := NewLocal(t.TempDir())
	for i := 0; i < 15; i++ {
		url := "http://host" + string(rune('a'+i)) + ":17878"
		if err := l.Remember(Connection{BaseURL: url, LastConnectedAt: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := l.History()
	if err != nil || len(items) != MaxConnections {
		t.Fatalf("history = %d, %v", len(items), err)
	}
	if items[0].BaseURL != "http://hosto:17878" {
		t.Fatalf("newest first, got %q", items[0].BaseURL)
	}
	if err := l.Remember(Connection{BaseURL: "http://hosto:17878", LibraryName: "Updated", LastConnectedAt: 100}); err != nil {
		t.Fatal(err)
	}
	items, _ = l.History()
	if len(items) != MaxConnections || items[0].LibraryName != "Updated" {
		t.Fatalf("dedupe failed: %+v", items[0])
	}
	if err := l.Forget("http://hosto:17878"); err != nil {
		t.Fatal(err)
	}
	items, _ = l.History()
	if len(items) != MaxConnections-1 {
		t.Fatalf("forget left %d", len(items))
	}
}

func TestUsageHistoryIsScopedAndCapped(t *testing.T) {
	l := NewLocal(t.TempDir())
	for i := 0; i < 510; i++ {
		id := "item-" + strings.Repeat("x", i%3) + strconv.Itoa(i)
		if err := l.AddUsage("stable:lib-1", id); err != nil {
			t.Fatal(err)
		}
	}
	u, err := l.LoadUsage()
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Scopes["stable:lib-1"]) != MaxUsage {
		t.Fatalf("usage length = %d", len(u.Scopes["stable:lib-1"]))
	}
	if err := l.AddUsage("stable:lib-1", "last"); err != nil {
		t.Fatal(err)
	}
	u, _ = l.LoadUsage()
	if u.Scopes["stable:lib-1"][0] != "last" {
		t.Fatal("newest usage should be first")
	}
	if len(u.Scopes["legacy:name:Main"]) != 0 {
		t.Fatal("scopes must not mix")
	}
}

func TestMigrationStepsAreIdempotentAndMigrateLegacyFlutterData(t *testing.T) {
	dir := t.TempDir()
	legacyDir := t.TempDir()
	legacyFile := filepath.Join(legacyDir, "ev1", "0_photo.jpg")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyFile, []byte("JPG"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyIdx := filepath.Join(legacyDir, "index.json")
	if err := os.WriteFile(legacyIdx, []byte(`{"version":1,"entries":[{"id":"e1","kind":"file","fileName":"photo.jpg","mimeType":"image/jpeg","size":3,"receivedAt":1234,"storedPath":"ev1/0_photo.jpg"},{"id":"e2","kind":"text","fileName":"note.txt","mimeType":"text/plain","size":2,"receivedAt":1235,"text":"hi"}],"processedEvents":["ev1"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyTags := filepath.Join(legacyDir, "share_tags.json")
	if err := os.WriteFile(legacyTags, []byte(`{"version":1,"marks":[{"id":"ABCD","library":"Main","sharedAt":999}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	l := NewLocal(dir)
	addedInbox, err := l.MigrateLegacyInbox(legacyIdx)
	if err != nil || addedInbox != 2 {
		t.Fatalf("MigrateLegacyInbox = %d, %v", addedInbox, err)
	}
	addedTags, err := l.MigrateLegacyShareTags(legacyTags)
	if err != nil || addedTags != 1 {
		t.Fatalf("MigrateLegacyShareTags = %d, %v", addedTags, err)
	}
	ids, hexes := l.ShareTagSets("stable:lib-1", "legacy:name:Main")
	if !ids["ABCD"] || !hexes["ABCD"] {
		t.Fatalf("migrated share tag missing: %v %v", ids, hexes)
	}
	idx, _ := l.LoadInbox()
	if len(idx.Entries) != 2 || idx.Entries[0].StoredPath == "" {
		t.Fatalf("migrated inbox = %+v", idx.Entries)
	}

	runs := 0
	for i := 0; i < 3; i++ {
		ran, err := l.MarkMigration("import-flutter-inbox", func() error { runs++; return nil })
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && !ran {
			t.Fatal("first run should report true")
		}
		if i > 0 && ran {
			t.Fatal("repeat run should report false")
		}
	}
	if runs != 1 {
		t.Fatalf("step ran %d times", runs)
	}
	m, _ := l.LoadMigration()
	if m.Completed["import-flutter-inbox"] == "" || m.Version != MigrationVersion {
		t.Fatalf("migration state = %+v", m)
	}
}

func TestPreferencesDefaultsNormalizeAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	local := NewLocal(dir)
	prefs, err := local.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if prefs != DefaultPreferences() {
		t.Fatalf("fresh preferences = %+v", prefs)
	}

	want := Preferences{ThemeMode: "dark", LightTheme: "sand-light", DarkTheme: "midnight-dark", SystemAccent: false, GridColumns: 4, GalleryLayout: LayoutGrid, ShareMode: ShareModeChoose, ShareNotifyDisabled: true}
	if err := local.SavePreferences(want); err != nil {
		t.Fatal(err)
	}
	got, err := local.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if got.ThemeMode != want.ThemeMode || got.LightTheme != want.LightTheme || got.DarkTheme != want.DarkTheme || got.SystemAccent != want.SystemAccent || got.GridColumns != want.GridColumns || got.GalleryLayout != want.GalleryLayout || got.ShareMode != want.ShareMode || got.ShareNotifyDisabled != want.ShareNotifyDisabled || got.Version != PreferencesVersion {
		t.Fatalf("loaded preferences = %+v", got)
	}

	bad := Preferences{ThemeMode: "sepia", LightTheme: "", DarkTheme: "", GridColumns: 1, GalleryLayout: "masonry", ShareMode: "invalid"}.Normalize()
	if bad.ThemeMode != "system" || bad.LightTheme != "default-light" || bad.DarkTheme != "default-dark" || bad.GridColumns != 3 || bad.GalleryLayout != LayoutAuto || bad.ShareMode != ShareModeAuto {
		t.Fatalf("normalized = %+v", bad)
	}

	wf := Preferences{GridColumns: 5, GalleryLayout: LayoutWaterfall}.Normalize()
	if wf.GridColumns != 3 || !IsWaterfallLayout(wf.GalleryLayout, wf.GridColumns) {
		t.Fatalf("waterfall clamped = %+v", wf)
	}
	if !IsWaterfallLayout(LayoutAuto, 3) || IsWaterfallLayout(LayoutAuto, 4) || IsWaterfallLayout(LayoutGrid, 2) {
		t.Fatal("IsWaterfallLayout mismatch")
	}
}

func TestSaveHistoryCap300(t *testing.T) {
	local := NewLocal(t.TempDir())
	scope := "lib-1"

	ids := make([]string, 350)
	for i := range ids {
		ids[i] = "id-" + strconv.Itoa(i)
	}
	if err := local.AddSaveHistory(scope, ids); err != nil {
		t.Fatal(err)
	}
	got, err := local.SaveHistoryIDs(scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxSaveHistory {
		t.Fatalf("expected %d entries, got %d", MaxSaveHistory, len(got))
	}
	if got[0] != "id-0" || got[MaxSaveHistory-1] != "id-299" {
		t.Fatalf("unexpected ordering/bounds: first=%s last=%s", got[0], got[MaxSaveHistory-1])
	}

	// Prepending an existing ID moves it to the front without exceeding 300.
	if err := local.AddSaveHistory(scope, []string{"id-50", "new-id"}); err != nil {
		t.Fatal(err)
	}
	got2, err := local.SaveHistoryIDs(scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != MaxSaveHistory || got2[0] != "id-50" || got2[1] != "new-id" {
		t.Fatalf("unexpected deduplicated history: len=%d first=%s second=%s", len(got2), got2[0], got2[1])
	}
}
