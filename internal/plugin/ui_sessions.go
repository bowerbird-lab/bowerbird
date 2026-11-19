package plugin

import (
	"net/http"
	"strings"
	"sync"
)

// UISessions binds a WebView identity supplied by the host to one plugin's
// resources. Request URLs never select the plugin whose files are served.
// Session identities must be created and revoked by the window owner.
type UISessions struct {
	mu    sync.RWMutex
	pages map[any]uiSession
}

type uiSession struct {
	handler http.Handler
	api     http.Handler
	allowed func() bool
}

func (s *UISessions) Attach(page any, pluginDir string, allowed func() bool, api http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pages == nil {
		s.pages = make(map[any]uiSession)
	}
	s.pages[page] = uiSession{handler: UIResourceHandler(pluginDir), api: api, allowed: allowed}
}

func (s *UISessions) Detach(page any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pages, page)
}

// ServePage receives an identity from the WebView backend, not from plugin
// JavaScript or a URL. Unknown and closed pages are denied.
func (s *UISessions) ServePage(page any, w http.ResponseWriter, r *http.Request) {
	if page == nil {
		http.Error(w, "unknown plugin page", http.StatusForbidden)
		return
	}
	s.mu.RLock()
	session, ok := s.pages[page]
	s.mu.RUnlock()
	if !ok || session.allowed == nil || !session.allowed() {
		http.Error(w, "unknown plugin page", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/_api/") {
		if session.api == nil {
			http.NotFound(w, r)
			return
		}
		session.api.ServeHTTP(w, r)
		return
	}
	session.handler.ServeHTTP(w, r)
}
