package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// grantFingerprint binds user consent to the currently declared scope.
func grantFingerprint(m Manifest, capability string) string {
	var scope []string
	switch capability {
	case "network":
		scope = append(scope, m.Permissions.Network...)
	case "assets.read", "assets.write":
		want := capability[len("assets."):]
		for _, v := range m.Permissions.Assets {
			if v == want {
				scope = append(scope, v)
			}
		}
	default:
		return ""
	}
	if len(scope) == 0 {
		return ""
	}
	sort.Strings(scope)
	b, _ := json.Marshal(scope)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (h *Host) UIGranted(id, capability string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.plugins[id]
	if !ok || !p.Enabled {
		return false
	}
	fingerprint := grantFingerprint(p.Manifest, capability)
	return fingerprint != "" && p.Grants[capability] == fingerprint
}

func (h *Host) SetUIGrant(id, capability string, allowed bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.plugins[id]
	if !ok {
		return false
	}
	fingerprint := grantFingerprint(p.Manifest, capability)
	if fingerprint == "" {
		return false
	}
	if p.Grants == nil {
		p.Grants = make(map[string]string)
	}
	if allowed {
		p.Grants[capability] = fingerprint
	} else {
		delete(p.Grants, capability)
	}
	h.saveIndexLocked()
	return true
}
