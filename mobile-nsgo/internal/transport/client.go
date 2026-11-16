package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
)

// Config configures a Client. BaseURL is the desktop origin, with or without
// a scheme; a missing scheme becomes http://.
type Config struct {
	BaseURL    string
	Token      string
	DeviceName string
	HTTP       *http.Client
}

// Client is the only HTTP entry point. It holds the active origin and token,
// decodes each endpoint separately and classifies failures.
type Client struct {
	mu         sync.RWMutex
	baseURL    string
	token      string
	deviceName string
	http       *http.Client
	queryOK    bool
	queryKnown bool
}

// New builds a Client. A missing HTTP client gets explicit dial and response
// timeouts; individual calls still pass their own contexts.
func New(cfg Config) *Client {
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			MaxIdleConnsPerHost:   8,
		}}
	}
	c := &Client{http: hc, deviceName: strings.TrimSpace(cfg.DeviceName)}
	if c.deviceName == "" {
		c.deviceName = "Bowerbird Mobile"
	}
	c.SetSession(cfg.BaseURL, cfg.Token)
	return c
}

// NormalizeBaseURL trims the address and adds http:// when no scheme is given.
// The result has no trailing slash.
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("address is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("address has no host")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimSuffix(u.String(), "/"), nil
}

// SetSession replaces the origin and token. It resets the query capability so
// the next call reprobes the desktop that was reached.
func (c *Client) SetSession(baseURL, token string) {
	normalized := strings.TrimSpace(baseURL)
	if n, err := NormalizeBaseURL(baseURL); err == nil {
		normalized = n
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if normalized != c.baseURL {
		c.queryKnown = false
		c.queryOK = false
	}
	c.baseURL = normalized
	c.token = strings.TrimSpace(token)
}

// BaseURL returns the active origin, "" before one is configured.
func (c *Client) BaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// Token returns the active bearer token.
func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// DeviceName is the name sent when pairing.
func (c *Client) DeviceName() string { return c.deviceName }

// SetDeviceName changes the name sent by later pairings.
func (c *Client) SetDeviceName(name string) {
	if name = strings.TrimSpace(name); name != "" {
		c.deviceName = name
	}
}

// Status performs the unauthenticated liveness probe.
func (c *Client) Status(ctx context.Context) (model.Status, error) {
	var out model.Status
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/status", nil, nil, &out)
	return out, err
}

// Pair exchanges a one-time code for a token. It does not save the token.
// The desktop wraps this reply in its standard {"status":"success","data":…}
// envelope, while older builds returned the fields at the top level. Accept
// both shapes so the client works across desktop versions.
func (c *Client) Pair(ctx context.Context, code string) (model.PairResult, error) {
	body := model.PairRequest{Code: strings.TrimSpace(code), DeviceName: c.deviceName}
	var reply struct {
		model.PairResult
		Data *model.PairResult `json:"data"`
	}
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/pair", nil, body, &reply)
	out := reply.PairResult
	if reply.Data != nil {
		out = *reply.Data
	}
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(out.Token) == "" {
		return out, &APIError{Kind: KindInvalid, Message: "pairing reply has no token"}
	}
	return out, nil
}

// Capabilities records what endpoints the desktop advertised and answered.
// Zero values mean not yet probed or unsupported.
type Capabilities struct {
	Query  bool
	Stats  bool
	Legacy bool
}

// ProbeQuery checks POST /api/v1/query with a cheap one-item request. A 404 or
// 405 means an older desktop: the client falls back to /items. A 401 is a
// session problem and is returned, never mistaken for "unsupported".
func (c *Client) ProbeQuery(ctx context.Context) error {
	c.mu.RLock()
	known, supported := c.queryKnown, c.queryOK
	c.mu.RUnlock()
	if known {
		if supported {
			return nil
		}
		return &APIError{Kind: KindUnsupported, Message: "query endpoint is unavailable"}
	}
	body := map[string]any{"query": "", "sort": model.SortDesc, "offset": 0, "limit": 1}
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/query", nil, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return networkError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		c.setQuerySupport(false)
		_, _ = io.Copy(io.Discard, resp.Body)
		return &APIError{Status: resp.StatusCode, Kind: KindUnsupported, Message: "desktop does not expose /query"}
	}
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	c.setQuerySupport(true)
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) setQuerySupport(ok bool) {
	c.mu.Lock()
	c.queryKnown, c.queryOK = true, ok
	c.mu.Unlock()
}

// QuerySupported reports the cached probe result. Unknown is false.
func (c *Client) QuerySupported() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.queryKnown && c.queryOK
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	base := c.BaseURL()
	if base == "" {
		return nil, &APIError{Kind: KindInvalid, Message: "no desktop address configured"}
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	var reader io.Reader
	if body != nil {
		data, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, marshalErr
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token := c.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body, out any) error {
	req, err := c.newRequest(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return networkError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
	if err := dec.Decode(out); err != nil {
		return &APIError{Status: resp.StatusCode, Kind: KindServer, Message: "invalid JSON reply", Err: err}
	}
	return nil
}

// errorBody is the desktop's writeError shape. It also accepts {"error": ...}.
type errorBody struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body errorBody
	_ = json.Unmarshal(data, &body)
	code := strings.TrimSpace(body.Code)
	if code == "" {
		code = strings.TrimSpace(body.Error)
	}
	message := strings.TrimSpace(body.Message)
	if message == "" {
		message = strings.TrimSpace(body.Error)
	}
	if message == "" {
		message = strings.TrimSpace(string(data))
	}
	if len(message) > 300 {
		message = message[:300]
	}
	kind := classifyStatus(resp.StatusCode, code, message)
	if resp.StatusCode == 404 && code == "NOT_FOUND" {
		kind = KindNotFound
	}
	return &APIError{Status: resp.StatusCode, Code: code, Message: message, Kind: kind}
}

func networkError(err error) error {
	if errors.Is(err, context.Canceled) {
		return &APIError{Kind: KindCanceled, Err: err}
	}
	kind := KindNetwork
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "deadline") {
		kind = KindTimeout
	}
	return &APIError{Kind: kind, Err: err}
}

// Redact removes a token query parameter from a URL before it is logged or
// shown in an error message.
func Redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Has("token") {
		q.Set("token", "redacted")
		u.RawQuery = q.Encode()
	}
	return u.String()
}
