package plugin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxUITransfer = 24 << 20

type uiNetworkRequest struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"bodyBase64"`
}

func (h *Host) uiNetworkAllowed(id string, u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || !h.UIGranted(id, "network") {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.plugins[id]
	if !ok || !p.Enabled {
		return false
	}
	for _, rule := range p.Manifest.Permissions.Network {
		if matchURLHost(u.Hostname(), rule) {
			return true
		}
	}
	return false
}

// UINetworkHandler makes scoped requests only after explicit user consent.
func (h *Host) UINetworkHandler(id string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var input uiNetworkRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, maxUITransfer*2)).Decode(&input); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		u, err := url.Parse(input.URL)
		if err != nil || !h.uiNetworkAllowed(id, u) {
			http.Error(w, "network permission denied", 403)
			return
		}
		method := strings.ToUpper(input.Method)
		if method == "" {
			method = http.MethodGet
		}
		switch method {
		case "GET", "POST", "PUT", "DELETE", "PATCH":
		default:
			http.Error(w, "unsupported method", 400)
			return
		}
		body, err := base64.StdEncoding.DecodeString(input.BodyBase64)
		if err != nil || len(body) > maxUITransfer {
			http.Error(w, "invalid body", 400)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), method, u.String(), bytes.NewReader(body))
		if err != nil {
			http.Error(w, "invalid URL", 400)
			return
		}
		for k, v := range input.Headers {
			if strings.EqualFold(k, "host") || strings.EqualFold(k, "cookie") || strings.EqualFold(k, "content-length") {
				http.Error(w, "unsupported header", 400)
				return
			}
			req.Header.Set(k, v)
		}
		client := *h.currentHTTPClient()
		client.Jar = nil
		client.Timeout = 90 * time.Second
		client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 || !h.uiNetworkAllowed(id, next.URL) {
				return http.ErrUseLastResponse
			}
			return nil
		}
		response, err := client.Do(req)
		if err != nil {
			http.Error(w, "network request failed", 502)
			return
		}
		defer response.Body.Close()
		result, err := io.ReadAll(io.LimitReader(response.Body, maxUITransfer+1))
		if err != nil || len(result) > maxUITransfer {
			http.Error(w, "response too large or unreadable", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": response.StatusCode, "contentType": response.Header.Get("Content-Type"), "bodyBase64": base64.StdEncoding.EncodeToString(result)})
	})
}
