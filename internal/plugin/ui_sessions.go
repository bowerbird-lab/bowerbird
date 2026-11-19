package plugin

import (
	"net/http"
	"sync"
)

// UISessions binds a WebView identity supplied by the host to one plugin's
// resources. Request URLs never select the plugin whose files are served.
// Session identities must be created and revoked by the window owner.
type UISessions struct {
	mu    sync.RWMutex
	pages map[any]http.Handler
}

func (s *UISessions) Attach(page any, pluginDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pages == nil {
		s.pages = make(map[any]http.Handler)
	}
	s.pages[page] = UIResourceHandler(pluginDir)
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
	h := s.pages[page]
	s.mu.RUnlock()
	if h == nil {
		http.Error(w, "unknown plugin page", http.StatusForbidden)
		return
	}
	h.ServeHTTP(w, r)
}
