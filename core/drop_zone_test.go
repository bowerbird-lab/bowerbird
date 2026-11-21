package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeDropZoneSettings(t *testing.T) {
	def := DefaultDropZoneSettings()
	if !def.Enabled || def.Style != DropZoneStyleNotch || def.ScreenMode != DropZoneScreenPrimary {
		t.Fatalf("unexpected default settings: %+v", def)
	}

	got := NormalizeDropZoneSettings(DropZoneSettings{
		Enabled:    false,
		Style:      " CAPSULE ",
		ScreenMode: " CUSTOM ",
		ScreenIDs:  []int64{0, 101, 202, 101, 303},
	})
	if got.Enabled {
		t.Fatalf("expected Enabled=false")
	}
	if got.Style != DropZoneStyleCapsule {
		t.Fatalf("Style = %q, want %q", got.Style, DropZoneStyleCapsule)
	}
	if got.ScreenMode != DropZoneScreenCustom {
		t.Fatalf("ScreenMode = %q, want %q", got.ScreenMode, DropZoneScreenCustom)
	}
	if len(got.ScreenIDs) != 3 || got.ScreenIDs[0] != 101 || got.ScreenIDs[1] != 202 || got.ScreenIDs[2] != 303 {
		t.Fatalf("ScreenIDs = %v, want [101 202 303]", got.ScreenIDs)
	}

	fallback := NormalizeDropZoneSettings(DropZoneSettings{
		Enabled:    true,
		Style:      "invalid",
		ScreenMode: "invalid",
	})
	if fallback.Style != DropZoneStyleNotch || fallback.ScreenMode != DropZoneScreenPrimary {
		t.Fatalf("expected invalid values to fall back to defaults, got %+v", fallback)
	}
}

func TestUserDataStoreDropZoneSettingsPersistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWERBIRD_USER_DATA_DIR", dir)

	store, err := NewUserDataStore()
	if err != nil {
		t.Fatalf("NewUserDataStore: %v", err)
	}
	if got := store.GetDropZoneSettings(); got.Style != DropZoneStyleNotch || !got.Enabled {
		t.Fatalf("initial GetDropZoneSettings = %+v", got)
	}

	want := DropZoneSettings{
		Enabled:    true,
		Style:      DropZoneStyleCapsule,
		ScreenMode: DropZoneScreenCustom,
		ScreenIDs:  []int64{42, 99},
	}
	if err := store.SetDropZoneSettings(want); err != nil {
		t.Fatalf("SetDropZoneSettings: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "user_data.json")); err != nil {
		t.Fatalf("user_data.json not created: %v", err)
	}

	reloaded, err := NewUserDataStore()
	if err != nil {
		t.Fatalf("reload NewUserDataStore: %v", err)
	}
	got := reloaded.GetDropZoneSettings()
	if got.Enabled != want.Enabled || got.Style != want.Style || got.ScreenMode != want.ScreenMode || len(got.ScreenIDs) != 2 || got.ScreenIDs[0] != 42 || got.ScreenIDs[1] != 99 {
		t.Fatalf("reloaded = %+v, want %+v", got, want)
	}
}
