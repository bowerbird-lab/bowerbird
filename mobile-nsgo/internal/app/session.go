package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/platform"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

const (
	secureSessionTokenKey = "bowerbird_token"
	secureConnPrefix      = "bowerbird_conn_token_"
)

// ErrPairingRequired means the desktop is reachable and needs a code before
// the client may read the library.
var ErrPairingRequired = errors.New("pairing required")

// SessionManager owns the active connection and the remembered library
// identity. Tokens live in platform.SecureStore when available and are never
// written to JSON files.
type SessionManager struct {
	Client *transport.Client
	Local  *storage.Local
	Secure platform.SecureStore

	mu      sync.RWMutex
	session *storage.Session
	status  model.Status
}

// NewSessionManager wires the transport, local documents and optional host
// encrypted storage.
func NewSessionManager(client *transport.Client, local *storage.Local, secure platform.SecureStore) *SessionManager {
	return &SessionManager{Client: client, Local: local, Secure: secure}
}

func connTokenKey(baseURL string) string {
	norm := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	sum := sha256.Sum256([]byte(norm))
	return secureConnPrefix + hex.EncodeToString(sum[:16])
}

func (m *SessionManager) saveToken(baseURL, token string) {
	if m.Secure == nil {
		return
	}
	if token == "" {
		m.Secure.Delete(secureSessionTokenKey, func(error) {})
		return
	}
	m.Secure.Set(secureSessionTokenKey, token, func(error) {})
	if baseURL != "" {
		m.Secure.Set(connTokenKey(baseURL), token, func(error) {})
	}
}

// Restore loads the last session synchronously (using JSON metadata plus
// synchronous SecureStore when available, e.g. in tests).
func (m *SessionManager) Restore() (*storage.Session, error) {
	s, err := m.Local.LoadSession()
	if err != nil || s == nil {
		return nil, err
	}
	if s.Token != "" && m.Secure != nil {
		// Migrate legacy plaintext token out of session.json.
		m.saveToken(s.BaseURL, s.Token)
		_ = m.Local.SaveSession(*s)
	}
	if s.Token == "" && m.Secure != nil {
		var tok string
		var done bool
		m.Secure.Get(secureSessionTokenKey, func(v string, _ error) {
			tok = v
			done = true
		})
		if done && tok != "" {
			s.Token = tok
		}
	}
	m.Client.SetSession(s.BaseURL, s.Token)
	m.mu.Lock()
	m.session = s
	m.mu.Unlock()
	return s, nil
}

// RestoreAsync loads session metadata and resolves the encrypted token from
// SecureStore asynchronously (for Android HostChannel).
func (m *SessionManager) RestoreAsync(done func(*storage.Session, error)) {
	s, err := m.Local.LoadSession()
	if err != nil || s == nil {
		done(nil, err)
		return
	}
	if s.Token != "" {
		if m.Secure != nil {
			m.saveToken(s.BaseURL, s.Token)
			_ = m.Local.SaveSession(*s)
		}
		m.Client.SetSession(s.BaseURL, s.Token)
		m.mu.Lock()
		m.session = s
		m.mu.Unlock()
		done(s, nil)
		return
	}
	if m.Secure == nil {
		m.Client.SetSession(s.BaseURL, "")
		m.mu.Lock()
		m.session = s
		m.mu.Unlock()
		done(s, nil)
		return
	}
	m.Secure.Get(secureSessionTokenKey, func(val string, getErr error) {
		if val == "" {
			m.Secure.Get(connTokenKey(s.BaseURL), func(connVal string, _ error) {
				s.Token = connVal
				m.Client.SetSession(s.BaseURL, s.Token)
				m.mu.Lock()
				m.session = s
				m.mu.Unlock()
				done(s, nil)
			})
			return
		}
		_ = getErr
		s.Token = val
		m.Client.SetSession(s.BaseURL, s.Token)
		m.mu.Lock()
		m.session = s
		m.mu.Unlock()
		done(s, nil)
	})
}

// LoadConnectionToken resolves the stored token for a historical connection.
func (m *SessionManager) LoadConnectionToken(c storage.Connection, done func(string)) {
	if c.Token != "" {
		if m.Secure != nil {
			m.Secure.Set(connTokenKey(c.BaseURL), c.Token, func(error) {})
			_ = m.Local.Remember(c)
		}
		done(c.Token)
		return
	}
	if m.Secure == nil {
		done("")
		return
	}
	m.Secure.Get(connTokenKey(c.BaseURL), func(val string, _ error) {
		done(val)
	})
}

// ForgetConnection removes a remembered desktop and its encrypted token.
func (m *SessionManager) ForgetConnection(baseURL string) error {
	if m.Secure != nil && baseURL != "" {
		m.Secure.Delete(connTokenKey(baseURL), func(error) {})
	}
	return m.Local.Forget(baseURL)
}

// Current returns the active session, or nil.
func (m *SessionManager) Current() *storage.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return nil
	}
	copy := *m.session
	return &copy
}

// Status returns the most recent status reply.
func (m *SessionManager) Status() model.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Connect probes one address and, when needed, pairs with code. An empty code
// against a desktop that requires pairing returns ErrPairingRequired without
// saving anything. On success it saves the session and its history.
func (m *SessionManager) Connect(ctx context.Context, baseURL, code, deviceName string) (*storage.Session, error) {
	normalized, err := transport.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, &transport.APIError{Kind: transport.KindInvalid, Message: "地址无效: " + err.Error()}
	}
	m.Client.SetSession(normalized, "")
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	status, err := m.Client.Status(probeCtx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.status = status
	m.mu.Unlock()

	token := ""
	if status.PairingRequired || strings.TrimSpace(code) != "" {
		if strings.TrimSpace(code) == "" {
			return nil, ErrPairingRequired
		}
		if deviceName != "" {
			m.Client.SetDeviceName(deviceName)
		}
		pairCtx, pairCancel := context.WithTimeout(ctx, 12*time.Second)
		defer pairCancel()
		paired, pairErr := m.Client.Pair(pairCtx, code)
		if pairErr != nil {
			return nil, pairErr
		}
		token = paired.Token
	}

	session := storage.Session{
		Version:     storage.SessionVersion,
		BaseURL:     normalized,
		Token:       token,
		DeviceName:  deviceName,
		LibraryName: status.Name,
		ConnectedAt: time.Now().UnixMilli(),
	}
	m.Client.SetSession(normalized, token)
	if err := m.Local.SaveSession(session); err != nil {
		return nil, err
	}
	m.saveToken(normalized, token)
	m.mu.Lock()
	m.session = &session
	m.mu.Unlock()
	return &session, nil
}

// ConnectWithToken reconnects a remembered desktop without a code.
func (m *SessionManager) ConnectWithToken(ctx context.Context, c storage.Connection) (*storage.Session, error) {
	m.Client.SetSession(c.BaseURL, c.Token)
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	status, err := m.Client.Status(probeCtx)
	if err != nil {
		return nil, err
	}
	if status.PairingRequired && strings.TrimSpace(c.Token) == "" {
		return nil, ErrPairingRequired
	}
	if status.PairingRequired {
		// Verify token is still accepted by probing libraries/stats.
		verifyCtx, vCancel := context.WithTimeout(ctx, 6*time.Second)
		_, verifyErr := m.Client.Libraries(verifyCtx)
		vCancel()
		if transport.IsUnauthorized(verifyErr) {
			return nil, verifyErr
		}
	}
	session := storage.Session{
		Version:     storage.SessionVersion,
		BaseURL:     c.BaseURL,
		Token:       c.Token,
		DeviceName:  c.Name,
		LibraryID:   c.LibraryID,
		LibraryName: c.LibraryName,
		ConnectedAt: time.Now().UnixMilli(),
	}
	if session.LibraryName == "" {
		session.LibraryName = status.Name
	}
	if err := m.Local.SaveSession(session); err != nil {
		return nil, err
	}
	m.saveToken(c.BaseURL, c.Token)
	m.Client.SetSession(c.BaseURL, c.Token)
	m.mu.Lock()
	m.session = &session
	m.status = status
	m.mu.Unlock()
	return &session, nil
}

// Disconnect clears the active session and its session token while keeping history.
func (m *SessionManager) Disconnect() error {
	m.Client.SetSession("", "")
	m.mu.Lock()
	m.session = nil
	m.status = model.Status{}
	m.mu.Unlock()
	if m.Secure != nil {
		m.Secure.Delete(secureSessionTokenKey, func(error) {})
	}
	return m.Local.ClearSession()
}

// Libraries lists the desktop's libraries and refreshes the active library ID
// on the current session when matched.
func (m *SessionManager) Libraries(ctx context.Context) ([]model.Library, error) {
	libs, err := m.Client.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	for _, lib := range libs {
		if lib.IsActive && lib.ID != "" {
			m.mu.Lock()
			changed := false
			if m.session != nil && (m.session.LibraryID != lib.ID || m.session.LibraryName != lib.Name) {
				m.session.LibraryID = lib.ID
				m.session.LibraryName = lib.Name
				changed = true
			}
			s := m.session
			m.mu.Unlock()
			if changed && s != nil {
				_ = m.Local.SaveSession(*s)
			}
			break
		}
	}
	return libs, nil
}

// SwitchLibrary changes the desktop's active library and updates the session
// identity. Every list and detail must be invalidated by the caller.
func (m *SessionManager) SwitchLibrary(ctx context.Context, lib model.Library) error {
	if err := m.Client.SwitchLibrary(ctx, model.SwitchLibraryRequest{ID: lib.ID, Path: lib.Path}); err != nil {
		return err
	}
	m.mu.Lock()
	if m.session != nil {
		m.session.LibraryID = lib.ID
		m.session.LibraryName = lib.Name
	}
	s := m.session
	m.mu.Unlock()
	if s != nil {
		return m.Local.SaveSession(*s)
	}
	return nil
}

// UseLibraryName records the library the desktop reports after a
// library_changed event.
func (m *SessionManager) UseLibraryName(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	m.mu.Lock()
	if m.session != nil {
		if m.session.LibraryName != name {
			m.session.LibraryID = ""
		}
		m.session.LibraryName = name
	}
	s := m.session
	m.mu.Unlock()
	if s != nil {
		_ = m.Local.SaveSession(*s)
	}
}

// ScopeKey returns the storage scope for usage history and share marks. A
// stable library ID is required to share data across renames; otherwise the
// legacy name scope is used, and never mixed with stable data.
func (m *SessionManager) ScopeKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return ""
	}
	if m.session.LibraryID != "" {
		return "stable:" + m.session.LibraryID
	}
	if m.session.LibraryName != "" {
		return "legacy:name:" + m.session.LibraryName
	}
	return ""
}

// LegacyNameScope returns the legacy name scope even when a stable library ID
// is known, so read-only lookups can still match migrated Flutter share marks.
func (m *SessionManager) LegacyNameScope() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil || m.session.LibraryName == "" {
		return ""
	}
	return "legacy:name:" + m.session.LibraryName
}

// ProbeProgress describes one candidate checked by ProbeCandidatesWithProgress.
// Address is already normalized to host:port, so the connect screen can show
// the address currently being tested.
type ProbeProgress struct {
	Index      int
	Total      int
	Address    string
	Responsive bool
	Err        error
}

// ProbeCandidates tries each address in order with a short status request and
// returns the first that answers. It is used by QR pairing and discovery.
func ProbeCandidates(ctx context.Context, client *transport.Client, hosts []string, port int) (string, model.Status, error) {
	return ProbeCandidatesWithProgress(ctx, client, hosts, port, nil)
}

// ProbeCandidatesWithProgress is ProbeCandidates with a callback for the
// connect screen. QR payloads can carry several LAN/Tailscale addresses; each
// one is checked in payload order and the first reachable desktop wins.
func ProbeCandidatesWithProgress(ctx context.Context, client *transport.Client, hosts []string, port int, progress func(ProbeProgress)) (string, model.Status, error) {
	if port <= 0 {
		port = 17878
	}
	candidates := candidateBaseURLs(hosts, port)
	if len(candidates) == 0 {
		return "", model.Status{}, errors.New("pairing payload has no usable addresses")
	}
	var lastErr error
	for i, candidate := range candidates {
		if progress != nil {
			progress(ProbeProgress{Index: i, Total: len(candidates), Address: candidate})
		}
		client.SetSession(candidate, "")
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		status, err := client.Status(probeCtx)
		cancel()
		if err == nil {
			if progress != nil {
				progress(ProbeProgress{Index: i, Total: len(candidates), Address: candidate, Responsive: true})
			}
			return candidate, status, nil
		}
		lastErr = err
		if progress != nil {
			progress(ProbeProgress{Index: i, Total: len(candidates), Address: candidate, Err: err})
		}
		if ctx.Err() != nil {
			// A user/app cancellation should still stop immediately. A probe
			// budget expiry, however, is reported as a candidate failure so the
			// UI does not claim the user cancelled the scan.
			if errors.Is(ctx.Err(), context.Canceled) {
				return "", model.Status{}, ctx.Err()
			}
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no candidate address answered")
	} else {
		lastErr = fmt.Errorf("no candidate address answered: %w", lastErr)
	}
	return "", model.Status{}, lastErr
}

// candidateBaseURLs normalizes, de-duplicates and preserves the payload order
// of QR candidate addresses. Entries may be bare hosts, host:port pairs or
// full http(s) URLs.
func candidateBaseURLs(hosts []string, port int) []string {
	out := make([]string, 0, len(hosts))
	seen := make(map[string]bool, len(hosts))
	for _, raw := range hosts {
		base, err := candidateBaseURL(raw, port)
		if err != nil || base == "" || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	return out
}

func candidateBaseURL(raw string, port int) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty address")
	}
	if strings.Contains(raw, "://") {
		return transport.NormalizeBaseURL(raw)
	}
	host := raw
	portText := ""
	if h, p, err := net.SplitHostPort(raw); err == nil {
		host, portText = h, p
	} else if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		host = strings.Trim(raw, "[]")
	} else if strings.Count(raw, ":") > 1 {
		// Bare IPv6 literal without a port.
		host = raw
	}
	if host == "" {
		return "", errors.New("empty host")
	}
	if portText == "" {
		portText = strconv.Itoa(port)
	}
	return transport.NormalizeBaseURL(net.JoinHostPort(host, portText))
}

// EventAction is what the app must do after a WebSocket event.
type EventAction int

const (
	EventIgnore EventAction = iota
	EventRefresh
	EventLibraryChanged
	EventRevoked
)

// ClassifyEvent maps a socket event to the state change it requires.
func ClassifyEvent(ev model.WSEvent) (EventAction, string) {
	switch transport.EventName(ev) {
	case model.EventItemsUpdated:
		return EventRefresh, ""
	case model.EventLibraryChanged:
		name, _ := transport.LibraryChanged(ev)
		return EventLibraryChanged, name
	case model.EventPairingRevoked:
		return EventRevoked, ""
	default:
		return EventIgnore, ""
	}
}
