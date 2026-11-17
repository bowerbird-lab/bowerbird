package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const MaxRecentlyUsed = 300

var recentUsedMu sync.Mutex

func (s *Service) recentUsedFilePath() (string, error) {
	lib, err := s.GetActiveLibrary()
	if err != nil || lib == nil || lib.Path == "" {
		return "", err
	}
	return filepath.Join(lib.Path, "recent_used.json"), nil
}

// GetRecentlyUsedIDs returns the list of recently used item IDs (newest first, max 300).
func (s *Service) GetRecentlyUsedIDs() ([]string, error) {
	if s == nil {
		return nil, nil
	}
	path, err := s.recentUsedFilePath()
	if err != nil || path == "" {
		return nil, err
	}
	recentUsedMu.Lock()
	defer recentUsedMu.Unlock()
	return readRecentUsedFile(path)
}

// RecordRecentlyUsed prepends the given item IDs to the recently used list (deduplicated, max 300).
func (s *Service) RecordRecentlyUsed(ids []string) error {
	if s == nil || len(ids) == 0 {
		return nil
	}
	path, err := s.recentUsedFilePath()
	if err != nil || path == "" {
		return err
	}
	recentUsedMu.Lock()
	defer recentUsedMu.Unlock()

	existing, _ := readRecentUsedFile(path)
	seen := make(map[string]bool, len(ids)+len(existing))
	merged := make([]string, 0, min(MaxRecentlyUsed, len(ids)+len(existing)))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, id)
		if len(merged) >= MaxRecentlyUsed {
			break
		}
	}
	for _, id := range existing {
		if len(merged) >= MaxRecentlyUsed {
			break
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, id)
	}

	data, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readRecentUsedFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	if len(ids) > MaxRecentlyUsed {
		ids = ids[:MaxRecentlyUsed]
	}
	return ids, nil
}
