package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
)

// WSConn is one live WebSocket session.
type WSConn struct {
	conn *websocket.Conn
}

// DialWS opens /api/v1/ws with the active token. The token travels as a query
// parameter because that is what the desktop contract accepts for media and
// sockets; callers must not print the URL.
func (c *Client) DialWS(ctx context.Context) (*WSConn, error) {
	base := c.BaseURL()
	if base == "" {
		return nil, &APIError{Kind: KindInvalid, Message: "no desktop address configured"}
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return nil, &APIError{Kind: KindInvalid, Message: "unsupported scheme " + u.Scheme}
	}
	u.Path = "/api/v1/ws"
	q := u.Query()
	if token := c.Token(); token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, u.String(), &websocket.DialOptions{
		HTTPClient: c.http,
	})
	if err != nil {
		return nil, &APIError{Kind: KindNetwork, Message: "websocket connection failed", Err: err}
	}
	conn.SetReadLimit(4 << 20)
	return &WSConn{conn: conn}, nil
}

// Read waits for the next event. A closed socket returns an error; the caller
// decides whether to reconnect.
func (w *WSConn) Read(ctx context.Context) (model.WSEvent, error) {
	if w == nil || w.conn == nil {
		return model.WSEvent{}, &APIError{Kind: KindNetwork, Message: "websocket is not connected"}
	}
	_, data, err := w.conn.Read(ctx)
	if err != nil {
		return model.WSEvent{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return model.WSEvent{}, err
	}
	var ev model.WSEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return model.WSEvent{}, err
	}
	ev.Raw = raw
	if ev.Event == "" {
		if s, ok := raw["event"].(string); ok {
			ev.Event = s
		}
	}
	return ev, nil
}

// Close ends the socket.
func (w *WSConn) Close() {
	if w == nil || w.conn == nil {
		return
	}
	_ = w.conn.Close(websocket.StatusNormalClosure, "client closed")
	w.conn = nil
}

// EventName reads the event name from either the typed or raw message.
func EventName(ev model.WSEvent) string {
	if ev.Event != "" {
		return ev.Event
	}
	if s, ok := ev.Raw["event"].(string); ok {
		return s
	}
	return ""
}

// Revoked reports the pairing_revoked event.
func Revoked(ev model.WSEvent) bool {
	return EventName(ev) == model.EventPairingRevoked
}

// LibraryChanged reports library_changed, with the new library name if sent.
func LibraryChanged(ev model.WSEvent) (string, bool) {
	if EventName(ev) != model.EventLibraryChanged {
		return "", false
	}
	if len(ev.Data) > 0 {
		var data struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(ev.Data, &data); err == nil {
			return data.Name, true
		}
	}
	return "", true
}

// Backoff returns the reconnect delay after n failures: 1, 2, 4, 8, 16 then
// 30 seconds, matching the old client's rhythm.
func Backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}
	if attempt >= len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt]
}

// WSURL is the redacted socket address, for diagnostics.
func (c *Client) WSURL() string {
	u, err := url.Parse(c.BaseURL())
	if err != nil {
		return ""
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/ws"
	if token := c.Token(); token != "" {
		q := u.Query()
		q.Set("token", token)
		u.RawQuery = q.Encode()
	}
	return Redact(u.String())
}

// ValidatePairPayload checks a QR/deep-link JSON object. It accepts the
// canonical {"v":1,"ips":[...],"port":...,"code":...} shape.
func ValidatePairPayload(raw []byte) (ips []string, port int, code string, err error) {
	var payload struct {
		V    int      `json:"v"`
		IPs  []string `json:"ips"`
		Port int      `json:"port"`
		Code string   `json:"code"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, 0, "", fmt.Errorf("invalid pairing payload: %w", err)
	}
	for _, ip := range payload.IPs {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return nil, 0, "", fmt.Errorf("pairing payload has no addresses")
	}
	if payload.Port <= 0 || payload.Port > 65535 {
		payload.Port = 17878
	}
	code = strings.TrimSpace(payload.Code)
	if code == "" {
		return nil, 0, "", fmt.Errorf("pairing payload has no code")
	}
	return ips, payload.Port, code, nil
}
