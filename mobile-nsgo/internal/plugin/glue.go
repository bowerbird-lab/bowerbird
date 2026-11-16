package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// PreviewPluginDescriptor is the cross-platform preview plugin contract
// received from the connected PC.
type PreviewPluginDescriptor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Renderer     string   `json:"renderer"`
	Entry        string   `json:"entry,omitempty"`
	HTML         string   `json:"html,omitempty"`
	Extensions   []string `json:"extensions,omitempty"`
	MimePrefixes []string `json:"mimePrefixes,omitempty"`
	SupportsZoom bool     `json:"supportsZoom,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Platforms    []string `json:"platforms,omitempty"`
	Script       string   `json:"script,omitempty"`
	Enabled      bool     `json:"enabled"`
}

// PreviewSyncBundle is the preview plugin bundle pushed by a PC host.
type PreviewSyncBundle struct {
	HostID      string                    `json:"hostId"`
	HostName    string                    `json:"hostName"`
	Count       int                       `json:"count"`
	Hash        string                    `json:"hash"`
	GeneratedAt time.Time                 `json:"generatedAt"`
	Plugins     []PreviewPluginDescriptor `json:"plugins"`
}

// PreviewGlue is the mobile-nsgo glue runtime that stores per-PC preview
// plugin bundles and resolves which renderer to use for any previewable item.
type PreviewGlue struct {
	mu       sync.RWMutex
	cacheDir string
	hostID   string
	hostName string
	hash     string
	plugins  []PreviewPluginDescriptor
}

var unsafeHostIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// NewPreviewGlue creates a PreviewGlue instance backed by cacheDir for per-PC storage.
func NewPreviewGlue(cacheDir string) *PreviewGlue {
	return &PreviewGlue{cacheDir: cacheDir}
}

// HostCachePath returns the isolated cache file path for a specific PC hostID.
func (g *PreviewGlue) HostCachePath(hostID string) string {
	safe := unsafeHostIDChars.ReplaceAllString(strings.TrimSpace(hostID), "_")
	if safe == "" {
		safe = "default"
	}
	return filepath.Join(g.cacheDir, "plugins", safe, "preview_bundle.json")
}

// ApplyBundle updates the active preview plugins from a PC sync bundle and
// persists it to that PC's isolated cache.
func (g *PreviewGlue) ApplyBundle(bundle PreviewSyncBundle) error {
	sorted := make([]PreviewPluginDescriptor, 0, len(bundle.Plugins))
	for _, p := range bundle.Plugins {
		if !p.Enabled {
			continue
		}
		if len(p.Platforms) > 0 && !slices.ContainsFunc(p.Platforms, func(pl string) bool {
			return strings.EqualFold(strings.TrimSpace(pl), "mobile")
		}) {
			continue
		}
		sorted = append(sorted, p)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		return sorted[i].ID < sorted[j].ID
	})

	raw, _ := json.Marshal(sorted)
	sum := sha256.Sum256(raw)
	computedHash := hex.EncodeToString(sum[:])

	g.mu.Lock()
	g.hostID = bundle.HostID
	g.hostName = bundle.HostName
	g.hash = computedHash
	g.plugins = sorted
	g.mu.Unlock()

	if g.cacheDir != "" && strings.TrimSpace(bundle.HostID) != "" {
		return g.saveBundle(bundle)
	}
	return nil
}

func (g *PreviewGlue) saveBundle(bundle PreviewSyncBundle) error {
	path := g.HostCachePath(bundle.HostID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadFromCache loads the previously synced preview plugins for a specific PC hostID.
func (g *PreviewGlue) LoadFromCache(hostID string) (bool, error) {
	if g.cacheDir == "" || strings.TrimSpace(hostID) == "" {
		return false, nil
	}
	path := g.HostCachePath(hostID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var bundle PreviewSyncBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return false, err
	}
	if err := g.ApplyBundle(bundle); err != nil {
		return false, err
	}
	return true, nil
}

// NeedsSync reports whether the local cached plugins differ from the PC's
// reported plugin count or SHA-256 hash.
func (g *PreviewGlue) NeedsSync(remoteCount int, remoteHash string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if len(g.plugins) != remoteCount {
		return true
	}
	return g.hash != strings.TrimSpace(remoteHash)
}

// Plugins returns a copy of the currently active preview plugins.
func (g *PreviewGlue) Plugins() []PreviewPluginDescriptor {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]PreviewPluginDescriptor, len(g.plugins))
	copy(out, g.plugins)
	return out
}

// ResolvePreview matches a file extension or MIME type against the synced
// preview plugins using the same priority rules as the desktop host.
func (g *PreviewGlue) ResolvePreview(ext, mime string) (PreviewPluginDescriptor, bool) {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	mime = strings.ToLower(strings.TrimSpace(mime))

	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, p := range g.plugins {
		if ext != "" && slices.ContainsFunc(p.Extensions, func(e string) bool {
			return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e)), ".") == ext
		}) {
			return p, true
		}
		if mime != "" && slices.ContainsFunc(p.MimePrefixes, func(prefix string) bool {
			return strings.HasPrefix(mime, strings.ToLower(strings.TrimSpace(prefix)))
		}) {
			return p, true
		}
	}
	return PreviewPluginDescriptor{}, false
}
