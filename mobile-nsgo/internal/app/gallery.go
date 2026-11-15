// Package app holds the client's page state and the use cases that talk to the
// desktop. State types are goroutine-safe and reject results from an older
// session, library, filter or request generation, so a slow reply can never
// overwrite a newer page.
package app

import (
	"sync"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
)

// Gallery is the state of one paginated item list (home, search, trash,
// usage history). The zero value is empty until Reset is called.
type Gallery struct {
	mu          sync.RWMutex
	generation  uint64
	items       []model.ItemSummary
	total       int
	offset      int
	limit       int
	spec        query.Spec
	sort        string
	loading     bool
	loadingMore bool
	err         error
	loadedOnce  bool
}

// NewGallery returns an empty list with the default page size.
func NewGallery() *Gallery {
	return &Gallery{limit: model.DefaultPageSize, sort: model.SortDesc}
}

// Reset starts a new generation for a filter or library change. It clears the
// window immediately and returns the generation to attach to the request.
func (g *Gallery) Reset(spec query.Spec, sort string, limit int) uint64 {
	if sort != model.SortAsc && sort != model.SortDesc {
		sort = model.SortDesc
	}
	if limit <= 0 {
		limit = model.DefaultPageSize
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.generation++
	g.items = nil
	g.total = 0
	g.offset = 0
	g.limit = limit
	g.spec = spec
	g.sort = sort
	g.loading = true
	g.loadingMore = false
	g.err = nil
	g.loadedOnce = false
	return g.generation
}

// Invalidate bumps the generation without clearing data. Use it when the
// library identity or session changed; the next Request uses a new token and
// any in-flight reply is dropped.
func (g *Gallery) Invalidate() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.generation++
	return g.generation
}

// Request starts the first page or the next one. appendPage true keeps the
// loaded window. The token must be passed to Apply.
func (g *Gallery) Request(appendPage bool) (token uint64, spec query.Spec, sort string, offset, limit int, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loading || g.loadingMore {
		return 0, query.Spec{}, "", 0, 0, false
	}
	offset = 0
	if appendPage {
		if len(g.items) >= g.total && g.loadedOnce {
			return 0, query.Spec{}, "", 0, 0, false
		}
		offset = len(g.items)
		g.loadingMore = true
	} else {
		g.loading = true
	}
	g.err = nil
	return g.generation, g.spec, g.sort, offset, g.limit, true
}

// Apply records a page if token is still current. It returns false for a stale
// reply and leaves the newer state untouched.
func (g *Gallery) Apply(token uint64, page model.ItemsPage, appendPage bool, err error) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if token != g.generation {
		return false
	}
	if !appendPage {
		g.loading = false
	}
	g.loadingMore = false
	if err != nil {
		g.err = err
		return true
	}
	g.loadedOnce = true
	g.total = page.Total
	if appendPage {
		seen := make(map[string]bool, len(g.items))
		for _, item := range g.items {
			seen[item.ID] = true
		}
		for _, item := range page.Items {
			if item.ID == "" || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			g.items = append(g.items, item)
		}
	} else {
		g.items = append(g.items[:0], page.Items...)
	}
	g.offset = len(g.items)
	return true
}

// Replace swaps the whole window for a locally updated list, still guarded by
// the generation.
func (g *Gallery) Replace(token uint64, items []model.ItemSummary, total int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if token != g.generation {
		return false
	}
	g.items = append(g.items[:0], items...)
	g.total = total
	g.offset = len(g.items)
	return true
}

// Remove drops one item locally so the grid responds before the refresh lands.
func (g *Gallery) Remove(token uint64, id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if token != g.generation {
		return false
	}
	out := g.items[:0]
	for _, item := range g.items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	if len(out) != len(g.items) && g.total > 0 {
		g.total--
	}
	g.items = out
	g.offset = len(g.items)
	return true
}

// Fail records a load error for the current generation.
func (g *Gallery) Fail(token uint64, err error) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if token != g.generation {
		return false
	}
	g.loading = false
	g.loadingMore = false
	g.err = err
	return true
}

// Snapshot is an immutable view for rendering.
type GallerySnapshot struct {
	Generation  uint64
	Items       []model.ItemSummary
	Total       int
	Spec        query.Spec
	Sort        string
	Loading     bool
	LoadingMore bool
	Err         error
	LoadedOnce  bool
}

func (g *Gallery) Snapshot() GallerySnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	items := make([]model.ItemSummary, len(g.items))
	copy(items, g.items)
	return GallerySnapshot{
		Generation: g.generation, Items: items, Total: g.total,
		Spec: g.spec, Sort: g.sort, Loading: g.loading,
		LoadingMore: g.loadingMore, Err: g.err, LoadedOnce: g.loadedOnce,
	}
}

// WindowLimit returns a page size covering the currently loaded window for
// WebSocket / foreground reloads, clamped between DefaultPageSize and MaxQueryPage.
func (g *Gallery) WindowLimit() int {
	g.mu.RLock()
	n := len(g.items)
	g.mu.RUnlock()
	if n <= model.DefaultPageSize {
		return model.DefaultPageSize
	}
	if n > model.MaxQueryPage {
		return model.MaxQueryPage
	}
	return n
}

// Generation returns the current request generation.
func (g *Gallery) Generation() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.generation
}

// Len returns the loaded item count.
func (g *Gallery) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.items)
}

// Total returns the filtered total, or 0 before the first page.
func (g *Gallery) Total() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.total
}

// Spec returns the active filters.
func (g *Gallery) Spec() query.Spec {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.spec
}
