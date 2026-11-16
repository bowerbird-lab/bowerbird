package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/platform"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func TestGalleryRejectsStaleGenerations(t *testing.T) {
	g := NewGallery()
	first := g.Reset(query.Spec{Text: "old"}, model.SortDesc, 60)
	second := g.Reset(query.Spec{Text: "new"}, model.SortDesc, 60)
	if first == second {
		t.Fatal("generation did not advance")
	}
	if g.Apply(first, model.ItemsPage{Total: 10, Items: []model.ItemSummary{{ID: "old"}}}, false, nil) {
		t.Fatal("stale apply should be rejected")
	}
	if g.Len() != 0 {
		t.Fatal("stale page entered the state")
	}
	if !g.Apply(second, model.ItemsPage{Total: 1, Items: []model.ItemSummary{{ID: "new"}}}, false, nil) {
		t.Fatal("current apply should be accepted")
	}
	snap := g.Snapshot()
	if len(snap.Items) != 1 || snap.Items[0].ID != "new" || snap.Total != 1 || snap.Loading {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestGalleryPaginationDedupesAndStopsAtTotal(t *testing.T) {
	g := NewGallery()
	token := g.Reset(query.Spec{}, model.SortDesc, 2)
	if !g.Apply(token, model.ItemsPage{Total: 3, Items: []model.ItemSummary{{ID: "a"}, {ID: "b"}}}, false, nil) {
		t.Fatal("first page rejected")
	}
	tok, _, _, offset, _, ok := g.Request(true)
	if !ok || offset != 2 {
		t.Fatalf("request next = %d, %v, %v", offset, ok, tok)
	}
	g.Apply(tok, model.ItemsPage{Total: 3, Items: []model.ItemSummary{{ID: "b"}, {ID: "c"}}}, true, nil)
	if g.Len() != 3 {
		t.Fatalf("len = %d, want 3", g.Len())
	}
	if _, _, _, _, _, ok := g.Request(true); ok {
		t.Fatal("must not ask past the total")
	}
}

func TestGalleryRemoveAndLocalTotal(t *testing.T) {
	g := NewGallery()
	token := g.Reset(query.Spec{}, model.SortDesc, 60)
	g.Apply(token, model.ItemsPage{Total: 2, Items: []model.ItemSummary{{ID: "a"}, {ID: "b"}}}, false, nil)
	if !g.Remove(token, "a") || g.Total() != 1 || g.Len() != 1 {
		t.Fatalf("remove = %d/%d", g.Len(), g.Total())
	}
}

func TestSessionConnectPairsAndSaves(t *testing.T) {
	var paired bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			_ = json.NewEncoder(w).Encode(model.Status{Name: "Main Library", PairingRequired: true, Capabilities: []string{"query"}})
		case "/api/v1/pair":
			paired = true
			var req model.PairRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Code != "123456" {
				w.WriteHeader(401)
				return
			}
			_ = json.NewEncoder(w).Encode(model.PairResult{Token: "tok"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	local := storage.NewLocal(t.TempDir())
	client := transport.New(transport.Config{})
	secure := platform.NewMemorySecureStore()
	m := NewSessionManager(client, local, secure)
	if _, err := m.Connect(context.Background(), srv.URL, "", "Phone"); err != ErrPairingRequired {
		t.Fatalf("no code = %v, want ErrPairingRequired", err)
	}
	if paired {
		t.Fatal("must not pair without a code")
	}
	s, err := m.Connect(context.Background(), srv.URL, "123456", "Phone")
	if err != nil || s.Token != "tok" || s.LibraryName != "Main Library" || !paired {
		t.Fatalf("Connect = %+v, %v", s, err)
	}
	loaded, err := local.LoadSession()
	if err != nil || loaded == nil || loaded.Token != "" {
		t.Fatalf("saved session JSON must not contain token: %+v, %v", loaded, err)
	}
	restored, err := NewSessionManager(transport.New(transport.Config{}), local, secure).Restore()
	if err != nil || restored == nil || restored.Token != "tok" {
		t.Fatalf("restored session = %+v, %v", restored, err)
	}
	history, _ := local.History()
	if len(history) != 1 || history[0].BaseURL != srv.URL {
		t.Fatalf("history = %+v", history)
	}
	if m.ScopeKey() != "legacy:name:Main Library" {
		t.Fatalf("scope = %q", m.ScopeKey())
	}
	if err := m.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if current, _ := local.LoadSession(); current != nil {
		t.Fatal("session should be cleared")
	}
	if history, _ := local.History(); len(history) != 1 {
		t.Fatal("history should survive disconnect")
	}
}

func TestProbeCandidatesPicksFirstReachable(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.Status{Name: "Here"})
	}))
	defer good.Close()
	var host string
	var port int
	// Extract from http://127.0.0.1:port
	host = good.URL[len("http://"):]
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			host, port = host[:i], atoi(host[i+1:])
			break
		}
	}
	client := transport.New(transport.Config{})
	base, status, err := ProbeCandidates(context.Background(), client, []string{"127.0.0.1", ""}, port)
	if err != nil || status.Name != "Here" || base == "" {
		t.Fatalf("probe = %q %+v %v", base, status, err)
	}
}

func TestProbeCandidatesSkipsUnreachableAndReportsProgress(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.Status{Name: "Reachable"})
	}))
	defer good.Close()

	var progress []ProbeProgress
	client := transport.New(transport.Config{})
	base, status, err := ProbeCandidatesWithProgress(context.Background(), client,
		[]string{"127.0.0.1:1", bad.URL, good.URL, good.URL}, 17878,
		func(update ProbeProgress) { progress = append(progress, update) })
	if err != nil || base != good.URL || status.Name != "Reachable" {
		t.Fatalf("probe = %q %+v %v", base, status, err)
	}
	if len(progress) < 4 {
		t.Fatalf("progress = %+v, want attempts for unreachable and reachable candidates", progress)
	}
	if progress[len(progress)-1].Responsive != true {
		t.Fatalf("last progress = %+v, want responsive", progress[len(progress)-1])
	}
}

func TestCandidateBaseURLsNormalizeDedupeAndKeepOrder(t *testing.T) {
	got := candidateBaseURLs([]string{
		" 192.168.1.8 ",
		"192.168.1.8:17878",
		"192.168.1.8",
		"[fd00::1]",
		"http://example.com:1234",
		"",
	}, 17878)
	want := []string{
		"http://192.168.1.8:17878",
		"http://[fd00::1]:17878",
		"http://example.com:1234",
	}
	if len(got) != len(want) {
		t.Fatalf("candidateBaseURLs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidateBaseURLs[%d] = %q, want %q (%#v)", i, got[i], want[i], got)
		}
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func TestClassifyEvent(t *testing.T) {
	cases := []struct {
		ev   model.WSEvent
		want EventAction
	}{
		{model.WSEvent{Event: model.EventItemsUpdated}, EventRefresh},
		{model.WSEvent{Event: model.EventLibraryChanged}, EventLibraryChanged},
		{model.WSEvent{Event: model.EventPairingRevoked}, EventRevoked},
		{model.WSEvent{Event: "unknown"}, EventIgnore},
	}
	for _, c := range cases {
		if got, _ := ClassifyEvent(c.ev); got != c.want {
			t.Errorf("%s = %v, want %v", c.ev.Event, got, c.want)
		}
	}
}

type stubIntake struct {
	events []platform.ShareEvent
	acked  []string
}

func (s *stubIntake) TakePending(done func([]platform.ShareEvent, error)) {
	done(append([]platform.ShareEvent(nil), s.events...), nil)
}

func (s *stubIntake) Ack(ids []string, done func(error)) {
	s.acked = append(s.acked, ids...)
	done(nil)
}

func TestInboxConsumePeekPersistAckIdempotency(t *testing.T) {
	dir := t.TempDir()
	local := storage.NewLocal(dir)
	inbox := NewInbox(local)
	intake := &stubIntake{
		events: []platform.ShareEvent{{
			ID: "ev-1",
			Items: []platform.SharedItem{{
				Kind: "text",
				Name: "hello.txt",
				Text: "hello world",
			}},
		}},
	}
	var added int
	var err error
	inbox.Consume(context.Background(), intake, func(a int, e error) {
		added, err = a, e
	})
	if err != nil || added != 1 || len(intake.acked) != 1 || intake.acked[0] != "ev-1" {
		t.Fatalf("first consume = %d, %v, acked=%v", added, err, intake.acked)
	}
	// Replay same event after simulated crash before ack: must ack without duplicating.
	inbox.Consume(context.Background(), intake, func(a int, e error) {
		added, err = a, e
	})
	if err != nil || added != 0 {
		t.Fatalf("replay consume = %d, %v", added, err)
	}
	entries, _ := inbox.Entries()
	if len(entries) != 1 || entries[0].Text != "hello world" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGalleryWindowLimitClamps(t *testing.T) {
	g := NewGallery()
	if g.WindowLimit() != model.DefaultPageSize {
		t.Fatalf("empty window = %d", g.WindowLimit())
	}
	tok := g.Reset(query.Spec{}, model.SortDesc, 200)
	items := make([]model.ItemSummary, 150)
	for i := range items {
		items[i] = model.ItemSummary{ID: "id-" + string(rune('a'+i%26)) + string(rune('0'+i/26))}
	}
	g.Apply(tok, model.ItemsPage{Total: 200, Items: items}, false, nil)
	if g.WindowLimit() != 150 {
		t.Fatalf("window = %d, want 150", g.WindowLimit())
	}
}
