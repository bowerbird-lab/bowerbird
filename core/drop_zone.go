package core

import "strings"

// DropZoneSettingsChangedEvent is broadcast whenever the top drop-zone
// configuration changes so the host can reconcile floating windows and the
// settings UI can refresh.
const DropZoneSettingsChangedEvent = "drop-zone-settings-changed"

const (
	DropZoneStyleNotch   = "notch"
	DropZoneStyleCapsule = "capsule"
)

const (
	DropZoneScreenPrimary = "primary"
	DropZoneScreenAll     = "all"
	DropZoneScreenCustom  = "custom"
)

// DropZoneSettings controls the floating top-of-screen drop zone overlay.
type DropZoneSettings struct {
	Enabled    bool    `json:"enabled"`
	Style      string  `json:"style"`
	ScreenMode string  `json:"screenMode"`
	ScreenIDs  []int64 `json:"screenIds,omitempty"`
}

// DefaultDropZoneSettings returns the default drop-zone configuration.
func DefaultDropZoneSettings() DropZoneSettings {
	return DropZoneSettings{
		Enabled:    true,
		Style:      DropZoneStyleNotch,
		ScreenMode: DropZoneScreenPrimary,
	}
}

// NormalizeDropZoneSettings validates and fills defaults for DropZoneSettings.
func NormalizeDropZoneSettings(in DropZoneSettings) DropZoneSettings {
	out := DefaultDropZoneSettings()
	out.Enabled = in.Enabled

	switch strings.ToLower(strings.TrimSpace(in.Style)) {
	case DropZoneStyleNotch:
		out.Style = DropZoneStyleNotch
	case DropZoneStyleCapsule:
		out.Style = DropZoneStyleCapsule
	}

	switch strings.ToLower(strings.TrimSpace(in.ScreenMode)) {
	case DropZoneScreenPrimary:
		out.ScreenMode = DropZoneScreenPrimary
	case DropZoneScreenAll:
		out.ScreenMode = DropZoneScreenAll
	case DropZoneScreenCustom:
		out.ScreenMode = DropZoneScreenCustom
	}

	if len(in.ScreenIDs) > 0 {
		seen := make(map[int64]struct{}, len(in.ScreenIDs))
		for _, id := range in.ScreenIDs {
			if id == 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out.ScreenIDs = append(out.ScreenIDs, id)
		}
	}
	return out
}

// GetDropZoneSettings returns the persisted drop-zone settings, or defaults.
func (s *Service) GetDropZoneSettings() DropZoneSettings {
	if s.mgr == nil || s.mgr.userData == nil {
		return DefaultDropZoneSettings()
	}
	return s.mgr.userData.GetDropZoneSettings()
}

// SetDropZoneSettings validates, persists, and broadcasts drop-zone settings.
func (s *Service) SetDropZoneSettings(settings DropZoneSettings) error {
	norm := NormalizeDropZoneSettings(settings)
	if s.mgr != nil && s.mgr.userData != nil {
		if err := s.mgr.userData.SetDropZoneSettings(norm); err != nil {
			return err
		}
	}
	s.emit(DropZoneSettingsChangedEvent, norm)
	return nil
}
