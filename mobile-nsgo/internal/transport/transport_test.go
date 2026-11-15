package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"192.168.1.2:17878":      "http://192.168.1.2:17878",
		"http://host:17878/":     "http://host:17878",
		"https://host/base/":     "https://host/base",
		"http://100.80.20.10":    "http://100.80.20.10",
		" http://host:17878/\n ": "http://host:17878",
	}
	for in, want := range cases {
		got, err := NormalizeBaseURL(in)
		if err != nil {
			t.Fatalf("NormalizeBaseURL(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "ftp://host", "http://"} {
		if _, err := NormalizeBaseURL(bad); err == nil {
			t.Errorf("NormalizeBaseURL(%q) should fail", bad)
		}
	}
}

func TestStatusPairAndErrors(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "Lib", Version: "1.2.0", PairingRequired: true, Capabilities: []string{"query"}})
		case "/api/v1/pair":
			var req model.PairRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Code != "123456" {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "invalid_code", "message": "expired"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok", "name": "Bowerbird"})
		case "/api/v1/items/x":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(model.ItemDetail{ID: "x", Name: "Cat"})
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	st, err := c.Status(context.Background())
	if err != nil || st.Name != "Lib" || !st.HasCapability("query") {
		t.Fatalf("Status = %+v, %v", st, err)
	}
	if _, err := c.Pair(context.Background(), "000000"); KindOf(err) != KindUnauthorized {
		t.Fatalf("bad pair kind = %v, err = %v", KindOf(err), err)
	}
	pr, err := c.Pair(context.Background(), "123456")
	if err != nil || pr.Token != "tok" {
		t.Fatalf("Pair = %+v, %v", pr, err)
	}
	c.SetSession(srv.URL, pr.Token)
	item, err := c.Item(context.Background(), "x")
	if err != nil || item.Name != "Cat" || gotAuth != "Bearer tok" {
		t.Fatalf("Item = %+v, %v, auth=%q", item, err, gotAuth)
	}
}

func TestListFallsBackOnlyWhenFiltersAreExpressible(t *testing.T) {
	var queryCalls, itemCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/query":
			queryCalls++
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "NOT_FOUND", "message": "no query"})
		case "/api/v1/items":
			itemCalls++
			if r.URL.Query().Get("q") != "cat" {
				t.Errorf("legacy q = %q", r.URL.Query().Get("q"))
			}
			_ = json.NewEncoder(w).Encode(model.ItemsPage{Total: 1, Items: []model.ItemSummary{{ID: "a"}}})
		}
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Token: "tok"})

	page, err := c.List(context.Background(), query.Spec{Text: "cat"}, model.SortDesc, 0, 60)
	if err != nil || page.Total != 1 || itemCalls != 1 || queryCalls != 1 {
		t.Fatalf("List basic = %+v, %v (q=%d items=%d)", page, err, queryCalls, itemCalls)
	}
	if c.QuerySupported() {
		t.Fatal("query support should be false after 404")
	}

	_, err = c.List(context.Background(), query.Spec{Text: "cat", Rating: 5}, model.SortDesc, 0, 60)
	if !IsUnsupported(err) {
		t.Fatalf("advanced filter on legacy = %v, want unsupported", err)
	}
	if itemCalls != 1 {
		t.Fatal("must not fetch a broad page for an unsupported filter")
	}
}

func TestListUsesQueryWhenAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		var body struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Limit != 1 && (body.Query != "rating:5" || body.Limit != 61) {
			t.Errorf("query body = %+v", body)
		}
		_ = json.NewEncoder(w).Encode(model.ItemsPage{Total: 2, Limit: 61, Items: []model.ItemSummary{{ID: "a"}, {ID: "b"}}})
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Token: "tok"})
	page, err := c.List(context.Background(), query.Spec{Rating: 5}, model.SortDesc, 0, 61)
	if err != nil || page.Total != 2 || !c.QuerySupported() {
		t.Fatalf("List = %+v, %v, supported=%v", page, err, c.QuerySupported())
	}
}

func TestDownloadUsesPartAndRenames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/originals/x" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, "hello media")
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Token: "tok"})
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "file.bin")
	var reported int64
	n, err := c.Download(context.Background(), "x", dest, func(loaded, total int64) { reported = loaded })
	if err != nil || n != 11 || reported != 11 {
		t.Fatalf("Download = %d, %v, reported=%d", n, err, reported)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "hello media" {
		t.Fatalf("dest = %q, %v", data, err)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file should be gone")
	}
}

func TestUploadStreamsMultipartFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/upload" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			w.WriteHeader(400)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if header.Filename != "cat.png" || string(data) != "PNGDATA" {
			t.Errorf("upload = %q %q", header.Filename, data)
		}
		_ = json.NewEncoder(w).Encode(model.UploadResult{Status: "success", Count: 1})
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Token: "tok"})
	res, err := c.Upload(context.Background(), "cat.png", strings.NewReader("PNGDATA"), 7)
	if err != nil || res.Count != 1 {
		t.Fatalf("Upload = %+v, %v", res, err)
	}
}

func TestValidatePairPayload(t *testing.T) {
	ips, port, code, err := ValidatePairPayload([]byte(`{"v":1,"ips":["192.168.1.2",""],"port":17878,"code":"493644"}`))
	if err != nil || port != 17878 || code != "493644" || len(ips) != 1 {
		t.Fatalf("payload = %v %d %q %v", ips, port, code, err)
	}
	if _, _, _, err := ValidatePairPayload([]byte(`{"ips":[]}`)); err == nil {
		t.Fatal("empty payload should fail")
	}
}

func TestRedactToken(t *testing.T) {
	got := Redact("http://host/api/v1/thumbs/x?token=secret&v=1")
	if strings.Contains(got, "secret") || !strings.Contains(got, "redacted") {
		t.Fatalf("Redact = %q", got)
	}
}

func TestStatsUnsupportedAndCollectionRestore(t *testing.T) {
	var emptied, restored bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/stats":
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v1/items/collection" && r.Method == http.MethodGet:
			if r.URL.Query().Get("name") != model.CollectionTrash {
				t.Errorf("collection = %s", r.URL.Query().Get("name"))
			}
			_ = json.NewEncoder(w).Encode(model.ItemsPage{Total: 1, Items: []model.ItemSummary{{ID: "t1"}}})
		case r.URL.Path == "/api/v1/items/collection" && r.Method == http.MethodDelete:
			emptied = true
			_ = json.NewEncoder(w).Encode(map[string]any{"emptied": true})
		case r.URL.Path == "/api/v1/items/t1/restore" && r.Method == http.MethodPost:
			restored = true
			_ = json.NewEncoder(w).Encode(map[string]any{"restored": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Token: "tok"})
	if _, err := c.Stats(context.Background()); !IsUnsupported(err) {
		t.Fatalf("stats 404 should be KindUnsupported, got %v", err)
	}
	page, err := c.Collection(context.Background(), model.CollectionTrash, 0, 60)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "t1" {
		t.Fatalf("collection = %+v, %v", page, err)
	}
	if err := c.Restore(context.Background(), "t1"); err != nil || !restored {
		t.Fatalf("restore = %v, %v", restored, err)
	}
	if err := c.EmptyTrash(context.Background()); err != nil || !emptied {
		t.Fatalf("empty trash = %v, %v", emptied, err)
	}
}

func TestBatchOperationsReportPartialFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/bad/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Token: "tok"})
	res := c.BatchAddTag(context.Background(), []string{"ok1", "bad", "ok2"}, "design", nil)
	if res.Total != 3 || res.Succeeded != 2 || len(res.Failures) != 1 || res.Failures[0].ID != "bad" {
		t.Fatalf("BatchAddTag = %+v", res)
	}
	fRes := c.BatchAddFolder(context.Background(), []string{"ok1", "bad"}, "f1", nil)
	if fRes.Total != 2 || fRes.Succeeded != 1 || len(fRes.Failures) != 1 {
		t.Fatalf("BatchAddFolder = %+v", fRes)
	}
}
