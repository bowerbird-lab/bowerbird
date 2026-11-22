package core

import (
	"path/filepath"
	"sync"
)

// LibraryPool manages open LibraryManager instances keyed by normalized library
// directory path so multiple desktop windows and the mobile server can operate
// on the same or different libraries independently without colliding.
type LibraryPool struct {
	mu      sync.Mutex
	root    *LibraryManager
	entries map[string]*libraryPoolEntry
}

type libraryPoolEntry struct {
	mgr  *LibraryManager
	refs int
}

func newLibraryPool(root *LibraryManager) *LibraryPool {
	return &LibraryPool{
		root:    root,
		entries: make(map[string]*libraryPoolEntry),
	}
}

// RegisterInitial records an already-opened initial library on the root manager
// so subsequent Acquire calls for the same path share it.
func (p *LibraryPool) RegisterInitial(mgr *LibraryManager) {
	if p == nil || mgr == nil {
		return
	}
	path := mgr.GetActiveLibraryPath()
	if path == "" {
		return
	}
	clean := filepath.Clean(path)
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.entries[clean]; ok {
		if existing.mgr == mgr {
			return
		}
	}
	p.entries[clean] = &libraryPoolEntry{mgr: mgr, refs: 1}
}

// Acquire returns a LibraryManager with libraryPath open, incrementing its
// reference count. If libraryPath is already open in the pool, that manager is
// reused so in-memory caches and item locks are shared.
func (p *LibraryPool) Acquire(libraryPath string) (*LibraryManager, error) {
	clean := filepath.Clean(libraryPath)
	p.mu.Lock()
	if entry, ok := p.entries[clean]; ok && entry.mgr != nil && entry.mgr.GetActiveLibrary() != nil {
		entry.refs++
		mgr := entry.mgr
		p.mu.Unlock()
		return mgr, nil
	}
	p.mu.Unlock()

	sub := p.root.newChildManager()
	if _, err := sub.openLibraryInternal(clean, false); err != nil {
		return nil, err
	}

	p.mu.Lock()
	if entry, ok := p.entries[clean]; ok && entry.mgr != nil && entry.mgr.GetActiveLibrary() != nil {
		// Another goroutine opened it while we were opening; reuse the winner.
		entry.refs++
		winner := entry.mgr
		p.mu.Unlock()
		sub.closeDBOnly()
		return winner, nil
	}
	p.entries[clean] = &libraryPoolEntry{mgr: sub, refs: 1}
	p.mu.Unlock()
	return sub, nil
}

// Release decrements the reference count for mgr and closes its database if no
// callers remain.
func (p *LibraryPool) Release(mgr *LibraryManager) {
	if p == nil || mgr == nil {
		return
	}
	p.mu.Lock()
	var toClose *LibraryManager
	for key, entry := range p.entries {
		if entry.mgr != mgr {
			continue
		}
		entry.refs--
		if entry.refs <= 0 {
			delete(p.entries, key)
			toClose = entry.mgr
		}
		break
	}
	p.mu.Unlock()
	if toClose != nil {
		toClose.closeDBOnly()
	}
}

// AnyActive returns an active LibraryManager from the pool, or nil if none is open.
func (p *LibraryPool) AnyActive() *LibraryManager {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, entry := range p.entries {
		if entry.mgr != nil && entry.mgr.GetActiveLibrary() != nil {
			return entry.mgr
		}
	}
	return nil
}
