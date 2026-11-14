package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinsAreProtectedAndValid(t *testing.T) {
	builtins, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if len(builtins) != 2 {
		t.Fatalf("builtins = %d, want 2", len(builtins))
	}
	for _, th := range builtins {
		if !th.Builtin {
			t.Fatalf("%s is not marked built-in", th.ID)
		}
		if err := th.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppearanceComesFromTheID(t *testing.T) {
	cases := map[string]Appearance{
		"nord-light":    Light,
		"nord-dark":     Dark,
		"default-light": Light,
	}
	for id, want := range cases {
		got, err := AppearanceFor(id)
		if err != nil || got != want {
			t.Fatalf("AppearanceFor(%q) = %q, %v; want %q", id, got, err, want)
		}
	}
	if _, err := AppearanceFor("nord"); err == nil {
		t.Fatal("an ID without a suffix was accepted")
	}
}

func TestUserThemesMergeAndProtectedIDsAreIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine-dark.json"), []byte(`{"colors":{"accent":"#ff0000"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A user file with a built-in ID must not replace the shipped palette.
	if err := os.WriteFile(filepath.Join(dir, "default-dark.json"), []byte(`{"colors":{"background":"#000000"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken-light.json"), []byte(`{"colors":{"accent":"nope"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	themes := Load(dir)
	var mine, def *Theme
	for _, th := range themes {
		switch th.ID {
		case "mine-dark":
			mine = th
		case "default-dark":
			def = th
		}
	}
	if mine == nil {
		t.Fatal("valid user theme was not loaded")
	}
	if mine.Colors.Accent != "#ff0000" {
		t.Fatalf("accent = %q, want the user value", mine.Colors.Accent)
	}
	if mine.Colors.Background != "#141416" {
		t.Fatalf("missing token did not fall back: background = %q", mine.Colors.Background)
	}
	if def == nil || def.Colors.Background != "#141416" {
		t.Fatal("a user file replaced the protected default-dark theme")
	}
	for _, th := range themes {
		if th.ID == "broken-light" {
			t.Fatal("invalid user theme was kept")
		}
	}
}

func TestColorFormatRoundTrips(t *testing.T) {
	for _, raw := range []string{"#ffffff", "#00000059", "#6ea8ff", "#1234"} {
		col, err := ParseColor(raw)
		if err != nil {
			t.Fatalf("ParseColor(%q): %v", raw, err)
		}
		got := FormatColor(col)
		back, err := ParseColor(got)
		if err != nil || back != col {
			t.Fatalf("round trip %q -> %q -> %v", raw, got, err)
		}
	}
	if _, err := ParseColor("red"); err == nil {
		t.Fatal("invalid colour was accepted")
	}
}
