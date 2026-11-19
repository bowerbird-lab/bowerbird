package storage

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// File modes: local documents hold private paths and metadata.
const (
	privateFile = 0o600
	publicFile  = 0o644
)

// Session is the active connection. Token is intentionally omitted from JSON
// and persisted only through platform.SecureStore.
type Session struct {
	Version      int      `json:"version"`
	DeviceID     string   `json:"deviceId,omitempty"`
	BaseURL      string   `json:"baseUrl"`
	Token        string   `json:"-"`
	CandidateIPs []string `json:"candidateIps,omitempty"`
	Port         int      `json:"port,omitempty"`
	DeviceName   string   `json:"deviceName"`
	LibraryID    string   `json:"libraryId,omitempty"`
	LibraryName  string   `json:"libraryName,omitempty"`
	ConnectedAt  int64    `json:"connectedAt"`
}

// Connection is one remembered desktop. Token is stored in SecureStore.
type Connection struct {
	DeviceID        string   `json:"deviceId,omitempty"`
	Name            string   `json:"name,omitempty"`
	BaseURL         string   `json:"baseUrl"`
	Token           string   `json:"-"`
	CandidateIPs    []string `json:"candidateIps,omitempty"`
	Port            int      `json:"port,omitempty"`
	LibraryID       string   `json:"libraryId,omitempty"`
	LibraryName     string   `json:"libraryName,omitempty"`
	LastConnectedAt int64    `json:"lastConnectedAt"`
}

// Connections is the capped connection history.
type Connections struct {
	Version int          `json:"version"`
	Items   []Connection `json:"items"`
}

// UsageHistory keeps recently used item IDs per stable library identity. The
// old app had one unscoped list of at most 500 IDs; that list stays under
// Legacy until a migration can map it to a library.
type UsageHistory struct {
	Version int                 `json:"version"`
	Scopes  map[string][]string `json:"scopes"`
	Legacy  []string            `json:"legacy,omitempty"`
}

// InboxEntry is one pending share: a file already copied into app storage or
// text saved inline.
type InboxEntry struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"` // "file" or "text"
	Name       string `json:"name"`
	Text       string `json:"text,omitempty"`
	StoredPath string `json:"storedPath,omitempty"` // relative to DataDir
	MimeType   string `json:"mimeType,omitempty"`
	Size       int64  `json:"size,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
	Attempts   int    `json:"attempts,omitempty"`
	LastError  string `json:"lastError,omitempty"`
	Source     string `json:"source,omitempty"` // "share", "picker", "clipboard"
	EventID    string `json:"eventId,omitempty"`
}

// InboxIndex is share_inbox/index.json, version 1 in the old app.
type InboxIndex struct {
	Version         int          `json:"version"`
	Entries         []InboxEntry `json:"entries"`
	ProcessedEvents []string     `json:"processedEvents"`
	UpdatedAt       int64        `json:"updatedAt"`
}

// ShareTag marks an item the phone uploaded. Scope is a stable library ID when
// one exists, else the legacy library name, never mixed with a stable ID.
type ShareTag struct {
	ItemID    string `json:"itemId"`
	Hex       string `json:"hex,omitempty"`
	Scope     string `json:"scope"`
	CreatedAt int64  `json:"createdAt"`
}

// ShareTags is share_tags.json.
type ShareTags struct {
	Version int        `json:"version"`
	Tags    []ShareTag `json:"tags"`
}

// MigrationState records completed, idempotent migration steps.
type MigrationState struct {
	Version   int               `json:"version"`
	Completed map[string]string `json:"completed"` // step -> ISO time or note
	UpdatedAt int64             `json:"updatedAt"`
}

// Constants.
const (
	SessionVersion     = 1
	ConnectionsVersion = 1
	UsageVersion       = 1
	InboxVersion       = 1
	ShareTagsVersion   = 1
	SaveHistoryVersion = 1
	MigrationVersion   = 1

	MaxConnections = 10
	MaxUsage       = 500
	MaxShareTags   = 2000
	MaxSaveHistory = 300
	MaxProcessed   = 300
)

// SaveHistory keeps recently saved item IDs from mobile shares per stable
// library scope, newest first, capped at 300 IDs.
type SaveHistory struct {
	Version int                 `json:"version"`
	Scopes  map[string][]string `json:"scopes"`
}

// Local wraps a Store with the documented document names and their limits.
type Local struct {
	store *Store
}

// NewLocal opens the app's data directory.
func NewLocal(dir string) *Local { return &Local{store: New(dir)} }

// Store returns the underlying document store.
func (l *Local) Store() *Store { return l.store }

// LoadSession returns the saved session, or nil. If a legacy dev JSON file
// still contains a plaintext token field, it is read into Token so the caller
// can migrate it to SecureStore and re-save without it.
func (l *Local) LoadSession() (*Session, error) {
	var raw struct {
		Version      int      `json:"version"`
		DeviceID     string   `json:"deviceId"`
		BaseURL      string   `json:"baseUrl"`
		Token        string   `json:"token"`
		CandidateIPs []string `json:"candidateIps"`
		Port         int      `json:"port"`
		DeviceName   string   `json:"deviceName"`
		LibraryID    string   `json:"libraryId"`
		LibraryName  string   `json:"libraryName"`
		ConnectedAt  int64    `json:"connectedAt"`
	}
	found, err := l.store.Load("session.json", &raw)
	if !found || err != nil {
		return nil, err
	}
	if raw.BaseURL == "" {
		return nil, nil
	}
	return &Session{
		Version:      raw.Version,
		DeviceID:     raw.DeviceID,
		BaseURL:      raw.BaseURL,
		Token:        raw.Token,
		CandidateIPs: raw.CandidateIPs,
		Port:         raw.Port,
		DeviceName:   raw.DeviceName,
		LibraryID:    raw.LibraryID,
		LibraryName:  raw.LibraryName,
		ConnectedAt:  raw.ConnectedAt,
	}, nil
}

// SaveSession writes the active non-secret connection metadata and remembers it
// in history. Tokens are not written to JSON.
func (l *Local) SaveSession(s Session) error {
	if s.Version == 0 {
		s.Version = SessionVersion
	}
	if s.ConnectedAt == 0 {
		s.ConnectedAt = time.Now().UnixMilli()
	}
	if err := l.store.Save("session.json", s, privateFile); err != nil {
		return err
	}
	return l.Remember(Connection{
		DeviceID:        s.DeviceID,
		BaseURL:         s.BaseURL,
		Token:           s.Token,
		CandidateIPs:    s.CandidateIPs,
		Port:            s.Port,
		LibraryID:       s.LibraryID,
		LibraryName:     s.LibraryName,
		LastConnectedAt: s.ConnectedAt,
	})
}

// ClearSession removes the active connection but keeps the history.
func (l *Local) ClearSession() error {
	path, err := l.store.Path("session.json")
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// History returns remembered connections, newest first. Any legacy plaintext
// token present in an older connections.json is surfaced so callers can migrate
// it to SecureStore.
func (l *Local) History() ([]Connection, error) {
	var raw struct {
		Version int `json:"version"`
		Items   []struct {
			DeviceID        string   `json:"deviceId,omitempty"`
			Name            string   `json:"name,omitempty"`
			BaseURL         string   `json:"baseUrl"`
			Token           string   `json:"token"`
			CandidateIPs    []string `json:"candidateIps,omitempty"`
			Port            int      `json:"port,omitempty"`
			LibraryID       string   `json:"libraryId,omitempty"`
			LibraryName     string   `json:"libraryName,omitempty"`
			LastConnectedAt int64    `json:"lastConnectedAt"`
		} `json:"items"`
	}
	if _, err := l.store.Load("connections.json", &raw); err != nil {
		return nil, err
	}
	items := make([]Connection, 0, len(raw.Items))
	for _, item := range raw.Items {
		items = append(items, Connection{
			DeviceID:        item.DeviceID,
			Name:            item.Name,
			BaseURL:         item.BaseURL,
			Token:           item.Token,
			CandidateIPs:    item.CandidateIPs,
			Port:            item.Port,
			LibraryID:       item.LibraryID,
			LibraryName:     item.LibraryName,
			LastConnectedAt: item.LastConnectedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].LastConnectedAt > items[j].LastConnectedAt })
	if len(items) > MaxConnections {
		items = items[:MaxConnections]
	}
	return items, nil
}

// Remember inserts or refreshes a connection, merging multiple IPs belonging
// to the same desktop device into one entry, newest first, capped at 10.
func (l *Local) Remember(c Connection) error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil
	}
	if c.LastConnectedAt == 0 {
		c.LastConnectedAt = time.Now().UnixMilli()
	}
	if host, port := HostPortFromURL(c.BaseURL); host != "" {
		if c.Port <= 0 && port > 0 {
			c.Port = port
		}
		c.CandidateIPs = MergeIPs(c.CandidateIPs, []string{host})
	} else {
		c.CandidateIPs = MergeIPs(c.CandidateIPs)
	}

	existing, err := l.History()
	if err != nil {
		return err
	}
	var rest []Connection
	for _, old := range existing {
		if sameDevice(old, c) {
			if strings.TrimSpace(c.Name) == "" {
				c.Name = old.Name
			}
			if strings.TrimSpace(c.DeviceID) == "" {
				c.DeviceID = old.DeviceID
			}
			if strings.TrimSpace(c.LibraryID) == "" {
				c.LibraryID = old.LibraryID
			}
			if strings.TrimSpace(c.LibraryName) == "" {
				c.LibraryName = old.LibraryName
			}
			if c.Port <= 0 {
				c.Port = old.Port
			}
			c.CandidateIPs = MergeIPs(c.CandidateIPs, old.CandidateIPs)
			if oldHost, _ := HostPortFromURL(old.BaseURL); oldHost != "" {
				c.CandidateIPs = MergeIPs(c.CandidateIPs, []string{oldHost})
			}
			continue
		}
		rest = append(rest, old)
	}
	out := append([]Connection{c}, rest...)
	if len(out) > MaxConnections {
		out = out[:MaxConnections]
	}
	return l.store.Save("connections.json", Connections{Version: ConnectionsVersion, Items: out}, privateFile)
}

// Forget removes one device from history by its baseURL or deviceID.
func (l *Local) Forget(key string) error {
	existing, err := l.History()
	if err != nil {
		return err
	}
	out := make([]Connection, 0, len(existing))
	for _, c := range existing {
		if matchesConnectionKey(c, key) {
			continue
		}
		out = append(out, c)
	}
	return l.store.Save("connections.json", Connections{Version: ConnectionsVersion, Items: out}, privateFile)
}

func matchesConnectionKey(c Connection, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if c.DeviceID != "" && c.DeviceID == key {
		return true
	}
	if sameOrigin(c.BaseURL, key) {
		return true
	}
	return false
}

func sameDevice(a, b Connection) bool {
	if a.DeviceID != "" && b.DeviceID != "" {
		return a.DeviceID == b.DeviceID
	}
	if sameOrigin(a.BaseURL, b.BaseURL) {
		return true
	}
	// If either side lacks DeviceID (e.g., legacy saved connection), match by overlapping host:port.
	aEndpoints := connectionEndpointSet(a)
	for ep := range connectionEndpointSet(b) {
		if aEndpoints[ep] {
			return true
		}
	}
	return false
}

func connectionEndpointSet(c Connection) map[string]bool {
	set := make(map[string]bool)
	port := c.Port
	if host, p := HostPortFromURL(c.BaseURL); host != "" {
		if port <= 0 {
			port = p
		}
		if p <= 0 {
			p = port
		}
		set[strings.ToLower(net.JoinHostPort(host, strconv.Itoa(p)))] = true
	}
	if port <= 0 {
		port = 17878
	}
	for _, raw := range c.CandidateIPs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if h, p := HostPortFromURL(raw); h != "" {
			if p <= 0 {
				p = port
			}
			set[strings.ToLower(net.JoinHostPort(h, strconv.Itoa(p)))] = true
			continue
		}
		set[strings.ToLower(net.JoinHostPort(strings.Trim(raw, "[]"), strconv.Itoa(port)))] = true
	}
	return set
}

// HostPortFromURL extracts the host and optional port from a URL or host:port string.
func HostPortFromURL(raw string) (string, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", 0
		}
		h := u.Hostname()
		p, _ := strconv.Atoi(u.Port())
		return h, p
	}
	if h, pStr, err := net.SplitHostPort(raw); err == nil {
		p, _ := strconv.Atoi(pStr)
		return h, p
	}
	return "", 0
}

// MergeIPs normalizes and deduplicates IP/host slices while preserving order.
func MergeIPs(groups ...[]string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, g := range groups {
		for _, raw := range g {
			ip := strings.TrimSpace(raw)
			if ip == "" {
				continue
			}
			if h, _ := HostPortFromURL(ip); h != "" {
				ip = h
			} else {
				ip = strings.Trim(ip, "[]")
			}
			key := strings.ToLower(ip)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ip)
		}
	}
	return out
}

func sameOrigin(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

// LoadUsage returns the usage document, creating an empty one if missing.
func (l *Local) LoadUsage() (UsageHistory, error) {
	var u UsageHistory
	if _, err := l.store.Load("usage.json", &u); err != nil {
		return u, err
	}
	if u.Scopes == nil {
		u.Scopes = map[string][]string{}
	}
	if len(u.Legacy) > MaxUsage {
		u.Legacy = u.Legacy[:MaxUsage]
	}
	return u, nil
}

// UsageIDs returns the ordered item IDs for a scope (plus legacy unscoped IDs
// if any, deduplicated).
func (l *Local) UsageIDs(scope string) ([]string, error) {
	u, err := l.LoadUsage()
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	appendUnique := func(list []string) {
		for _, id := range list {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	if scope != "" {
		appendUnique(u.Scopes[scope])
	}
	appendUnique(u.Legacy)
	return out, nil
}

// AddUsage puts id first in a library scope and trims it to 500 IDs.
func (l *Local) AddUsage(scope, id string) error {
	if strings.TrimSpace(scope) == "" || strings.TrimSpace(id) == "" {
		return nil
	}
	u, err := l.LoadUsage()
	if err != nil {
		return err
	}
	next := []string{id}
	for _, old := range u.Scopes[scope] {
		if old != id {
			next = append(next, old)
		}
	}
	if len(next) > MaxUsage {
		next = next[:MaxUsage]
	}
	u.Scopes[scope] = next
	u.Version = UsageVersion
	return l.store.Save("usage.json", u, privateFile)
}

// LoadInbox returns the inbox index, empty when missing.
func (l *Local) LoadInbox() (InboxIndex, error) {
	var idx InboxIndex
	if _, err := l.store.Load("share_inbox/index.json", &idx); err != nil {
		return idx, err
	}
	if idx.Entries == nil {
		idx.Entries = []InboxEntry{}
	}
	if idx.ProcessedEvents == nil {
		idx.ProcessedEvents = []string{}
	}
	idx.Version = InboxVersion
	return idx, nil
}

// SaveInbox writes the inbox index and trims processed event IDs to 300.
func (l *Local) SaveInbox(idx InboxIndex) error {
	if idx.Entries == nil {
		idx.Entries = []InboxEntry{}
	}
	if len(idx.ProcessedEvents) > MaxProcessed {
		idx.ProcessedEvents = idx.ProcessedEvents[len(idx.ProcessedEvents)-MaxProcessed:]
	}
	idx.Version = InboxVersion
	idx.UpdatedAt = time.Now().UnixMilli()
	return l.store.Save("share_inbox/index.json", idx, privateFile)
}

// LoadShareTags returns the phone-share marks.
func (l *Local) LoadShareTags() (ShareTags, error) {
	var t ShareTags
	if _, err := l.store.Load("share_inbox/share_tags.json", &t); err != nil {
		return t, err
	}
	if t.Tags == nil {
		t.Tags = []ShareTag{}
	}
	t.Version = ShareTagsVersion
	return t, nil
}

// SaveShareTags writes the marks and caps them at 2,000.
func (l *Local) SaveShareTags(t ShareTags) error {
	if t.Tags == nil {
		t.Tags = []ShareTag{}
	}
	if len(t.Tags) > MaxShareTags {
		t.Tags = t.Tags[:MaxShareTags]
	}
	t.Version = ShareTagsVersion
	return l.store.Save("share_inbox/share_tags.json", t, privateFile)
}

// RemoveShareTag deletes marks matching itemID within scope (or any scope if empty).
func (l *Local) RemoveShareTag(scope, itemID string) error {
	id := strings.TrimSpace(itemID)
	if id == "" {
		return nil
	}
	tags, err := l.LoadShareTags()
	if err != nil {
		return err
	}
	out := make([]ShareTag, 0, len(tags.Tags))
	changed := false
	for _, t := range tags.Tags {
		sameID := strings.EqualFold(t.ItemID, id) || strings.EqualFold(t.Hex, id)
		sameScope := scope == "" || t.Scope == scope
		if sameID && sameScope {
			changed = true
			continue
		}
		out = append(out, t)
	}
	if !changed {
		return nil
	}
	tags.Tags = out
	return l.SaveShareTags(tags)
}

// ShareTagSets builds fast lookup sets of item IDs and uppercase SHA-256 hexes
// marked as shared from the phone. If fallbackLegacyScope is non-empty, legacy
// name-scoped tags matching it are also included without mixing scopes on write.
func (l *Local) ShareTagSets(scope, fallbackLegacyScope string) (ids map[string]bool, hexes map[string]bool) {
	ids = map[string]bool{}
	hexes = map[string]bool{}
	tags, err := l.LoadShareTags()
	if err != nil {
		return ids, hexes
	}
	for _, t := range tags.Tags {
		if t.Scope != scope && (fallbackLegacyScope == "" || t.Scope != fallbackLegacyScope) {
			continue
		}
		if t.ItemID != "" {
			ids[strings.ToUpper(t.ItemID)] = true
			ids[t.ItemID] = true
		}
		if t.Hex != "" {
			hexes[strings.ToUpper(t.Hex)] = true
		}
	}
	return ids, hexes
}

// MigrateLegacyShareTags imports Flutter-era share_tags.json ({version, marks:[{id,library,sharedAt}]})
// into the NSGo store without overwriting or deleting the source file.
func (l *Local) MigrateLegacyShareTags(legacyPath string) (int, error) {
	if strings.TrimSpace(legacyPath) == "" {
		return 0, nil
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var doc struct {
		Version int `json:"version"`
		Marks   []struct {
			ID       string `json:"id"`
			Library  string `json:"library"`
			SharedAt int64  `json:"sharedAt"`
		} `json:"marks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, err
	}
	if len(doc.Marks) == 0 {
		return 0, nil
	}
	existing, err := l.LoadShareTags()
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(existing.Tags))
	for _, t := range existing.Tags {
		seen[t.Scope+"\x00"+strings.ToUpper(t.ItemID)] = true
	}
	added := 0
	for _, m := range doc.Marks {
		id := strings.ToUpper(strings.TrimSpace(m.ID))
		lib := strings.TrimSpace(m.Library)
		if id == "" || lib == "" {
			continue
		}
		scope := "legacy:name:" + lib
		key := scope + "\x00" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		existing.Tags = append(existing.Tags, ShareTag{
			ItemID:    id,
			Hex:       id,
			Scope:     scope,
			CreatedAt: m.SharedAt,
		})
		added++
	}
	if added == 0 {
		return 0, nil
	}
	return added, l.SaveShareTags(existing)
}

// MigrateLegacyInbox imports Flutter-era share_inbox/index.json entries and
// copies their referenced files into NSGo's DataDir/share_inbox/media/ without
// deleting the legacy source files.
func (l *Local) MigrateLegacyInbox(legacyIndexPath string) (int, error) {
	if strings.TrimSpace(legacyIndexPath) == "" {
		return 0, nil
	}
	data, err := os.ReadFile(legacyIndexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var doc struct {
		Version int `json:"version"`
		Entries []struct {
			ID         string `json:"id"`
			Kind       string `json:"kind"`
			FileName   string `json:"fileName"`
			Name       string `json:"name"`
			MimeType   string `json:"mimeType"`
			Size       int64  `json:"size"`
			ReceivedAt int64  `json:"receivedAt"`
			CreatedAt  int64  `json:"createdAt"`
			StoredPath string `json:"storedPath"`
			Text       string `json:"text"`
		} `json:"entries"`
		ProcessedEvents []string `json:"processedEvents"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, err
	}
	idx, err := l.LoadInbox()
	if err != nil {
		return 0, err
	}
	knownEntry := make(map[string]bool, len(idx.Entries))
	for _, e := range idx.Entries {
		knownEntry[e.ID] = true
	}
	knownProc := make(map[string]bool, len(idx.ProcessedEvents))
	for _, id := range idx.ProcessedEvents {
		knownProc[id] = true
	}
	for _, id := range doc.ProcessedEvents {
		if id != "" && !knownProc[id] {
			knownProc[id] = true
			idx.ProcessedEvents = append(idx.ProcessedEvents, id)
		}
	}

	legacyRoot := filepath.Dir(legacyIndexPath)
	added := 0
	for _, raw := range doc.Entries {
		if raw.ID == "" || knownEntry[raw.ID] {
			continue
		}
		name := strings.TrimSpace(raw.FileName)
		if name == "" {
			name = strings.TrimSpace(raw.Name)
		}
		created := raw.ReceivedAt
		if created == 0 {
			created = raw.CreatedAt
		}
		if created == 0 {
			created = time.Now().UnixMilli()
		}
		entry := InboxEntry{
			ID:        raw.ID,
			Kind:      raw.Kind,
			Name:      name,
			Text:      raw.Text,
			MimeType:  raw.MimeType,
			Size:      raw.Size,
			CreatedAt: created,
			Source:    "share",
		}
		if raw.Kind != "text" {
			entry.Kind = "file"
			relOld := filepath.Clean(raw.StoredPath)
			if relOld == "." || relOld == "" || filepath.IsAbs(relOld) || strings.HasPrefix(relOld, "..") {
				continue
			}
			srcPath := filepath.Join(legacyRoot, relOld)
			src, openErr := os.Open(srcPath)
			if openErr != nil {
				// Also check if already relative to DataDir
				if altPath, pathErr := l.store.Path(relOld); pathErr == nil {
					src, openErr = os.Open(altPath)
				}
			}
			if openErr != nil {
				continue
			}
			newRel := filepath.Join("share_inbox", "media", "legacy-"+raw.ID, filepath.Base(srcPath))
			size, writeErr := l.store.WriteReader(newRel, src, privateFile)
			_ = src.Close()
			if writeErr != nil {
				continue
			}
			entry.StoredPath = filepath.ToSlash(newRel)
			if entry.Size == 0 {
				entry.Size = size
			}
		}
		idx.Entries = append(idx.Entries, entry)
		knownEntry[entry.ID] = true
		added++
	}
	if err := l.SaveInbox(idx); err != nil {
		return added, err
	}
	return added, nil
}

// LoadMigration returns the migration state.
func (l *Local) LoadMigration() (MigrationState, error) {
	var m MigrationState
	_, err := l.store.Load("migration.json", &m)
	if m.Completed == nil {
		m.Completed = map[string]string{}
	}
	if m.Version == 0 {
		m.Version = MigrationVersion
	}
	return m, err
}

// MarkMigration runs a step once. It returns whether the step ran now.
func (l *Local) MarkMigration(step string, run func() error) (bool, error) {
	m, err := l.LoadMigration()
	if err != nil {
		return false, err
	}
	if _, done := m.Completed[step]; done {
		return false, nil
	}
	if err := run(); err != nil {
		return false, err
	}
	m.Completed[step] = time.Now().UTC().Format(time.RFC3339)
	m.Version = MigrationVersion
	m.UpdatedAt = time.Now().UnixMilli()
	return true, l.store.Save("migration.json", m, privateFile)
}

// Gallery layout modes.
const (
	LayoutAuto      = "auto"
	LayoutWaterfall = "waterfall"
	LayoutGrid      = "grid"

	ShareModeAuto   = "auto"
	ShareModeChoose = "choose"
)

// Preferences is the app's local appearance and share settings.
type Preferences struct {
	Version             int    `json:"version"`
	ThemeMode           string `json:"themeMode"` // "system", "light" or "dark"
	LightTheme          string `json:"lightTheme"`
	DarkTheme           string `json:"darkTheme"`
	SystemAccent        bool   `json:"systemAccent"`
	GridColumns         int    `json:"gridColumns"`
	GalleryLayout       string `json:"galleryLayout"` // "auto", "waterfall" or "grid"
	ShareMode              string `json:"shareMode"`     // "auto" or "choose"
	ShareNotifyDisabled    bool   `json:"shareNotifyDisabled,omitempty"`
	PredictiveBackDisabled bool   `json:"predictiveBackDisabled,omitempty"`
}

// PredictiveBackEnabled reports whether Android predictive back is enabled.
func (p Preferences) PredictiveBackEnabled() bool {
	return !p.PredictiveBackDisabled
}

// PreferencesVersion is the current preferences document version.
const PreferencesVersion = 3

// DefaultPreferences are the values a fresh install and a corrupt document
// fall back to.
func DefaultPreferences() Preferences {
	return Preferences{
		Version:       PreferencesVersion,
		ThemeMode:     "system",
		LightTheme:    "default-light",
		DarkTheme:     "default-dark",
		SystemAccent:  true,
		GridColumns:   3,
		GalleryLayout: LayoutAuto,
		ShareMode:     ShareModeAuto,
	}
}

// ClampGridColumns bounds cols to the range supported by layout (2..3 for
// waterfall, 2..5 for auto and grid).
func ClampGridColumns(layout string, cols int) int {
	maxCols := 5
	if layout == LayoutWaterfall {
		maxCols = 3
	}
	if cols < 2 {
		return 2
	}
	if cols > maxCols {
		return maxCols
	}
	return cols
}

// IsWaterfallLayout reports whether the given layout mode and column count
// render as a waterfall (masonry) layout rather than a square grid.
func IsWaterfallLayout(layout string, cols int) bool {
	switch layout {
	case LayoutWaterfall:
		return true
	case LayoutGrid:
		return false
	default:
		return ClampGridColumns(LayoutAuto, cols) <= 3
	}
}

// LoadPreferences returns the saved preferences, repaired to the defaults when
// the file is missing or carries a value the app cannot use.
func (l *Local) LoadPreferences() (Preferences, error) {
	prefs := DefaultPreferences()
	found, err := l.store.Load("preferences.json", &prefs)
	if !found || err != nil {
		return DefaultPreferences(), err
	}
	return prefs.Normalize(), nil
}

// SavePreferences writes the appearance settings atomically.
func (l *Local) SavePreferences(prefs Preferences) error {
	prefs = prefs.Normalize()
	return l.store.Save("preferences.json", prefs, privateFile)
}

// Normalize fills missing values and rejects an unknown colour mode.
func (p Preferences) Normalize() Preferences {
	def := DefaultPreferences()
	p.Version = PreferencesVersion
	switch p.ThemeMode {
	case "system", "light", "dark":
	default:
		p.ThemeMode = def.ThemeMode
	}
	if strings.TrimSpace(p.LightTheme) == "" {
		p.LightTheme = def.LightTheme
	}
	if strings.TrimSpace(p.DarkTheme) == "" {
		p.DarkTheme = def.DarkTheme
	}
	switch p.GalleryLayout {
	case LayoutAuto, LayoutWaterfall, LayoutGrid:
	default:
		p.GalleryLayout = def.GalleryLayout
	}
	if p.GridColumns < 2 || p.GridColumns > 5 {
		p.GridColumns = def.GridColumns
	}
	p.GridColumns = ClampGridColumns(p.GalleryLayout, p.GridColumns)
	switch p.ShareMode {
	case ShareModeAuto, ShareModeChoose:
	default:
		p.ShareMode = def.ShareMode
	}
	return p
}

// LoadSaveHistory returns the local save-history document.
func (l *Local) LoadSaveHistory() (SaveHistory, error) {
	out := SaveHistory{Version: SaveHistoryVersion, Scopes: map[string][]string{}}
	if _, err := l.store.Load("save_history.json", &out); err != nil {
		return out, err
	}
	if out.Version == 0 {
		out.Version = SaveHistoryVersion
	}
	if out.Scopes == nil {
		out.Scopes = map[string][]string{}
	}
	return out, nil
}

// SaveHistoryIDs returns the saved item IDs for scope, newest first.
func (l *Local) SaveHistoryIDs(scope string) ([]string, error) {
	if scope == "" {
		return nil, nil
	}
	h, err := l.LoadSaveHistory()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), h.Scopes[scope]...), nil
}

// AddSaveHistory prepends item IDs for scope, deduplicates, and caps at 300.
func (l *Local) AddSaveHistory(scope string, ids []string) error {
	if scope == "" || len(ids) == 0 {
		return nil
	}
	h, err := l.LoadSaveHistory()
	if err != nil {
		return err
	}
	existing := h.Scopes[scope]
	seen := make(map[string]bool, len(ids)+len(existing))
	next := make([]string, 0, len(ids)+len(existing))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		next = append(next, id)
	}
	for _, id := range existing {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		next = append(next, id)
		if len(next) >= MaxSaveHistory {
			break
		}
	}
	if len(next) > MaxSaveHistory {
		next = next[:MaxSaveHistory]
	}
	h.Scopes[scope] = next
	return l.store.Save("save_history.json", h, privateFile)
}
