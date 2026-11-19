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

func (m *SessionManager) saveToken(baseURL, deviceID, token string) {
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
	if deviceID != "" {
		m.Secure.Set(connTokenKey("device:"+deviceID), token, func(error) {})
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
		m.saveToken(s.BaseURL, s.DeviceID, s.Token)
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
			m.saveToken(s.BaseURL, s.DeviceID, s.Token)
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

// LoadConnectionToken resolves the stored token for a historical connection,
// checking device ID, active BaseURL, and any bound candidate IPs.
func (m *SessionManager) LoadConnectionToken(c storage.Connection, done func(string)) {
	if c.Token != "" {
		if m.Secure != nil {
			m.saveToken(c.BaseURL, c.DeviceID, c.Token)
			_ = m.Local.Remember(c)
		}
		done(c.Token)
		return
	}
	if m.Secure == nil {
		done("")
		return
	}
	var keys []string
	seen := make(map[string]bool)
	addKey := func(raw string) {
		if raw == "" {
			return
		}
		k := connTokenKey(raw)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if c.DeviceID != "" {
		addKey("device:" + c.DeviceID)
	}
	addKey(c.BaseURL)
	for _, u := range connectionCandidateURLs(c) {
		addKey(u)
	}
	var tryNext func(idx int)
	tryNext = func(idx int) {
		if idx >= len(keys) {
			done("")
			return
		}
		m.Secure.Get(keys[idx], func(val string, _ error) {
			if val != "" {
				done(val)
				return
			}
			tryNext(idx + 1)
		})
	}
	tryNext(0)
}

// ForgetConnection removes a remembered desktop and all its encrypted tokens.
func (m *SessionManager) ForgetConnection(baseURL string) error {
	if m.Secure != nil && baseURL != "" {
		if history, err := m.Local.History(); err == nil {
			for _, c := range history {
				if strings.EqualFold(strings.TrimRight(c.BaseURL, "/"), strings.TrimRight(baseURL, "/")) || (c.DeviceID != "" && c.DeviceID == baseURL) {
					if c.DeviceID != "" {
						m.Secure.Delete(connTokenKey("device:"+c.DeviceID), func(error) {})
					}
					for _, u := range connectionCandidateURLs(c) {
						m.Secure.Delete(connTokenKey(u), func(error) {})
					}
				}
			}
		}
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
// saving anything. On success it saves the session and its history, binding any
// additional candidate IPs reported by the desktop.
func (m *SessionManager) Connect(ctx context.Context, baseURL, code, deviceName string) (*storage.Session, error) {
	return m.ConnectWithCandidates(ctx, baseURL, nil, 0, code, deviceName)
}

// ConnectWithCandidates connects to baseURL (or probes hosts when baseURL is empty),
// pairs with code if needed, and binds all candidate IPs to the single device session.
func (m *SessionManager) ConnectWithCandidates(ctx context.Context, baseURL string, extraHosts []string, port int, code, deviceName string) (*storage.Session, error) {
	var normalized string
	var status model.Status
	var err error

	if strings.TrimSpace(baseURL) != "" {
		normalized, err = transport.NormalizeBaseURL(baseURL)
		if err != nil {
			return nil, &transport.APIError{Kind: transport.KindInvalid, Message: "地址无效: " + err.Error()}
		}
		m.Client.SetSession(normalized, "")
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		status, err = m.Client.Status(probeCtx)
		cancel()
		if err != nil {
			return nil, err
		}
	} else {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		normalized, status, err = ProbeCandidates(probeCtx, m.Client, extraHosts, port)
		cancel()
		if err != nil {
			return nil, err
		}
		m.Client.SetSession(normalized, "")
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

	activeHost, activePort := storage.HostPortFromURL(normalized)
	if port <= 0 {
		if status.Port > 0 {
			port = status.Port
		} else if activePort > 0 {
			port = activePort
		} else {
			port = 17878
		}
	}
	candidates := storage.MergeIPs([]string{activeHost}, extraHosts, status.Addresses)

	session := storage.Session{
		Version:      storage.SessionVersion,
		DeviceID:     status.DeviceID,
		BaseURL:      normalized,
		Token:        token,
		CandidateIPs: candidates,
		Port:         port,
		DeviceName:   deviceName,
		LibraryName:  status.Name,
		ConnectedAt:  time.Now().UnixMilli(),
	}
	m.Client.SetSession(normalized, token)
	if err := m.Local.SaveSession(session); err != nil {
		return nil, err
	}
	m.saveToken(normalized, status.DeviceID, token)
	m.mu.Lock()
	m.session = &session
	m.mu.Unlock()
	return &session, nil
}

// ConnectWithToken reconnects a remembered desktop without a code. When the
// device has multiple bound IPs, all candidates are tested in parallel and the
// fastest reachable address is used automatically.
func (m *SessionManager) ConnectWithToken(ctx context.Context, c storage.Connection) (*storage.Session, error) {
	candidates := connectionCandidateURLs(c)
	if len(candidates) == 0 {
		return nil, &transport.APIError{Kind: transport.KindInvalid, Message: "地址无效: empty address"}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	bestBase, status, err := ProbeCandidates(probeCtx, m.Client, candidates, c.Port)
	cancel()
	if err != nil {
		return nil, err
	}

	m.Client.SetSession(bestBase, c.Token)
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

	deviceID := status.DeviceID
	if deviceID == "" {
		deviceID = c.DeviceID
	}
	port := c.Port
	if status.Port > 0 {
		port = status.Port
	} else if _, p := storage.HostPortFromURL(bestBase); port <= 0 && p > 0 {
		port = p
	}
	activeHost, _ := storage.HostPortFromURL(bestBase)
	mergedIPs := storage.MergeIPs([]string{activeHost}, c.CandidateIPs, status.Addresses)

	session := storage.Session{
		Version:      storage.SessionVersion,
		DeviceID:     deviceID,
		BaseURL:      bestBase,
		Token:        c.Token,
		CandidateIPs: mergedIPs,
		Port:         port,
		DeviceName:   c.Name,
		LibraryID:    c.LibraryID,
		LibraryName:  c.LibraryName,
		ConnectedAt:  time.Now().UnixMilli(),
	}
	if session.LibraryName == "" {
		session.LibraryName = status.Name
	}
	if err := m.Local.SaveSession(session); err != nil {
		return nil, err
	}
	m.saveToken(bestBase, deviceID, c.Token)
	m.Client.SetSession(bestBase, c.Token)
	m.mu.Lock()
	m.session = &session
	m.status = status
	m.mu.Unlock()
	return &session, nil
}

func connectionCandidateURLs(c storage.Connection) []string {
	port := c.Port
	if port <= 0 {
		if _, p := storage.HostPortFromURL(c.BaseURL); p > 0 {
			port = p
		} else {
			port = 17878
		}
	}
	hosts := make([]string, 0, 1+len(c.CandidateIPs))
	if strings.TrimSpace(c.BaseURL) != "" {
		hosts = append(hosts, c.BaseURL)
	}
	hosts = append(hosts, c.CandidateIPs...)
	return candidateBaseURLs(hosts, port)
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
	Latency    time.Duration
	Err        error
}

// ProbeCandidates tests all candidate addresses in parallel with a short status
// request and returns the fastest reachable one. It is used by QR pairing,
// discovery, and multi-IP reconnects.
func ProbeCandidates(ctx context.Context, client *transport.Client, hosts []string, port int) (string, model.Status, error) {
	return ProbeCandidatesWithProgress(ctx, client, hosts, port, nil)
}

// ProbeCandidatesWithProgress tests all candidate addresses concurrently and
// picks the fastest reachable desktop, cancelling remaining probes immediately.
func ProbeCandidatesWithProgress(ctx context.Context, client *transport.Client, hosts []string, port int, progress func(ProbeProgress)) (string, model.Status, error) {
	if port <= 0 {
		port = 17878
	}
	candidates := candidateBaseURLs(hosts, port)
	if len(candidates) == 0 {
		return "", model.Status{}, errors.New("pairing payload has no usable addresses")
	}

	if progress != nil {
		for i, candidate := range candidates {
			progress(ProbeProgress{Index: i, Total: len(candidates), Address: candidate})
		}
	}

	type probeResult struct {
		index     int
		candidate string
		status    model.Status
		latency   time.Duration
		err       error
	}

	probeCtx, cancelAll := context.WithCancel(ctx)
	defer cancelAll()

	results := make(chan probeResult, len(candidates))
	for i, candidate := range candidates {
		go func(idx int, addr string) {
			reqCtx, reqCancel := context.WithTimeout(probeCtx, 3*time.Second)
			defer reqCancel()
			start := time.Now()
			status, err := client.StatusAt(reqCtx, addr)
			results <- probeResult{
				index:     idx,
				candidate: addr,
				status:    status,
				latency:   time.Since(start),
				err:       err,
			}
		}(i, candidate)
	}

	var lastErr error
	for range candidates {
		res := <-results
		if res.err == nil {
			cancelAll()
			if progress != nil {
				progress(ProbeProgress{
					Index:      res.index,
					Total:      len(candidates),
					Address:    res.candidate,
					Responsive: true,
					Latency:    res.latency,
				})
			}
			return res.candidate, res.status, nil
		}
		lastErr = res.err
		if progress != nil {
			progress(ProbeProgress{
				Index:   res.index,
				Total:   len(candidates),
				Address: res.candidate,
				Latency: res.latency,
				Err:     res.err,
			})
		}
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		return "", model.Status{}, ctx.Err()
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
