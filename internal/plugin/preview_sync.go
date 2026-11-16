package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"
)

// PreviewPluginDescriptor is the cross-platform preview plugin contract shared
// between desktop (PC) and mobile-nsgo clients.
type PreviewPluginDescriptor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Renderer     string   `json:"renderer"`
	Entry        string   `json:"entry,omitempty"`
	Extensions   []string `json:"extensions,omitempty"`
	MimePrefixes []string `json:"mimePrefixes,omitempty"`
	SupportsZoom bool     `json:"supportsZoom,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Platforms    []string `json:"platforms,omitempty"`
	Enabled      bool     `json:"enabled"`
}

// PreviewSyncBundle is the payload pushed from PC to connected mobile clients
// so mobile-nsgo can render previewable items using the exact same plugin rules.
type PreviewSyncBundle struct {
	HostID      string                    `json:"hostId"`
	HostName    string                    `json:"hostName"`
	Count       int                       `json:"count"`
	Hash        string                    `json:"hash"`
	GeneratedAt time.Time                 `json:"generatedAt"`
	Plugins     []PreviewPluginDescriptor `json:"plugins"`
}

// IsPreviewPlugin reports whether a manifest declares preview capabilities.
func IsPreviewPlugin(m Manifest) bool {
	if strings.EqualFold(strings.TrimSpace(m.Kind), "preview") {
		return true
	}
	if strings.TrimSpace(m.Preview.Renderer) != "" || strings.TrimSpace(m.Native) != "" {
		return true
	}
	return slices.ContainsFunc(m.Capabilities, func(c string) bool {
		return strings.EqualFold(strings.TrimSpace(c), "preview")
	})
}

// ToPreviewDescriptor converts a Manifest into a cross-platform PreviewPluginDescriptor.
func ToPreviewDescriptor(m Manifest, enabled bool) PreviewPluginDescriptor {
	renderer := strings.TrimSpace(m.Preview.Renderer)
	if renderer == "" {
		renderer = strings.TrimSpace(m.Native)
	}
	exts := m.Preview.Extensions
	if len(exts) == 0 {
		exts = m.Extensions
	}
	mimes := m.Preview.MimePrefixes
	if len(mimes) == 0 {
		mimes = m.MimePrefixes
	}
	priority := m.Preview.Priority
	if priority == 0 {
		priority = m.Priority
	}
	platforms := m.Preview.Platforms
	if len(platforms) == 0 {
		platforms = m.Platforms
	}
	if len(platforms) == 0 {
		platforms = []string{"desktop", "mobile"}
	}
	return PreviewPluginDescriptor{
		ID:           m.ID,
		Name:         m.Name,
		Version:      m.Version,
		Renderer:     renderer,
		Entry:        strings.TrimSpace(m.Preview.Entry),
		Extensions:   append([]string(nil), exts...),
		MimePrefixes: append([]string(nil), mimes...),
		SupportsZoom: m.SupportsZoom || m.Preview.SupportsZoom,
		Priority:     priority,
		Platforms:    append([]string(nil), platforms...),
		Enabled:      enabled,
	}
}

// BuildPreviewSyncBundle builds a PreviewSyncBundle from a list of manifests.
// Only enabled preview plugins that support the "mobile" platform are included.
func BuildPreviewSyncBundle(hostID, hostName string, manifests []Manifest, isEnabled func(id string) bool) PreviewSyncBundle {
	var out []PreviewPluginDescriptor
	for _, m := range manifests {
		if !IsPreviewPlugin(m) {
			continue
		}
		enabled := true
		if isEnabled != nil {
			enabled = isEnabled(m.ID)
		}
		if !enabled {
			continue
		}
		desc := ToPreviewDescriptor(m, enabled)
		if !slices.ContainsFunc(desc.Platforms, func(p string) bool {
			return strings.EqualFold(strings.TrimSpace(p), "mobile")
		}) {
			continue
		}
		out = append(out, desc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	raw, _ := json.Marshal(out)
	sum := sha256.Sum256(raw)
	return PreviewSyncBundle{
		HostID:      hostID,
		HostName:    hostName,
		Count:       len(out),
		Hash:        hex.EncodeToString(sum[:]),
		GeneratedAt: time.Now().UTC(),
		Plugins:     out,
	}
}
