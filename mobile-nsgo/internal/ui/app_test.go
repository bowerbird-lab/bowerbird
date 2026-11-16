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

func TestMain(m *testing.M) { nsgotest.Main(m) }

func isolatedRoot(t *testing.T) func(*core.Scope) core.Node {
	dir := t.TempDir()
	return func(cx *core.Scope) core.Node { return rootWithDir(cx, dir) }
}

func TestConnectScreenRendersAndValidatesEmptyAddress(t *testing.T) {
	app := nsgotest.Start(t, isolatedRoot(t), nsgotest.Size(420, 800))
	if v := app.Find("id:connect.address"); v == nil {
		t.Fatal("address field is missing")
	}
	if v := app.Find("id:connect.submit"); v == nil {
		t.Fatal("connect button is missing")
	}
	if v := app.Find("id:connect.scan"); v == nil {
		t.Fatal("scan button is missing")
	}
	app.Press("id:connect.submit")
	if v := app.Find("text:请输入电脑地址"); v == nil {
		t.Fatal("empty address should show an error")
	}
}

func TestAddressInputIsControlled(t *testing.T) {
	app := nsgotest.Start(t, isolatedRoot(t), nsgotest.Size(420, 800))
	app.Type("id:connect.address", "192.168.1.8:17878")
	if v := app.Find("id:connect.address"); v.Text != "192.168.1.8:17878" {
		t.Fatalf("address field = %q", v.Text)
	}
}

func TestMonthGroupingAndDensityAnchorPreservation(t *testing.T) {
	jan := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC).UnixMilli()
	feb := time.Date(2026, 2, 10, 10, 0, 0, 0, time.UTC).UnixMilli()
	items := []model.ItemSummary{
		{ID: "a1", Date: feb},
		{ID: "a2", Date: feb},
		{ID: "a3", Date: feb},
		{ID: "a4", Date: feb},
		{ID: "b1", Date: jan},
	}
	rows3 := BuildMonthRows(items, 3)
	// Feb: header + 2 rows (3 + 1), Jan: header + 1 row (1) => 5 rows
	if len(rows3) != 5 {
		t.Fatalf("rows3 = %d, want 5", len(rows3))
	}
	if rows3[0].Kind != RowMonthHeader || rows3[0].Count != 4 {
		t.Fatalf("row 0 = %+v", rows3[0])
	}
	if rows3[2].Slots[0].FlatIndex != 3 || rows3[4].Slots[0].FlatIndex != 4 {
		t.Fatalf("flat index mismatch: %+v / %+v", rows3[2], rows3[4])
	}
	anchor := FirstItemIDAtRow(rows3, 2)
	if anchor != "a4" {
		t.Fatalf("anchor = %q, want a4", anchor)
	}
	rows2 := BuildMonthRows(items, 2)
	idx2 := RowIndexForItemID(rows2, anchor)
	if rows2[idx2].Slots[1].Item.ID != "a4" {
		t.Fatalf("restored row %d = %+v", idx2, rows2[idx2])
	}
}

func TestPairingToGalleryDrawerFilterAndDetailFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "TestLib", Version: "1.2.0", PairingRequired: true, Capabilities: []string{"query"}})
		case "/api/v1/pair":
			_ = json.NewEncoder(w).Encode(model.PairResult{Token: "tok", Name: "TestLib"})
		case "/api/v1/query":
			_ = json.NewEncoder(w).Encode(model.ItemsPage{
				Total: 1,
				Items: []model.ItemSummary{{
					ID: "item-1", Name: "Sunset", Filename: "sunset.jpg",
					Extension: "jpg", MimeType: "image/jpeg", Size: 1024,
					Width: 800, Height: 600, Date: time.Now().UnixMilli(),
				}},
			})
		case "/api/v1/stats":
			// Simulate older desktop returning 404 -> drawer must show "不可用"
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/folders":
			_ = json.NewEncoder(w).Encode([]model.Folder{{ID: "f1", Name: "Design", ItemCount: 1}})
		case "/api/v1/tags":
			_ = json.NewEncoder(w).Encode([]model.Tag{{Name: "sky", ItemCount: 1}})
		case "/api/v1/colors":
			_ = json.NewEncoder(w).Encode([]string{"#FF6600"})
		case "/api/v1/items/item-1":
			_ = json.NewEncoder(w).Encode(model.ItemDetail{
				ID: "item-1", Name: "Sunset", Filename: "sunset.jpg",
				MimeType: "image/jpeg", Size: 1024, Note: "Golden hour",
			})
		default:
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	app := nsgotest.Start(t, isolatedRoot(t), nsgotest.Size(420, 800))
	app.Type("id:connect.address", srv.URL)
	app.Press("id:connect.submit")
	app.WaitFor("id:connect.code", 2*time.Second)

	app.Type("id:connect.code", "123456")
	app.Press("id:connect.pair")
	app.WaitFor("id:gallery.drawer", 2*time.Second)

	// Open drawer and check stats fallback shows 不可用 rather than 0.
	app.Press("id:gallery.drawer")
	app.WaitFor("id:drawer.all >> text:不可用", 2*time.Second)
	app.WaitFor("text:Design", 2*time.Second)

	// Drag the main panel left to close the drawer, then right to reopen it.
	app.Pan("id:gallery.main", core.Point{X: -40}, core.Point{X: -420})
	app.Advance(600 * time.Millisecond)
	if tx := app.Find("id:gallery.main").Style.Transform.TranslateX; tx > 1 {
		t.Fatalf("drawer closed by drag at x=%v, want 0", tx)
	}
	app.Pan("id:gallery.main", core.Point{X: 40}, core.Point{X: 360})
	app.Advance(600 * time.Millisecond)
	drawerWidth := core.Points(420 * 0.78)
	if tx := app.Find("id:gallery.main").Style.Transform.TranslateX; tx < drawerWidth-2 || tx > drawerWidth+2 {
		t.Fatalf("drawer opened by drag at x=%v, want %v", tx, drawerWidth)
	}

	// Pulling past the open position must spring back without leaving the
	// main panel stuck at an accumulated overscroll offset.
	app.Pan("id:gallery.main", core.Point{X: 40}, core.Point{X: 200})
	app.Advance(600 * time.Millisecond)
	if tx := app.Find("id:gallery.main").Style.Transform.TranslateX; tx < drawerWidth-2 || tx > drawerWidth+2 {
		t.Fatalf("drawer overscroll settled at x=%v, want %v", tx, drawerWidth)
	}
	app.Back()
	app.Advance(400 * time.Millisecond)

	// Open detail (immersive by default), single-tap to show header, then open metadata sheet.
	app.WaitFor("id:gallery.tile", 2*time.Second)
	app.Press("id:gallery.tile >> nth=0")
	app.WaitFor("id:detail.media", 2*time.Second)
	if len(app.QueryMounted("id:screen.gallery")) == 0 {
		t.Fatal("the library left the tree behind the preview, losing its scroll")
	}
	app.Press("id:detail.media")
	app.WaitFor("id:detail.meta", 2*time.Second)
	app.Press("id:detail.meta")
	app.WaitFor("text:Golden hour", 2*time.Second)
	app.Back()
	app.Back()

	// Test search page and query submission.
	app.WaitFor("id:gallery.search", 2*time.Second)
	app.Press("id:gallery.search")
	app.WaitFor("id:search.text", 2*time.Second)
	app.Type("id:search.text", "sunset")
	app.Submit("id:search.text")
	app.WaitFor("text:1 个结果", 2*time.Second)
}

func TestPreviewKeepsGalleryScrollOffset(t *testing.T) {
	items := make([]model.ItemSummary, 0, 40)
	for i := 0; i < 40; i++ {
		items = append(items, model.ItemSummary{
			ID: "item-" + itoa(i), Name: "Item " + itoa(i), Filename: "item-" + itoa(i) + ".jpg",
			Extension: "jpg", MimeType: "image/jpeg", Size: 1024,
			Width: 800, Height: 600, Date: time.Now().Add(-time.Duration(i) * time.Hour).UnixMilli(),
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "Lib", PairingRequired: false, Capabilities: []string{"query"}})
		case "/api/v1/query":
			_ = json.NewEncoder(w).Encode(model.ItemsPage{Total: len(items), Items: items})
		case "/api/v1/stats":
			_ = json.NewEncoder(w).Encode(model.Stats{Total: len(items)})
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()

	app := nsgotest.Start(t, isolatedRoot(t), nsgotest.Size(420, 800))
	app.Type("id:connect.address", srv.URL)
	app.Press("id:connect.submit")
	app.WaitFor("id:gallery.list", 2*time.Second)

	app.DragScroll("id:gallery.list", core.Point{Y: -900}, 0)
	app.Advance(400 * time.Millisecond)
	before := app.QueryMounted("id:gallery.list")[0].Offset.Y
	if before == 0 {
		t.Fatal("the gallery did not scroll")
	}

	app.Press("id:gallery.tile >> nth=0")
	app.WaitFor("id:detail.media", 2*time.Second)
	if len(app.QueryMounted("id:screen.gallery")) == 0 {
		t.Fatal("the library left the tree behind the preview")
	}
	app.Back()
	app.Advance(500 * time.Millisecond)
	app.WaitFor("id:gallery.list", 2*time.Second)

	if after := app.QueryMounted("id:gallery.list")[0].Offset.Y; after != before {
		t.Fatalf("gallery scroll moved from %v to %v after the preview", before, after)
	}
}

func TestFilterSheetAppliesSizeAndRatingQueries(t *testing.T) {
	var seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "Lib", PairingRequired: false, Capabilities: []string{"query"}})
		case "/api/v1/query":
			var body struct {
				Query string `json:"query"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Query != "" {
				seenQuery = body.Query
			}
			_ = json.NewEncoder(w).Encode(model.ItemsPage{Total: 0, Items: []model.ItemSummary{}})
		case "/api/v1/stats":
			_ = json.NewEncoder(w).Encode(model.Stats{Total: 0})
		default:
			w.WriteHeader(200)
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	app := nsgotest.Start(t, isolatedRoot(t), nsgotest.Size(420, 800))
	app.Type("id:connect.address", srv.URL)
	app.Press("id:connect.submit")
	app.WaitFor("id:gallery.more", 2*time.Second)

	app.Press("id:gallery.more")
	app.WaitFor("text:筛选", 2*time.Second)
	app.Press("text:筛选")
	app.WaitFor("text:小 <1MB", 2*time.Second)

	// Drawer items sit on the sheet without a fill until they are touched.
	chip := app.Find("text:小 <1MB")
	if bg := chip.Parent.Style.Background; bg != core.Color(0) {
		t.Fatalf("filter chip has a resting background %#x", bg)
	}
	app.PressDown("text:小 <1MB")
	if bg := app.Find("text:小 <1MB").Parent.Style.Background; bg == core.Color(0) {
		t.Fatal("filter chip has no background while pressed")
	}
	app.PressUp("text:小 <1MB")

	// Dragging the handle moves the visible panel with the finger, then
	// eases it back when the drag is short.
	finger := app.Touch("id:filter.sheet.handle", core.Point{X: 200, Y: 12})
	for y := 23; y <= 100; y += 11 {
		finger.Move(core.Point{X: 200, Y: core.Points(y)})
	}
	if got := app.Find("id:filter.sheet").Children[0].Style.Transform.TranslateY; got < 70 {
		t.Fatalf("sheet follows the handle by %v, want about 88", got)
	}
	finger.Lift()
	app.Advance(400 * time.Millisecond)
	if got := app.Find("id:filter.sheet").Children[0].Style.Transform.TranslateY; got > 1 {
		t.Fatalf("short handle drag settled at %v, want 0", got)
	}

	// The panel body keeps vertical drags for its scroll view; only the
	// handle closes the sheet.
	app.Pan("id:filter.sheet", core.Point{Y: 20}, core.Point{Y: 500})
	app.Advance(400 * time.Millisecond)
	if !app.Exists("id:filter.sheet") {
		t.Fatal("a drag on the filter body closed the sheet")
	}
	app.Pan("id:filter.sheet.handle", core.Point{Y: 20}, core.Point{Y: 500})
	app.Advance(400 * time.Millisecond)
	if app.Exists("id:filter.sheet") {
		t.Fatal("filter sheet stayed after a downward swipe on its handle")
	}

	app.Press("id:gallery.more")
	app.WaitFor("text:筛选", 2*time.Second)
	app.Press("text:筛选")
	app.WaitFor("text:小 <1MB", 2*time.Second)
	app.Press("text:小 <1MB")
	app.Press("text:5★")
	app.Press("id:filter.apply")
	app.WaitFor("id:gallery.more", 2*time.Second)
	if seenQuery != "size:small rating:5" {
		t.Fatalf("seenQuery = %q, want %q", seenQuery, "size:small rating:5")
	}
}

func TestDarkModeAppearanceUpdatesCleanly(t *testing.T) {
	app := nsgotest.Start(t, isolatedRoot(t), nsgotest.Size(420, 800),
		nsgotest.Environment(core.Environment{Dark: true}))
	if v := app.Find("text:Bowerbird"); v == nil {
		t.Fatal("connect screen missing in dark mode")
	}
	app.SetEnvironment(core.Environment{Dark: false})
	if v := app.Find("id:connect.address"); v == nil {
		t.Fatal("connect address missing after toggling light mode")
	}
}

func TestSettingsScreenSelectsAndPersistsThemes(t *testing.T) {
	dir := t.TempDir()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return rootWithDir(cx, dir) }, nsgotest.Size(420, 900))

	app.Press("id:connect.settings")
	app.WaitFor("id:settings.header", 2*time.Second)
	if v := app.Find("text:颜色模式"); v == nil {
		t.Fatal("appearance card is missing")
	}

	app.Press("id:settings.mode >> text:深色")
	app.WaitFor("id:settings.lightTheme", 2*time.Second)

	app.Press("id:settings.lightTheme")
	app.WaitFor("id:settings.theme.sand-light", 2*time.Second)
	app.Press("id:settings.theme.sand-light")
	app.WaitForGone("id:settings.themePicker", 2*time.Second)
	app.FastForward(400 * time.Millisecond)

	app.Press("id:settings.darkTheme")
	app.WaitFor("id:settings.theme.midnight-dark", 2*time.Second)
	app.Press("id:settings.theme.midnight-dark")
	app.WaitForGone("id:settings.themePicker", 2*time.Second)
	app.FastForward(400 * time.Millisecond)

	app.Press("id:settings.gridMore")

	prefs, err := storage.NewLocal(dir).LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if prefs.ThemeMode != "dark" || prefs.LightTheme != "sand-light" || prefs.DarkTheme != "midnight-dark" || prefs.GridColumns != 4 {
		t.Fatalf("persisted preferences = %+v", prefs)
	}

	// The settings screen returns to the connect screen it was opened from.
	app.Press("id:settings.back")
	app.WaitFor("id:connect.address", 2*time.Second)
	if app.Exists("id:settings.header") {
		t.Fatal("settings stayed open after back")
	}
}

func TestThemeSettingsResetAndReloadKeepDefaults(t *testing.T) {
	dir := t.TempDir()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return rootWithDir(cx, dir) }, nsgotest.Size(420, 900))
	app.Press("id:connect.settings")
	app.WaitFor("id:settings.themes.reset", 2*time.Second)
	app.Press("id:settings.themes.reset")
	app.Press("id:settings.themes.reload")
	app.Press("id:settings.gridMore")

	prefs, err := storage.NewLocal(dir).LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if prefs.ThemeMode != "system" || prefs.LightTheme != "default-light" || prefs.DarkTheme != "default-dark" {
		t.Fatalf("preferences after reset = %+v", prefs)
	}
}

func TestPreviewMatchAnimatesAndCleansUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "Lib", PairingRequired: false, Capabilities: []string{"query"}})
		case "/api/v1/query":
			_ = json.NewEncoder(w).Encode(model.ItemsPage{
				Total: 1,
				Items: []model.ItemSummary{{
					ID: "item-1", Name: "One", Filename: "one.jpg",
					Extension: "jpg", MimeType: "image/jpeg", Size: 1024,
					Width: 800, Height: 600, Date: time.Now().UnixMilli(),
				}},
			})
		case "/api/v1/stats":
			_ = json.NewEncoder(w).Encode(model.Stats{Total: 1})
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()

	var appState *App
	dir := t.TempDir()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		node := rootWithDir(cx, dir)
		appState = node.(*App)
		appState.forceMatchAnim = true
		return node
	}, nsgotest.Size(420, 800))

	app.Type("id:connect.address", srv.URL)
	app.Press("id:connect.submit")
	app.WaitFor("id:gallery.tile", 2*time.Second)

	app.Press("id:gallery.tile >> nth=0")
	if len(app.QueryMounted("id:detail.media")) != 1 {
		t.Fatal("expected detail.media to mount after open hero flight")
	}
	// After handoff, preview.match is unmounted so nothing stays stuck.
	app.Advance(180 * time.Millisecond)
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("preview.match stayed mounted after handoff: %d", n)
	}

	// Closing via back button reveals gallery and cleans up preview.match.
	app.Press("id:detail.media")
	app.WaitFor("id:detail.back", 2*time.Second)
	app.Press("id:detail.back")
	app.Advance(previewMatchCloseDuration + 80*time.Millisecond)
	if len(app.QueryMounted("id:detail.media")) != 0 {
		t.Fatal("expected detail.media to unmount after close")
	}
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("close preview.match stayed mounted after finish: %d", n)
	}

	// Inspect frame 0 via Post+Settle: preview.match mounts at the contain frame while gallery stays visible.
	appState.scope.Post(func() {
		appState.openDetail(appState.scope, 0, screenGallery, core.Bounds{
			Origin: core.Point{X: 12, Y: 94},
			Size:   core.Size{Width: 128, Height: 128},
		})
	})
	app.Settle()
	match := app.QueryMounted("id:preview.match")
	if len(match) != 1 || match[0].Frame.Size.Width != 420 || match[0].Frame.Size.Height != 315 {
		t.Fatalf("frame 0 preview.match = %+v, want contain frame 420x315", match)
	}
	if len(app.QueryMounted("id:detail.media")) != 0 {
		t.Fatal("expected detail.media to wait until hero flight finishes")
	}
	app.Advance(previewMatchOpenDuration + 180*time.Millisecond)
	if len(app.QueryMounted("id:detail.media")) != 1 {
		t.Fatal("expected detail.media after advancing open hero flight")
	}
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("preview.match stayed mounted after advancing handoff: %d", n)
	}

	// System back button/gesture also triggers the shrink-back animation and cleans up.
	app.Back()
	app.Advance(previewMatchCloseDuration + 80*time.Millisecond)
	if len(app.QueryMounted("id:detail.media")) != 0 {
		t.Fatal("expected detail.media to unmount after system back")
	}
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("system back preview.match stayed mounted after finish: %d", n)
	}

	// Interactive predictive back: start -> scrub -> cancel restores detail cleanly.
	appState.scope.Post(func() {
		appState.openDetail(appState.scope, 0, screenGallery, core.Bounds{
			Origin: core.Point{X: 12, Y: 94},
			Size:   core.Size{Width: 128, Height: 128},
		})
	})
	app.Settle()
	app.Advance(previewMatchOpenDuration + 180*time.Millisecond)
	appState.scope.Post(func() {
		appState.startInteractiveBack(0.1)
	})
	app.Settle()
	app.Advance(20 * time.Millisecond) // RequestAnimationFrame reveals gallery behind hero
	if len(app.QueryMounted("id:preview.match")) != 1 {
		t.Fatal("expected preview.match during interactive predictive back")
	}
	appState.scope.Post(func() {
		appState.updateInteractiveBack(0.6)
		appState.finishInteractiveBack(false)
	})
	app.Settle()
	app.Advance(360 * time.Millisecond)
	if len(app.QueryMounted("id:detail.media")) != 1 {
		t.Fatal("expected detail.media restored after cancelled predictive back")
	}
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("preview.match stayed mounted after cancelled predictive back: %d", n)
	}

	// Interactive predictive back: start -> scrub -> invoke shrinks into tile and cleans up.
	appState.scope.Post(func() {
		appState.startInteractiveBack(0.1)
	})
	app.Settle()
	app.Advance(20 * time.Millisecond)
	appState.scope.Post(func() {
		appState.updateInteractiveBack(0.8)
		appState.finishInteractiveBack(true)
	})
	app.Settle()
	app.Advance(260 * time.Millisecond)
	if len(app.QueryMounted("id:detail.media")) != 0 {
		t.Fatal("expected detail.media unmounted after invoked predictive back")
	}
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("preview.match stayed mounted after invoked predictive back: %d", n)
	}

	// Opening without valid source bounds skips the proxy entirely.
	appState.scope.Post(func() {
		appState.screen.Set(screenGallery)
		appState.openDetail(appState.scope, 0, screenGallery, core.Bounds{})
	})
	app.Settle()
	if n := len(app.QueryMounted("id:preview.match")); n != 0 {
		t.Fatalf("expected no preview.match without source bounds, got %d", n)
	}
	if len(app.QueryMounted("id:detail.media")) != 1 {
		t.Fatal("expected detail.media without source bounds")
	}
}
