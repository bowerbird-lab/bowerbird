package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinsValidateAndCoverBothAppearances(t *testing.T) {
	themes, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) < 4 {
		t.Fatalf("builtins = %d, want at least 4", len(themes))
	}
	seenLight, seenDark := false, false
	for _, th := range themes {
		if !th.Builtin {
			t.Fatalf("%s is not marked builtin", th.ID)
		}
		if err := th.Validate(); err != nil {
			t.Fatalf("%s: %v", th.ID, err)
		}
		if th.Appearance == Light {
			seenLight = true
		} else {
			seenDark = true
		}
		if th.Colors.Success == "" {
			t.Fatalf("%s has no success token", th.ID)
		}
	}
	if !seenLight || !seenDark {
		t.Fatalf("appearances = light:%v dark:%v", seenLight, seenDark)
	}
}

func TestLoadMergesDesktopThemeMissingMobileTokens(t *testing.T) {
	dir := t.TempDir()
	// A desktop theme file: it has no mobile-only success token.
	body := `{"colors":{"background":"#101010","surface":"#202020","text":"#ffffff"},"material":{"headerTint":"#000000cc"}}`
	if err := os.WriteFile(filepath.Join(dir, "mine-dark.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	themes := Load(dir)
	var mine *Theme
	for _, th := range themes {
		if th.ID == "mine-dark" {
			mine = th
		}
	}
	if mine == nil {
		t.Fatal("mine-dark was not loaded")
	}
	if mine.Colors.Background != "#101010" || mine.Colors.Accent == "" || mine.Colors.Success == "" {
		t.Fatalf("merged colours = %+v", mine.Colors)
	}
	if mine.Builtin || mine.Appearance != Dark {
		t.Fatalf("appearance = %v builtin = %v", mine.Appearance, mine.Builtin)
	}
	if err := mine.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadIgnoresInvalidAndBuiltinCollisions(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "default-dark.json"), []byte(`{"colors":{"background":"#000000"}}`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "broken-light.json"), []byte(`{"colors":{"background":"nope"}}`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "bad-name.json"), []byte(`{}`), 0o600)
	for _, th := range Load(dir) {
		if th.ID == "broken-light" || th.ID == "bad-name" {
			t.Fatalf("invalid theme %q was loaded", th.ID)
		}
		if th.ID == "default-dark" && !th.Builtin {
			t.Fatal("user file replaced a builtin")
		}
	}
}

func TestParseColorAcceptsShorthandAndAlpha(t *testing.T) {
	for _, raw := range []string{"#fff", "#ffffff", "#ffff", "#ffffffcc", "#ffffff80"} {
		if _, err := ParseColor(raw); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if _, err := ParseColor("white"); err == nil {
		t.Fatal("named colours should be rejected to keep theme files portable")
	}
}

func TestFormatColorRoundTrip(t *testing.T) {
	c, err := ParseColor("#123456cc")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatColor(c); got != "#123456cc" {
		t.Fatalf("FormatColor = %q", got)
	}
}

func TestDesktopThemeFolderLoadsUnchanged(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "internal", "theme", "presets")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("desktop theme folder not present")
	}
	seen := map[string]bool{}
	for _, th := range Load(dir) {
		if err := th.Validate(); err != nil {
			t.Fatalf("%s: %v", th.ID, err)
		}
		if th.Colors.Success == "" {
			t.Fatalf("%s: desktop theme did not inherit the mobile success token", th.ID)
		}
		seen[th.ID] = true
	}
	for _, id := range []string{"default-light", "default-dark", "sand-light", "midnight-dark"} {
		if !seen[id] {
			t.Fatalf("desktop theme %s did not load", id)
		}
	}
}
