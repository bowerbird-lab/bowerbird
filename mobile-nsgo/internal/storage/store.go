// Package storage owns the mobile app's versioned local data: the current
// session, connection history, usage history, the share inbox index and the
// phone-share tags. Writes are atomic and every file carries a version so the
// migration from the old Flutter app can be retried safely.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store is a JSON document directory. Every write goes to a temporary file in
// the same directory, is synced, then renamed over the target.
type Store struct {
	root string
	mu   sync.Mutex
}

// New creates a Store rooted at dir. It does not create the directory until a
// write happens.
func New(dir string) *Store { return &Store{root: dir} }

// Root returns the directory.
func (s *Store) Root() string { return s.root }

// Path resolves a relative document name under the root.
func (s *Store) Path(name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("invalid storage name %q", name)
	}
	return filepath.Join(s.root, clean), nil
}

// Load decodes name into v. found is false when the file does not exist. A
// corrupt document is renamed aside and reported, so the caller can start
// clean without destroying the only copy.
func (s *Store) Load(name string, v any) (found bool, err error) {
	path, err := s.Path(name)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		corrupt := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
		_ = os.Rename(path, corrupt)
		return false, fmt.Errorf("decode %s (moved to %s): %w", name, filepath.Base(corrupt), err)
	}
	return true, nil
}

// Save writes v atomically as indented JSON with the given file mode.
func (s *Store) Save(name string, v any, mode os.FileMode) error {
	path, err := s.Path(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

// WriteReader copies a stream to name atomically. It is used for share-intake
// payloads that must exist before the index points at them.
func (s *Store) WriteReader(name string, r io.Reader, mode os.FileMode) (int64, error) {
	path, err := s.Path(name)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	n, copyErr := io.Copy(tmp, r)
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if closeErr != nil {
		return n, closeErr
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return n, err
	}
	return n, os.Rename(tmpName, path)
}
