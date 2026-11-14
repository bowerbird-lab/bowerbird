package app

import (
	"runtime"
	"testing"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

func TestUIShortcutKeysReadsConfiguredAccelerators(t *testing.T) {
	settings := core.DefaultShortcutSettings()

	mods, key, ok := uiShortcutKeys(settings.Accelerator(core.ShortcutOpenSettings))
	if !ok {
		t.Fatal("open-settings accelerator did not map to a key")
	}
	wantMods := ui.Cmd
	wantKey := ui.KeyI
	if runtime.GOOS == "darwin" {
		wantKey = ui.KeyComma
	}
	if mods != wantMods || key != wantKey {
		t.Fatalf("settings shortcut = (%v, %v), want (%v, %v)", mods, key, wantMods, wantKey)
	}

	mods, key, ok = uiShortcutKeys(settings.Accelerator(core.ShortcutLogs))
	if !ok || mods != ui.Cmd || key != ui.KeyL {
		t.Fatalf("logs shortcut = (%v, %v, %v), want Cmd+L", mods, key, ok)
	}
}

func TestUIShortcutKeysHandlesCustomBindings(t *testing.T) {
	mods, key, ok := uiShortcutKeys("CmdOrCtrl+Shift+F11")
	if !ok || mods != ui.Cmd|ui.Shift || key != ui.KeyF11 {
		t.Fatalf("got (%v, %v, %v), want Ctrl/Command+Shift+F11", mods, key, ok)
	}
	if _, _, ok := uiShortcutKeys(""); ok {
		t.Fatal("empty accelerator must not match")
	}
}

// TestEveryCatalogShortcutHasUIKeys guards the settings list against drift:
// every entry the list shows must resolve to a key the handlers can match.
func TestEveryCatalogShortcutHasUIKeys(t *testing.T) {
	settings := core.DefaultShortcutSettings()
	for _, def := range core.ShortcutCatalog() {
		if _, _, ok := uiShortcutKeys(settings.Accelerator(def.ID)); !ok {
			t.Errorf("%q (%q) does not map to a ui key", def.ID, settings.Accelerator(def.ID))
		}
	}
}

func TestDeleteShortcutsMapToDeleteKeys(t *testing.T) {
	settings := core.DefaultShortcutSettings()
	for _, tc := range []struct {
		id   string
		want ui.Key
	}{
		{core.ShortcutDelete, ui.KeyDelete},
		{core.ShortcutDeleteBackspace, ui.KeyBackspace},
	} {
		mods, key, ok := uiShortcutKeys(settings.Accelerator(tc.id))
		if !ok || mods != 0 || key != tc.want {
			t.Errorf("%q = (%v, %v, %v), want (0, %v, true)", tc.id, mods, key, ok, tc.want)
		}
	}
}
