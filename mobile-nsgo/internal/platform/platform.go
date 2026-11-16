// Package platform declares what the Android (and later iOS) host must do for
// the Go app. Files, sharing, clipboard, pickers and saving are core services;
// this package covers the gaps: encrypted credentials, mDNS discovery, the
// share-intake queue, QR scanning, file-URI clipboard, MediaStore export,
// legacy migration scanning and the device name.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/nsgo-dev/nsgo/core"
)

// Service is one discovered Bowerbird desktop.
type Service struct {
	Name            string   `json:"name"`
	Hosts           []string `json:"hosts"`
	Port            int      `json:"port"`
	Version         string   `json:"version,omitempty"`
	PairingRequired bool     `json:"pairingRequired"`
}

// Discovery finds desktops on the local network.
type Discovery interface {
	Browse(ctx context.Context, done func([]Service, error))
	Stop()
}

// SecureStore keeps the session token and history tokens encrypted by the host.
// Token values must never be written to ordinary JSON in a release build.
type SecureStore interface {
	Get(key string, done func(value string, err error))
	Set(key, value string, done func(err error))
	Delete(key string, done func(err error))
}

// SharedItem is one content item received from another app.
type SharedItem struct {
	Kind     string `json:"kind"` // "file" or "text"
	Name     string `json:"name"`
	Text     string `json:"text,omitempty"`
	Path     string `json:"path,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

// ShareEvent is one delivery from the Android share sheet.
type ShareEvent struct {
	ID         string       `json:"id"`
	Items      []SharedItem `json:"items"`
	ReceivedAt int64        `json:"receivedAt"`
}

// ShareIntake implements peek -> persist -> ack. The host must keep an event
// until the app acknowledges it, so a killed process can replay it without
// duplicating entries.
type ShareIntake interface {
	TakePending(done func([]ShareEvent, error))
	Ack(ids []string, done func(error))
}

// Scanner opens the host's QR scanner and returns the raw payload.
type Scanner interface {
	Scan(done func(payload string, cancelled bool, err error))
}

// ClipboardCopier puts a local file URI onto the system clipboard via FileProvider.
type ClipboardCopier interface {
	CopyFile(path, mimeType, displayName string, done func(error))
}

// MediaSaver saves a local file directly into the platform media library
// (MediaStore on Android) using streaming I/O.
type MediaSaver interface {
	SaveToGallery(path, name, mimeType string, done func(uri string, err error))
}

// LegacyPaths describes Flutter-era files discovered on the device.
type LegacyPaths struct {
	FlutterDir          string
	FilesDir            string
	CacheDir            string
	LegacyInboxPath     string
	LegacyShareTagsPath string
	HasLegacySecure     bool
}

// LegacyMigrator inspects old Flutter storage locations on the host.
type LegacyMigrator interface {
	ScanLegacy(done func(LegacyPaths, error))
}

// Device supplies the name shown on the desktop's paired-device list.
type Device struct {
	Name string
}

// Env bundles the host capabilities. Nil fields degrade to explicit "not
// available" behavior rather than panics.
type Env struct {
	Discovery Discovery
	Secure    SecureStore
	Share     ShareIntake
	Scanner   Scanner
	Clipboard ClipboardCopier
	Media     MediaSaver
	Migrator  LegacyMigrator
	Device    Device
	// Accent is the system accent colour as #rrggbb, when the host provides one.
	Accent   string
	HasHost  bool
	DataDir  string
	CacheDir string
}

// ErrUnavailable is returned when the host has no implementation.
var ErrUnavailable = errors.New("platform capability unavailable")

// NoopDiscovery is used on desktops and in tests without a host.
type NoopDiscovery struct{}

func (NoopDiscovery) Browse(context.Context, func([]Service, error)) {}
func (NoopDiscovery) Stop()                                          {}

// MemorySecureStore is for tests and desktop development only.
type MemorySecureStore struct {
	values map[string]string
}

func NewMemorySecureStore() *MemorySecureStore {
	return &MemorySecureStore{values: map[string]string{}}
}

func (m *MemorySecureStore) Get(key string, done func(string, error)) {
	done(m.values[key], nil)
}
func (m *MemorySecureStore) Set(key, value string, done func(error)) {
	m.values[key] = value
	done(nil)
}
func (m *MemorySecureStore) Delete(key string, done func(error)) {
	delete(m.values, key)
	done(nil)
}

// HostEnv reads the host parameters and creates host-backed capabilities.
// Call it while building the root component, before any handler runs.
func HostEnv(cx *core.Scope) Env {
	env := Env{DataDir: cx.DataDir(), CacheDir: cx.CacheDir(), HasHost: cx.HasHost()}
	if name := stringParam(cx.HostParams(), "deviceName"); name != "" {
		env.Device.Name = name
	}
	env.Accent = stringParam(cx.HostParams(), "systemAccent")
	if env.HasHost {
		env.Discovery = &hostDiscovery{cx: cx}
		env.Secure = &hostSecure{cx: cx}
		env.Share = &hostShare{cx: cx}
		env.Scanner = &hostScanner{cx: cx}
		env.Clipboard = &hostClipboard{cx: cx}
		env.Media = &hostMedia{cx: cx}
		env.Migrator = &hostMigrator{cx: cx}
	}
	return env
}

type hostDiscovery struct{ cx *core.Scope }

func (h *hostDiscovery) Browse(ctx context.Context, done func([]Service, error)) {
	h.cx.CallHost("discovery.browse", nil, func(result any, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		done(decodeServices(result), nil)
	})
}

func (h *hostDiscovery) Stop() { h.cx.PostHost("discovery.stop", nil) }

func decodeServices(result any) []Service {
	list, _ := result.([]any)
	out := make([]Service, 0, len(list))
	for _, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		s := Service{
			Name:            stringParam(m, "name"),
			Port:            intParam(m, "port"),
			Version:         stringParam(m, "version"),
			PairingRequired: boolParam(m, "pairingRequired"),
		}
		if hosts, ok := m["hosts"].([]any); ok {
			for _, host := range hosts {
				if text, ok := host.(string); ok && text != "" {
					s.Hosts = append(s.Hosts, text)
				}
			}
		}
		if s.Name != "" && s.Port > 0 {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type hostSecure struct{ cx *core.Scope }

func (h *hostSecure) Get(key string, done func(string, error)) {
	h.cx.CallHost("secure.get", map[string]any{"key": key}, func(result any, err error) {
		if err != nil {
			done("", err)
			return
		}
		m, _ := result.(map[string]any)
		done(stringParam(m, "value"), nil)
	})
}

func (h *hostSecure) Set(key, value string, done func(error)) {
	h.cx.CallHost("secure.set", map[string]any{"key": key, "value": value}, func(_ any, err error) { done(err) })
}

func (h *hostSecure) Delete(key string, done func(error)) {
	h.cx.CallHost("secure.delete", map[string]any{"key": key}, func(_ any, err error) { done(err) })
}

type hostShare struct{ cx *core.Scope }

func (h *hostShare) TakePending(done func([]ShareEvent, error)) {
	h.cx.CallHost("share.takePending", nil, func(result any, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		done(decodeShareEvents(result), nil)
	})
}

func (h *hostShare) Ack(ids []string, done func(error)) {
	h.cx.CallHost("share.ack", map[string]any{"ids": ids}, func(_ any, err error) { done(err) })
}

type hostScanner struct{ cx *core.Scope }

func (h *hostScanner) Scan(done func(string, bool, error)) {
	h.cx.CallHost("scanner.scan", nil, func(result any, err error) {
		if err != nil {
			done("", false, err)
			return
		}
		m, _ := result.(map[string]any)
		done(stringParam(m, "payload"), boolParam(m, "cancelled"), nil)
	})
}

type hostClipboard struct{ cx *core.Scope }

func (h *hostClipboard) CopyFile(path, mimeType, displayName string, done func(error)) {
	h.cx.CallHost("clipboard.copyFile", map[string]any{
		"path":        path,
		"mimeType":    mimeType,
		"displayName": displayName,
	}, func(_ any, err error) { done(err) })
}

type hostMedia struct{ cx *core.Scope }

func (h *hostMedia) SaveToGallery(path, name, mimeType string, done func(string, error)) {
	h.cx.CallHost("media.saveToGallery", map[string]any{
		"path":     path,
		"name":     name,
		"mimeType": mimeType,
	}, func(result any, err error) {
		if err != nil {
			done("", err)
			return
		}
		m, _ := result.(map[string]any)
		done(stringParam(m, "uri"), nil)
	})
}

type hostMigrator struct{ cx *core.Scope }

func (h *hostMigrator) ScanLegacy(done func(LegacyPaths, error)) {
	h.cx.CallHost("migration.scanLegacy", nil, func(result any, err error) {
		if err != nil {
			done(LegacyPaths{}, err)
			return
		}
		m, _ := result.(map[string]any)
		done(LegacyPaths{
			FlutterDir:          stringParam(m, "flutterDir"),
			FilesDir:            stringParam(m, "filesDir"),
			CacheDir:            stringParam(m, "cacheDir"),
			LegacyInboxPath:     stringParam(m, "legacyInboxPath"),
			LegacyShareTagsPath: stringParam(m, "legacyShareTagsPath"),
			HasLegacySecure:     boolParam(m, "hasLegacySecure"),
		}, nil)
	})
}

func decodeShareEvents(result any) []ShareEvent {
	list, _ := result.([]any)
	out := make([]ShareEvent, 0, len(list))
	for _, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := stringParam(m, "id")
		if id == "" {
			id = stringParam(m, "eventId")
		}
		ev := ShareEvent{ID: id, ReceivedAt: int64(intParam(m, "receivedAt"))}
		items, _ := m["items"].([]any)
		for _, rawItem := range items {
			im, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			ev.Items = append(ev.Items, SharedItem{
				Kind:     stringParam(im, "kind"),
				Name:     stringParam(im, "name"),
				Text:     stringParam(im, "text"),
				Path:     stringParam(im, "path"),
				MimeType: stringParam(im, "mimeType"),
				Size:     int64(intParam(im, "size")),
			})
		}
		if ev.ID != "" && len(ev.Items) > 0 {
			out = append(out, ev)
		}
	}
	return out
}

func stringParam(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intParam(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		var i int
		_, _ = fmt.Sscanf(v, "%d", &i)
		return i
	}
	return 0
}

func boolParam(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}
