package core

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// PluginManifest is the single canonical schema for plugin.json across both
// marketplace management (core.PluginManager) and runtime execution (plugin.Host).
type PluginManifest struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	MinAppVersion string              `json:"minAppVersion,omitempty"`
	Kind          string              `json:"kind,omitempty"`
	Native        string              `json:"native,omitempty"` // Deprecated: use Preview.Entry for WebView previews
	Preview       PreviewContribution `json:"preview,omitempty"`
	Main          string              `json:"main,omitempty"`
	Description   string              `json:"description,omitempty"`
	Author        string              `json:"author,omitempty"`
	Priority      int                 `json:"priority,omitempty"`
	Extensions    []string            `json:"extensions,omitempty"`
	MimePrefixes  []string            `json:"mimePrefixes,omitempty"`
	SupportsZoom  bool                `json:"supportsZoom,omitempty"`
	Capabilities  []string            `json:"capabilities,omitempty"`
	URLPatterns   []string            `json:"urlPatterns,omitempty"`
	URLHosts      []string            `json:"urlHosts,omitempty"`
	Permissions   PluginPermissions   `json:"permissions,omitempty"`
	UI            PluginUI            `json:"ui,omitempty"`
	Settings      []PluginSetting     `json:"settings,omitempty"`
	Hooks         []string            `json:"hooks,omitempty"`
	Icon          string              `json:"icon,omitempty"`
	Homepage      string              `json:"homepage,omitempty"`
	Repository    string              `json:"repository,omitempty"`
	License       string              `json:"license,omitempty"`
	DownloadURL   string              `json:"downloadUrl,omitempty"`
	Builtin       bool                `json:"builtin,omitempty"`
	Platforms     []string            `json:"platforms,omitempty"`
	Extra         map[string]any      `json:"-,"`
}

// PreviewContribution describes a plugin-provided preview view.
type PreviewContribution struct {
	Renderer     string   `json:"renderer,omitempty"`
	Entry        string   `json:"entry,omitempty"`
	Extensions   []string `json:"extensions,omitempty"`
	MimePrefixes []string `json:"mimePrefixes,omitempty"`
	SupportsZoom bool     `json:"supportsZoom,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Platforms    []string `json:"platforms,omitempty"`
}

// PluginUI describes WebView pages contributed by the plugin.
type PluginUI struct {
	Panels    []PluginUIEntry         `json:"panels,omitempty"`
	Windows   []PluginUIEntry         `json:"windows,omitempty"`
	Inspector []PluginInspectorAction `json:"inspector,omitempty"`
}

// PluginInspectorAction contributes a button for a selected asset that opens a plugin window.
type PluginInspectorAction struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Window string `json:"window"`
}

// PluginUIEntry describes a local HTML entry or external HTTPS window URL.
type PluginUIEntry struct {
	ID                 string `json:"id"`
	Title              string `json:"title,omitempty"`
	Location           string `json:"location,omitempty"`
	Entry              string `json:"entry,omitempty"`
	URL                string `json:"url,omitempty"`
	InterceptDownloads bool   `json:"interceptDownloads,omitempty"`
}

// PluginPermissions declares the capabilities a plugin may use.
type PluginPermissions struct {
	Network      []string `json:"network,omitempty"`
	Assets       []string `json:"assets,omitempty"`
	Process      []string `json:"process,omitempty"`
	OpenExternal bool     `json:"openExternal,omitempty"`
}

// PluginSettingOption is one selectable option for a "select" setting.
type PluginSettingOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// PluginSetting is one row the settings window renders.
type PluginSetting struct {
	Key         string                `json:"key"`
	Label       string                `json:"label"`
	Type        string                `json:"type,omitempty"` // "text" (default) | "select"
	Help        string                `json:"help,omitempty"`
	Placeholder string                `json:"placeholder,omitempty"`
	Default     string                `json:"default,omitempty"`
	Secret      bool                  `json:"secret,omitempty"`
	Options     []PluginSettingOption `json:"options,omitempty"`
	OptionsHook string                `json:"optionsHook,omitempty"`
}

// CleanRelativePluginPath validates that a plugin-relative file path stays inside the plugin directory.
func CleanRelativePluginPath(raw string) (string, error) {
	cleaned := filepath.Clean(raw)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "\\") || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("invalid file path %q: path traversal or absolute paths not allowed", raw)
	}
	return filepath.ToSlash(cleaned), nil
}

// NormalizePluginExtensions normalizes extension lists to lowercase without leading dots.
func NormalizePluginExtensions(list []string) []string {
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, ext := range list {
		cleaned := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
		if cleaned == "" || seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		out = append(out, cleaned)
	}
	return out
}

// ValidatePluginManifest validates and normalizes a PluginManifest against the canonical schema.
func ValidatePluginManifest(m *PluginManifest) error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	m.ID = strings.TrimSpace(m.ID)
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	if m.ID == "" || strings.Contains(m.ID, "..") || strings.ContainsAny(m.ID, `/\`) || !validPluginIDRegex.MatchString(m.ID) {
		return errors.New("invalid plugin id")
	}
	if m.Name == "" || m.Version == "" {
		return errors.New("plugin name and version are required")
	}
	if m.Main != "" {
		cleanMain, err := CleanRelativePluginPath(m.Main)
		if err != nil {
			return errors.New("plugin main must stay inside its directory")
		}
		m.Main = cleanMain
	}
	if m.Preview.Entry != "" {
		cleanEntry, err := CleanRelativePluginPath(m.Preview.Entry)
		if err != nil {
			return fmt.Errorf("plugin preview entry must stay inside its directory: %q", m.Preview.Entry)
		}
		m.Preview.Entry = cleanEntry
	}
	for idx, entries := range [][]PluginUIEntry{m.UI.Panels, m.UI.Windows} {
		isWindow := idx == 1
		seen := map[string]bool{}
		for i := range entries {
			entry := &entries[i]
			if entry.ID == "" || strings.ContainsAny(entry.ID, "/\\") || seen[entry.ID] {
				return fmt.Errorf("invalid or duplicate plugin UI id %q", entry.ID)
			}
			seen[entry.ID] = true
			if entry.URL != "" {
				if !isWindow {
					return fmt.Errorf("plugin UI panel %q cannot use an external url", entry.ID)
				}
				if entry.Entry != "" {
					return fmt.Errorf("plugin UI window %q cannot specify both entry and url", entry.ID)
				}
				parsed, err := url.Parse(entry.URL)
				if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
					return fmt.Errorf("plugin UI window url must be a valid https URL: %q", entry.URL)
				}
				continue
			}
			cleanEntry, err := CleanRelativePluginPath(entry.Entry)
			if err != nil || entry.Entry == "" {
				return fmt.Errorf("plugin UI entry must stay inside its directory: %q", entry.Entry)
			}
			entry.Entry = cleanEntry
		}
	}
	windowIDs := make(map[string]bool, len(m.UI.Windows))
	for _, window := range m.UI.Windows {
		windowIDs[window.ID] = true
	}
	actionIDs := map[string]bool{}
	for _, action := range m.UI.Inspector {
		if action.ID == "" || strings.ContainsAny(action.ID, "/\\") || actionIDs[action.ID] || strings.TrimSpace(action.Title) == "" || action.Type != "window" || !windowIDs[action.Window] {
			return fmt.Errorf("invalid inspector action %q", action.ID)
		}
		actionIDs[action.ID] = true
	}
	if m.Preview.Renderer != "" {
		if m.Native == "" {
			m.Native = strings.TrimSpace(m.Preview.Renderer)
		}
		if len(m.Extensions) == 0 {
			m.Extensions = m.Preview.Extensions
		}
		if len(m.MimePrefixes) == 0 {
			m.MimePrefixes = m.Preview.MimePrefixes
		}
		if !m.SupportsZoom {
			m.SupportsZoom = m.Preview.SupportsZoom
		}
		if m.Priority == 0 {
			m.Priority = m.Preview.Priority
		}
	}
	if m.Native != "" && strings.ContainsAny(m.Native, " /\\") {
		return errors.New("invalid native renderer id")
	}
	if m.Kind == "" {
		if m.Native != "" || m.Preview.Entry != "" {
			m.Kind = "preview"
		} else {
			m.Kind = "metadata"
		}
	} else {
		switch m.Kind {
		case "preview", "metadata", "resolver", "action":
		default:
			return fmt.Errorf("invalid kind %q: must be 'preview', 'metadata', 'resolver', or 'action'", m.Kind)
		}
	}
	m.Extensions = NormalizePluginExtensions(m.Extensions)
	allowedHooks := map[string]bool{"resolveUrl": true, "inspect": true, "actions": true, "runAction": true, "thumbnail": true, "settingOptions": true}
	seenHooks := map[string]bool{}
	normalizedHooks := make([]string, 0, len(m.Hooks))
	for _, raw := range m.Hooks {
		hook := strings.TrimSpace(raw)
		if hook == "" {
			continue
		}
		if !allowedHooks[hook] {
			return fmt.Errorf("unknown plugin hook %q", hook)
		}
		if seenHooks[hook] {
			continue
		}
		seenHooks[hook] = true
		normalizedHooks = append(normalizedHooks, hook)
	}
	m.Hooks = normalizedHooks
	if m.Main != "" && len(m.Hooks) == 0 {
		return errors.New("JavaScript plugins must declare hooks in plugin.json")
	}
	if m.Main == "" && len(m.Hooks) > 0 {
		return errors.New("native plugins cannot declare JavaScript hooks")
	}
	for _, permission := range m.Permissions.Assets {
		if permission != "read" && permission != "write" {
			return fmt.Errorf("unknown asset permission %q", permission)
		}
	}
	for _, host := range m.Permissions.Network {
		host = strings.TrimSpace(host)
		if host == "" {
			return errors.New("empty network permission")
		}
		if host == "*" || strings.Contains(host, "://") || strings.ContainsAny(host, "/\\") {
			return fmt.Errorf("network permission must be a hostname pattern: %q", host)
		}
	}
	for _, process := range m.Permissions.Process {
		process = strings.TrimSpace(process)
		if process == "" {
			return errors.New("empty process permission")
		}
		if process == "*" {
			return errors.New("process permission cannot be a bare wildcard")
		}
	}
	return nil
}
