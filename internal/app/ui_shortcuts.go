package app

import (
	"fmt"
	"strings"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

// Windows has no native menu bar in this build, so the app menu's accelerators
// never reach the OS. In-app shortcuts therefore have to be read from the same
// ShortcutSettings the Settings window edits; hardcoding them made Ctrl+I and
// Ctrl+L (the Windows defaults) do nothing while the UI advertised other keys.
func (a *App) shortcutSettings() core.ShortcutSettings {
	if a.svc != nil {
		return a.svc.GetShortcutSettings()
	}
	return core.DefaultShortcutSettings()
}

func (a *App) shortcutMatched(c *ui.Context, id string) bool {
	mods, key, ok := a.shortcutKeys(id)
	if !ok {
		return false
	}
	return c.Shortcut(mods, key)
}

// uiShortcut is one catalog entry resolved to the values ui.Context.Shortcut
// compares against.
type uiShortcut struct {
	mods ui.Modifiers
	key  ui.Key
	ok   bool
}

// rebuildShortcutPairs resolves the whole catalog once per settings change. The
// main window checks two dozen bindings every frame, so reading and
// normalizing the settings map inside each check was pure overhead.
func (a *App) rebuildShortcutPairs() {
	settings := a.shortcutSettings()
	pairs := make(map[string]uiShortcut, len(core.ShortcutCatalog()))
	for _, def := range core.ShortcutCatalog() {
		mods, key, ok := uiShortcutKeys(settings.Accelerator(def.ID))
		pairs[def.ID] = uiShortcut{mods: mods, key: key, ok: ok}
	}
	a.shortcutPairs = pairs
}

// overlayShortcut runs fn when the overlay owns id's key. A modal overlay
// makes the window's shortcuts wait (ui.Context.Shortcut returns false behind
// a dialog), so the keys the preview must keep working are registered on the
// overlay itself instead of in handleMainShortcuts.
func (a *App) overlayShortcut(overlay ui.Element, id string, fn func()) {
	pair, ok := a.shortcutPairs[id]
	if !ok || !pair.ok {
		return
	}
	if overlay.OverlayShortcut(pair.mods, pair.key) {
		fn()
	}
}

// shortcutKeys resolves one catalog entry, fixed or not, to the modifiers and
// key ui.Shortcut compares against. Every handler goes through it so the
// binding Settings lists is the binding the window answers to.
func (a *App) shortcutKeys(id string) (ui.Modifiers, ui.Key, bool) {
	if pair, ok := a.shortcutPairs[id]; ok {
		return pair.mods, pair.key, pair.ok
	}
	return uiShortcutKeys(a.shortcutSettings().Accelerator(id))
}

// acceleratorFromUI turns a recorded key press into the catalog's accelerator
// syntax, the inverse of uiShortcutKeys. A modifier pressed on its own has no
// key name yet and records nothing.
func acceleratorFromUI(mods ui.Modifiers, key ui.Key) (string, bool) {
	name, ok := uiKeyName(key)
	if !ok {
		return "", false
	}
	return strings.Join(append(acceleratorModifiers(mods), name), "+"), true
}

// acceleratorModifiers renders the modifiers of a combination in catalog
// order.
func acceleratorModifiers(mods ui.Modifiers) []string {
	parts := make([]string, 0, 3)
	// ui.Cmd is Command on macOS and Control elsewhere; ui.Ctrl is the same
	// bit as ui.Cmd on Windows and Linux, so it only adds a part on macOS.
	if mods&ui.Cmd != 0 {
		parts = append(parts, "CmdOrCtrl")
	} else if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return parts
}

// shortcutPreviewAccel is what the recorder shows: the combination recorded
// last, or the modifiers held right now while no key has completed one, with
// an ellipsis standing for the key still missing.
func shortcutPreviewAccel(held ui.Modifiers, draft string) string {
	if draft != "" {
		return draft
	}
	parts := acceleratorModifiers(held)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(append(parts, "…"), "+")
}

// uiKeyName is the catalog's name for key, the inverse of uiKeyForName.
func uiKeyName(key ui.Key) (string, bool) {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + rune(key-ui.KeyA))), true
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + rune(key-ui.Key0))), true
	case key >= ui.KeyF1 && key <= ui.KeyF12:
		return fmt.Sprintf("F%d", int(key-ui.KeyF1)+1), true
	}
	switch key {
	case ui.KeyComma:
		return ",", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyPeriod:
		return ".", true
	case ui.KeySlash:
		return "/", true
	case ui.KeySemicolon:
		return ";", true
	case ui.KeyQuote:
		return "'", true
	case ui.KeyBracketLeft:
		return "[", true
	case ui.KeyBracketRight:
		return "]", true
	case ui.KeyBackslash:
		return `\`, true
	case ui.KeyBackquote:
		return "`", true
	case ui.KeySpace:
		return "Space", true
	case ui.KeyEscape:
		return "Escape", true
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyInsert:
		return "Insert", true
	case ui.KeyHome:
		return "Home", true
	case ui.KeyEnd:
		return "End", true
	case ui.KeyPageUp:
		return "PageUp", true
	case ui.KeyPageDown:
		return "PageDown", true
	case ui.KeyUp:
		return "Up", true
	case ui.KeyDown:
		return "Down", true
	case ui.KeyLeft:
		return "Left", true
	case ui.KeyRight:
		return "Right", true
	}
	return "", false
}

// uiShortcutKeys converts a stored accelerator such as "CmdOrCtrl+Shift+L"
// into the modifiers and key ui.Shortcut compares against.
func uiShortcutKeys(accel string) (ui.Modifiers, ui.Key, bool) {
	normalized := core.NormalizeAccelerator(accel)
	if normalized == "" {
		return 0, 0, false
	}
	parts := strings.Split(normalized, "+")
	if len(parts) == 0 {
		return 0, 0, false
	}
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "cmdorctrl":
			mods |= ui.Cmd
		case "ctrl":
			mods |= ui.Ctrl
		case "alt":
			mods |= ui.Alt
		case "shift":
			mods |= ui.Shift
		}
	}
	key, ok := uiKeyForName(parts[len(parts)-1])
	return mods, key, ok
}

func uiKeyForName(name string) (ui.Key, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ui.KeyUnknown, false
	}
	if len([]rune(name)) == 1 {
		r := []rune(name)[0]
		switch {
		case r >= 'a' && r <= 'z':
			return ui.KeyA + ui.Key(r-'a'), true
		case r >= 'A' && r <= 'Z':
			return ui.KeyA + ui.Key(r-'A'), true
		case r >= '0' && r <= '9':
			return ui.Key0 + ui.Key(r-'0'), true
		}
	}
	switch strings.ToLower(name) {
	case ",":
		return ui.KeyComma, true
	case "=":
		return ui.KeyEqual, true
	case "-":
		return ui.KeyMinus, true
	case ".":
		return ui.KeyPeriod, true
	case "/":
		return ui.KeySlash, true
	case ";":
		return ui.KeySemicolon, true
	case "'":
		return ui.KeyQuote, true
	case "[":
		return ui.KeyBracketLeft, true
	case "]":
		return ui.KeyBracketRight, true
	case "\\":
		return ui.KeyBackslash, true
	case "`":
		return ui.KeyBackquote, true
	case "space":
		return ui.KeySpace, true
	case "escape", "esc":
		return ui.KeyEscape, true
	case "enter", "return":
		return ui.KeyEnter, true
	case "tab":
		return ui.KeyTab, true
	case "backspace":
		return ui.KeyBackspace, true
	case "delete", "del":
		return ui.KeyDelete, true
	case "insert":
		return ui.KeyInsert, true
	case "home":
		return ui.KeyHome, true
	case "end":
		return ui.KeyEnd, true
	case "pageup", "page-up":
		return ui.KeyPageUp, true
	case "pagedown", "page-down":
		return ui.KeyPageDown, true
	case "up", "arrowup":
		return ui.KeyUp, true
	case "down", "arrowdown":
		return ui.KeyDown, true
	case "left", "arrowleft":
		return ui.KeyLeft, true
	case "right", "arrowright":
		return ui.KeyRight, true
	}
	if len(name) >= 2 && (name[0] == 'f' || name[0] == 'F') {
		n := 0
		for _, r := range name[1:] {
			if r < '0' || r > '9' {
				return ui.KeyUnknown, false
			}
			n = n*10 + int(r-'0')
		}
		if n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1), true
		}
	}
	return ui.KeyUnknown, false
}
