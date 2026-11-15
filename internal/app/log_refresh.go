package app

import "sync"

// logRefreshState coalesces any number of events into one pending UI task.
// Only the refresh callback is queued; entries stay in core's bounded buffer.
type logRefreshState struct {
	mu         sync.Mutex
	post       func(func())
	pending    bool
	generation uint64
}

func (s *logRefreshState) setPost(post func(func())) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.post = post
	s.pending = false
	s.generation++
}

func (s *logRefreshState) request(refresh func()) {
	s.mu.Lock()
	if s.post == nil || s.pending {
		s.mu.Unlock()
		return
	}
	post, generation := s.post, s.generation
	s.pending = true
	s.mu.Unlock()

	post(func() {
		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			return
		}
		s.pending = false
		s.mu.Unlock()
		refresh()
	})
}
