package core

import (
	"fmt"
	"runtime"
	"strings"
	"unicode"
)

const ShortcutsChangedEvent = "shortcuts-changed"

// ShortcutScope is whether a binding is registered as a global OS hotkey
// or only while a Bowerbird window is focused.
type ShortcutScope = string

const (
	ShortcutScopeApp    ShortcutScope = "app"
	ShortcutScopeGlobal ShortcutScope = "global"
)

// Shortcut IDs for every customizable action.
const (
	ShortcutCaptureArea       = "capture-area"
	ShortcutCaptureFullscreen = "capture-fullscreen"
	ShortcutCaptureWindow     = "capture-window"
	ShortcutToggleMainWindow  = "toggle-main-window"
	ShortcutOpenSettings      = "settings"
	ShortcutLogs              = "logs"
	ShortcutReload            = "reload"
	ShortcutForceReload       = "force-reload"
	ShortcutFullscreen        = "fullscreen"
	ShortcutQuit              = "quit"
	ShortcutCloseWindow       = "close-window"
	ShortcutNewWindow         = "new-window"
	ShortcutZoomIn            = "zoom-in"
	ShortcutZoomOut           = "zoom-out"
	ShortcutZoomReset         = "zoom-reset"

	// ShortcutDevTools is not part of the catalog: the windows render native
	// content, so there is no web inspector to toggle. The app menu keeps the
	// action for the platforms that draw the ≡ menu.
	ShortcutDevTools = "devtools"
)

// Shortcut IDs for the fixed bindings. They are part of the keyboard map the
// app ships and cannot be rebound, but Settings lists them so the whole map is
// visible in one place.
const (
	ShortcutSearch          = "search"
	ShortcutCopy            = "copy"
	ShortcutSelectAll       = "select-all"
	ShortcutDismiss         = "dismiss"
	ShortcutTogglePreview   = "toggle-preview"
	ShortcutMoveLeft        = "move-left"
	ShortcutMoveRight       = "move-right"
	ShortcutMoveUp          = "move-up"
	ShortcutMoveDown        = "move-down"
	ShortcutToggleSidebar   = "toggle-sidebar"
	ShortcutToggleInspector = "toggle-inspector"
	// Delete and Backspace both remove the selection: on macOS the main
	// delete key reports as Backspace, so listing only Delete would make the
	// Settings list disagree with the keyboard users actually have.
	ShortcutDelete          = "delete"
	ShortcutDeleteBackspace = "delete-backspace"
)

// ShortcutDef is the catalog entry for one action. Scope tells whether the
// binding is registered as an OS hotkey or handled in-app. Fixed entries are
// listed only: their accelerator is always the default and cannot be changed.
type ShortcutDef struct {
	ID    string
	Scope ShortcutScope
	Fixed bool
}

// ShortcutCatalog lists every binding the app knows about, in display order:
// global hotkeys, rebindable in-app actions, then the fixed basic bindings.
func ShortcutCatalog() []ShortcutDef {
	return []ShortcutDef{
		{ID: ShortcutCaptureArea, Scope: ShortcutScopeGlobal},
		{ID: ShortcutCaptureFullscreen, Scope: ShortcutScopeGlobal},
		{ID: ShortcutCaptureWindow, Scope: ShortcutScopeGlobal},
		{ID: ShortcutToggleMainWindow, Scope: ShortcutScopeGlobal},

		{ID: ShortcutOpenSettings, Scope: ShortcutScopeApp},
		{ID: ShortcutLogs, Scope: ShortcutScopeApp},
		{ID: ShortcutReload, Scope: ShortcutScopeApp},
		{ID: ShortcutForceReload, Scope: ShortcutScopeApp},
		{ID: ShortcutFullscreen, Scope: ShortcutScopeApp},
		{ID: ShortcutQuit, Scope: ShortcutScopeApp},
		{ID: ShortcutCloseWindow, Scope: ShortcutScopeApp},
		{ID: ShortcutNewWindow, Scope: ShortcutScopeApp},
		{ID: ShortcutZoomIn, Scope: ShortcutScopeApp},
		{ID: ShortcutZoomOut, Scope: ShortcutScopeApp},
		{ID: ShortcutZoomReset, Scope: ShortcutScopeApp},

		{ID: ShortcutSearch, Fixed: true},
		{ID: ShortcutCopy, Fixed: true},
		{ID: ShortcutSelectAll, Fixed: true},
		{ID: ShortcutDismiss, Fixed: true},
		{ID: ShortcutTogglePreview, Fixed: true},
		{ID: ShortcutMoveLeft, Fixed: true},
		{ID: ShortcutMoveRight, Fixed: true},
		{ID: ShortcutMoveUp, Fixed: true},
		{ID: ShortcutMoveDown, Fixed: true},
		{ID: ShortcutToggleSidebar, Fixed: true},
		{ID: ShortcutToggleInspector, Fixed: true},
		{ID: ShortcutDelete, Fixed: true},
		{ID: ShortcutDeleteBackspace, Fixed: true},
	}
}

// ShortcutSection groups the catalog the way Settings shows it.
type ShortcutSection struct {
	// Key is the i18n key suffix: "global", "app" or "fixed".
	Key  string
	Defs []ShortcutDef
}

// ShortcutSections splits the catalog into the groups Settings renders.
func ShortcutSections() []ShortcutSection {
	sections := []ShortcutSection{
		{Key: "global"},
		{Key: "app"},
		{Key: "fixed"},
	}
	for _, def := range ShortcutCatalog() {
		switch {
		case def.Fixed:
			sections[2].Defs = append(sections[2].Defs, def)
		case def.Scope == ShortcutScopeGlobal:
			sections[0].Defs = append(sections[0].Defs, def)
		default:
			sections[1].Defs = append(sections[1].Defs, def)
		}
	}
	return sections
}

func shortcutDefFor(id string) (ShortcutDef, bool) {
	for _, def := range ShortcutCatalog() {
		if def.ID == id {
			return def, true
		}
	}
	return ShortcutDef{}, false
}

// IsFixedShortcut reports whether id is a basic binding that Settings only
// lists.
func IsFixedShortcut(id string) bool {
	def, ok := shortcutDefFor(id)
	return ok && def.Fixed
}

func ShortcutScopeFor(id string) ShortcutScope {
	if def, ok := shortcutDefFor(id); ok && def.Scope != "" {
		return def.Scope
	}
	return ShortcutScopeApp
}

func IsGlobalShortcut(id string) bool {
	return ShortcutScopeFor(id) == ShortcutScopeGlobal
}

// ShortcutSettings maps action id -> accelerator (e.g. "Alt+A", "CmdOrCtrl+,").
type ShortcutSettings struct {
	Bindings map[string]string `json:"bindings"`
}

// defaultShortcutBindings is the immutable table every settings value starts
// from. DefaultShortcutSettings hands out a copy so callers can mutate it.
var defaultShortcutBindings = func() map[string]string {
	settingsAccel := "CmdOrCtrl+I"
	if runtime.GOOS == "darwin" {
		settingsAccel = "CmdOrCtrl+,"
	}
	return map[string]string{
		ShortcutCaptureArea:       "Alt+A",
		ShortcutCaptureFullscreen: "Alt+S",
		ShortcutCaptureWindow:     "Alt+W",
		ShortcutToggleMainWindow:  "Alt+B",
		ShortcutOpenSettings:      settingsAccel,
		ShortcutLogs:              "CmdOrCtrl+L",
		ShortcutReload:            "CmdOrCtrl+R",
		ShortcutForceReload:       "CmdOrCtrl+Shift+R",
		ShortcutFullscreen:        "F11",
		ShortcutQuit:              "CmdOrCtrl+Q",
		ShortcutCloseWindow:       "CmdOrCtrl+W",
		ShortcutNewWindow:         "CmdOrCtrl+N",
		ShortcutZoomIn:            "CmdOrCtrl+=",
		ShortcutZoomOut:           "CmdOrCtrl+-",
		ShortcutZoomReset:         "CmdOrCtrl+0",

		// Fixed bindings: shown in Settings, never stored or changed.
		ShortcutSearch:          "CmdOrCtrl+F",
		ShortcutCopy:            "CmdOrCtrl+C",
		ShortcutSelectAll:       "CmdOrCtrl+A",
		ShortcutDismiss:         "Escape",
		ShortcutTogglePreview:   "Space",
		ShortcutMoveLeft:        "Left",
		ShortcutMoveRight:       "Right",
		ShortcutMoveUp:          "Up",
		ShortcutMoveDown:        "Down",
		ShortcutToggleSidebar:   "CmdOrCtrl+Shift+B",
		ShortcutToggleInspector: "CmdOrCtrl+Shift+I",
		ShortcutDelete:          "Delete",
		ShortcutDeleteBackspace: "Backspace",
	}
}()

func DefaultShortcutSettings() ShortcutSettings {
	bindings := make(map[string]string, len(defaultShortcutBindings))
	for id, accel := range defaultShortcutBindings {
		bindings[id] = accel
	}
	return ShortcutSettings{Bindings: bindings}
}

func NormalizeShortcutSettings(in ShortcutSettings) ShortcutSettings {
	out := DefaultShortcutSettings()
	if in.Bindings == nil {
		return out
	}
	for id, accel := range in.Bindings {
		// Unknown ids and fixed bindings are dropped: the former no longer
		// exist, the latter always keep their default.
		if _, ok := out.Bindings[id]; !ok || IsFixedShortcut(id) {
			continue
		}
		norm := NormalizeAccelerator(accel)
		// Empty is an explicit unset — keep it so the action has no shortcut.
		out.Bindings[id] = norm
	}
	return out
}

// ShortcutConflict returns the id of the other action that already uses accel,
// or "". It is the live check the recorder runs as keys come in, so a conflict
// shows before the user saves.
func ShortcutConflict(settings ShortcutSettings, id, accel string) string {
	norm := NormalizeAccelerator(accel)
	if id == "" || norm == "" {
		return ""
	}
	bindings := NormalizeShortcutSettings(settings).Bindings
	for other, value := range bindings {
		if other == id || value == "" {
			continue
		}
		if strings.EqualFold(value, norm) {
			return other
		}
	}
	return ""
}

func ValidateShortcutSettings(in ShortcutSettings) error {
	norm := NormalizeShortcutSettings(in)
	seen := map[string]string{}
	for id, accel := range norm.Bindings {
		if accel == "" {
			continue
		}
		key := strings.ToLower(accel)
		if other, ok := seen[key]; ok {
			return fmt.Errorf("shortcut conflict: %s and %s both use %s", other, id, accel)
		}
		seen[key] = id
	}
	return nil
}

func (s ShortcutSettings) Accelerator(id string) string {
	// Fixed bindings ignore the stored map, so old user data cannot override
	// a basic key and the Settings list cannot drift from the handlers.
	if IsFixedShortcut(id) {
		return defaultShortcutBindings[id]
	}
	if s.Bindings != nil {
		if v, ok := s.Bindings[id]; ok {
			return NormalizeAccelerator(v)
		}
	}
	return defaultShortcutBindings[id]
}

// NormalizeAccelerator canonicalizes an accelerator string.
func NormalizeAccelerator(accel string) string {
	accel = strings.TrimSpace(accel)
	if accel == "" {
		return ""
	}
	parts := strings.Split(accel, "+")
	var ctrl, alt, shift, meta bool
	var key string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		low := strings.ToLower(p)
		switch low {
		case "ctrl", "control":
			ctrl = true
		case "cmdorctrl", "cmd", "command", "meta", "super":
			meta = true
		case "alt", "option", "opt":
			alt = true
		case "shift":
			shift = true
		default:
			key = p
		}
	}
	if key == "" {
		return ""
	}
	if len(key) == 1 {
		r := []rune(key)[0]
		if unicode.IsLetter(r) {
			key = strings.ToUpper(key)
		}
	}
	lowKey := strings.ToLower(key)
	switch lowKey {
	case "cmdorctrl", "ctrl", "control", "cmd", "command", "meta", "super", "alt", "option", "shift":
		return ""
	case "comma":
		key = ","
	case "equal", "plus":
		key = "="
	case "minus":
		key = "-"
	}
	var out []string
	if meta {
		out = append(out, "CmdOrCtrl")
	}
	if ctrl && !meta {
		out = append(out, "Ctrl")
	}
	if alt {
		out = append(out, "Alt")
	}
	if shift {
		out = append(out, "Shift")
	}
	out = append(out, key)
	return strings.Join(out, "+")
}

// AcceleratorKeys splits an accelerator into the labels of its key caps, one
// per key: the modifiers first, then the key. The UI draws each as its own
// square, joined by plus signs.
func AcceleratorKeys(accel string) []string {
	accel = NormalizeAccelerator(accel)
	if accel == "" {
		return nil
	}
	darwin := runtime.GOOS == "darwin"
	parts := strings.Split(accel, "+")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		switch strings.ToLower(p) {
		case "cmdorctrl":
			if darwin {
				keys = append(keys, "⌘")
			} else {
				keys = append(keys, "Ctrl")
			}
		case "ctrl":
			if darwin {
				keys = append(keys, "⌃")
			} else {
				keys = append(keys, "Ctrl")
			}
		case "alt":
			if darwin {
				keys = append(keys, "⌥")
			} else {
				keys = append(keys, "Alt")
			}
		case "shift":
			if darwin {
				keys = append(keys, "⇧")
			} else {
				keys = append(keys, "Shift")
			}
		default:
			keys = append(keys, shortcutKeyDisplay(p))
		}
	}
	return keys
}

// AcceleratorDisplay returns a human-readable shortcut for the current OS:
// macOS joins its glyphs without separators, as its menus do.
func AcceleratorDisplay(accel string) string {
	keys := AcceleratorKeys(accel)
	if len(keys) == 0 {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return strings.Join(keys, "")
	}
	return strings.Join(keys, "+")
}

// AcceleratorForMyGo maps stored accelerators onto mygo's syntax.
func AcceleratorForMyGo(accel string) string {
	accel = NormalizeAccelerator(accel)
	accel = strings.ReplaceAll(accel, "CmdOrCtrl", "CmdOrCtrl")
	return accel
}

// shortcutKeyDisplay shortens the key names whose full spelling would crowd a
// menu row or a Settings badge.
func shortcutKeyDisplay(key string) string {
	switch strings.ToLower(key) {
	case "escape", "esc":
		return "Esc"
	case "space":
		return "Space"
	case "left":
		return "←"
	case "right":
		return "→"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "pageup", "page-up":
		return "Page ↑"
	case "pagedown", "page-down":
		return "Page ↓"
	}
	return key
}
