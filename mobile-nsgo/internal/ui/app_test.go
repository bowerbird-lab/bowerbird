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
	app.Back()

	// Open detail (immersive by default), single-tap to show header, then open metadata sheet.
	app.WaitFor("id:gallery.tile", 2*time.Second)
	app.Press("id:gallery.tile")
	app.WaitFor("id:detail.media", 2*time.Second)
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
	app.Press("id:search.submit")
	app.WaitFor("text:1 个结果", 2*time.Second)
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
