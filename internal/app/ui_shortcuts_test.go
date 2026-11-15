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

func TestAcceleratorFromUIRoundTrips(t *testing.T) {
	cases := []struct {
		mods ui.Modifiers
		key  ui.Key
		want string
	}{
		{ui.Cmd, ui.KeyK, "CmdOrCtrl+K"},
		{ui.Cmd | ui.Shift, ui.KeyF11, "CmdOrCtrl+Shift+F11"},
		{0, ui.KeyDelete, "Delete"},
		{ui.Alt, ui.KeyEqual, "Alt+="},
		{ui.Cmd | ui.Alt | ui.Shift, ui.KeySpace, "CmdOrCtrl+Alt+Shift+Space"},
		{0, ui.KeyComma, ","},
	}
	for _, tc := range cases {
		got, ok := acceleratorFromUI(tc.mods, tc.key)
		if !ok || got != tc.want {
			t.Errorf("acceleratorFromUI(%v, %v) = (%q, %v), want %q", tc.mods, tc.key, got, ok, tc.want)
			continue
		}
		// The recorder's output must be what the handlers parse back.
		mods, key, ok := uiShortcutKeys(got)
		if !ok || mods != tc.mods || key != tc.key {
			t.Errorf("uiShortcutKeys(%q) = (%v, %v, %v), want (%v, %v)", got, mods, key, ok, tc.mods, tc.key)
		}
	}
	if _, ok := acceleratorFromUI(ui.Cmd, ui.KeyUnknown); ok {
		t.Error("a modifier alone must not record a shortcut")
	}
}

func TestShortcutPreviewShowsHeldModifiers(t *testing.T) {
	if got := shortcutPreviewAccel(0, ""); got != "" {
		t.Errorf("preview with nothing held = %q, want empty", got)
	}
	if got, want := shortcutPreviewAccel(ui.Cmd|ui.Shift, ""), "CmdOrCtrl+Shift+…"; got != want {
		t.Errorf("held modifiers preview = %q, want %q", got, want)
	}
	if got, want := shortcutPreviewAccel(ui.Alt, "CmdOrCtrl+K"), "CmdOrCtrl+K"; got != want {
		t.Errorf("recorded preview = %q, want %q", got, want)
	}
}
