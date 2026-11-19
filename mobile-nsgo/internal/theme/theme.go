// Package theme loads the same JSON colour themes the desktop app uses.
//
// A theme file is named by its ID and declares whether it is light or dark with
// a -light / -dark suffix, as in default-dark.json. The built-in presets are
// embedded in the binary and can never be removed; user themes live in
// <app data>/themes. The token names, merge and fallback rules match the
// desktop's internal/theme package so one palette works on both clients.
package theme

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
)

// Appearance is the light or dark mode a theme describes.
type Appearance string

const (
	Light Appearance = "light"
	Dark  Appearance = "dark"
)

// Colors holds every colour token the app paints with. Values are CSS-like
// #rgb, #rrggbb and #rrggbbaa strings. The first block is the desktop's theme
// schema; Success is the one mobile-only addition (the desktop has no green
// semantic role) and older desktop themes fall back to the built-in value.
type Colors struct {
	Window     string `json:"window"`
	Background string `json:"background"`
	Sidebar    string `json:"sidebar"`
	Surface    string `json:"surface"`
	SurfaceAlt string `json:"surfaceAlt"`
	Text       string `json:"text"`
	TextMuted  string `json:"textMuted"`
	TextFaint  string `json:"textFaint"`
	Border     string `json:"border"`
	Hover      string `json:"hover"`
	Pressed    string `json:"pressed"`
	Accent     string `json:"accent"`
	OnAccent   string `json:"onAccent"`
	Danger     string `json:"danger"`
	Warning    string `json:"warning"`
	Info       string `json:"info"`
	Star       string `json:"star"`
	StarStrong string `json:"starStrong"`
	Shadow     string `json:"shadow"`
	Overlay    string `json:"overlay"`
	Preview    string `json:"preview"`
	Badge      string `json:"badge"`
	OnBadge    string `json:"onBadge"`
	Success    string `json:"success"`
}

// DefaultButtonRadius is the default corner radius for mobile buttons (full pill/circle).
const DefaultButtonRadius float32 = 999

// Material holds the desktop's blur parameters and optional mobile/desktop
// radius tokens. The mobile app parses blur fields so a shared theme file
// validates, and uses ButtonRadius to unify button corner radii.
type Material struct {
	SidebarOpacity float32  `json:"sidebarOpacity"`
	HeaderTint     string   `json:"headerTint"`
	HeaderBlur     float32  `json:"headerBlurRadius"`
	ButtonRadius   *float32 `json:"buttonRadius,omitempty"`
}

// ButtonRadius returns the configured button corner radius, falling back to
// DefaultButtonRadius (full-round) when unset.
func (t *Theme) ButtonRadius() float32 {
	if t != nil && t.Material.ButtonRadius != nil && *t.Material.ButtonRadius >= 0 {
		return *t.Material.ButtonRadius
	}
	return DefaultButtonRadius
}

// FormatButtonRadius formats r as a token string for core.Theme.Tokens.
func FormatButtonRadius(r float32) string {
	return strconv.FormatFloat(float64(r), 'f', -1, 32)
}

// Theme is one colour variant, loaded from one JSON file.
type Theme struct {
	ID         string     `json:"-"`
	Appearance Appearance `json:"-"`
	Builtin    bool       `json:"-"`
	Colors     Colors     `json:"colors"`
	Material   Material   `json:"material"`
}

//go:embed presets/*.json
var presets embed.FS

// IDFor returns the file stem used as a theme's ID, without the .json suffix.
func IDFor(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".json")
}

// AppearanceFor derives the appearance from an ID's -light / -dark suffix.
func AppearanceFor(id string) (Appearance, error) {
	switch {
	case strings.HasSuffix(id, "-light"):
		return Light, nil
	case strings.HasSuffix(id, "-dark"):
		return Dark, nil
	default:
		return "", fmt.Errorf("theme %q: file name must end with -light or -dark", id)
	}
}

// BuiltinIDs are reserved for the embedded defaults. User files with these IDs
// are ignored so the shipped palette can never be replaced or damaged.
var BuiltinIDs = map[string]bool{
	"default-light": true,
	"default-dark":  true,
	"sand-light":    true,
	"midnight-dark": true,
}

// DefaultIDs are the embedded defaults for each appearance.
const (
	DefaultLight = "default-light"
	DefaultDark  = "default-dark"
)

// DefaultID returns the embedded default for appearance.
func DefaultID(appearance Appearance) string {
	if appearance == Dark {
		return DefaultDark
	}
	return DefaultLight
}

// Default returns the embedded theme for appearance, used as a last-resort
// palette when the theme index is empty.
func Default(appearance Appearance) *Theme {
	id := DefaultID(appearance)
	raw, err := presets.ReadFile("presets/" + id + ".json")
	if err != nil {
		return &Theme{ID: id, Appearance: appearance, Builtin: true}
	}
	t, err := Parse(id, raw)
	if err != nil {
		return &Theme{ID: id, Appearance: appearance, Builtin: true}
	}
	t.Builtin = true
	return t
}

// Builtins returns the embedded default themes.
func Builtins() ([]*Theme, error) {
	entries, err := presets.ReadDir("presets")
	if err != nil {
		return nil, err
	}
	out := make([]*Theme, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := presets.ReadFile("presets/" + entry.Name())
		if err != nil {
			return nil, err
		}
		t, err := Parse(IDFor(entry.Name()), raw)
		if err != nil {
			return nil, err
		}
		t.Builtin = true
		out = append(out, t)
	}
	sortThemes(out)
	return out, nil
}

// Parse reads one theme file. The ID decides the appearance and must end with
// -light or -dark.
func Parse(id string, raw []byte) (*Theme, error) {
	appearance, err := AppearanceFor(id)
	if err != nil {
		return nil, err
	}
	var t Theme
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("theme %q: %w", id, err)
	}
	t.ID, t.Appearance = id, appearance
	return &t, nil
}

// Load returns the built-in themes followed by the valid user themes in dir.
// Missing tokens in a user theme fall back to the matching built-in variant,
// and invalid files are skipped rather than taking the UI down.
func Load(dir string) []*Theme {
	builtins, err := Builtins()
	if err != nil {
		return nil
	}
	byID := make(map[string]*Theme, len(builtins))
	for _, t := range builtins {
		byID[t.ID] = t
	}
	out := append([]*Theme(nil), builtins...)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := IDFor(entry.Name())
		if BuiltinIDs[id] {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		t, err := Parse(id, raw)
		if err != nil {
			log.Printf("[Theme] skipping %s: %v", entry.Name(), err)
			continue
		}
		base := byID[DefaultLight]
		if t.Appearance == Dark {
			base = byID[DefaultDark]
		}
		t = Merge(base, t)
		if err := t.Validate(); err != nil {
			log.Printf("[Theme] skipping %s: %v", entry.Name(), err)
			continue
		}
		out = append(out, t)
	}
	sortThemes(out)
	return out
}

// Merge fills tokens missing from t with the values of base, so hand-written
// themes only need the colours they want to change.
func Merge(base, t *Theme) *Theme {
	if base == nil || t == nil {
		return t
	}
	c, b := t.Colors, base.Colors
	c.Window = or(c.Window, b.Window)
	c.Background = or(c.Background, b.Background)
	c.Sidebar = or(c.Sidebar, b.Sidebar)
	c.Surface = or(c.Surface, b.Surface)
	c.SurfaceAlt = or(c.SurfaceAlt, b.SurfaceAlt)
	c.Text = or(c.Text, b.Text)
	c.TextMuted = or(c.TextMuted, b.TextMuted)
	c.TextFaint = or(c.TextFaint, b.TextFaint)
	c.Border = or(c.Border, b.Border)
	c.Hover = or(c.Hover, b.Hover)
	c.Pressed = or(c.Pressed, b.Pressed)
	c.Accent = or(c.Accent, b.Accent)
	c.OnAccent = or(c.OnAccent, b.OnAccent)
	c.Danger = or(c.Danger, b.Danger)
	c.Warning = or(c.Warning, b.Warning)
	c.Info = or(c.Info, b.Info)
	c.Star = or(c.Star, b.Star)
	c.StarStrong = or(c.StarStrong, b.StarStrong)
	c.Shadow = or(c.Shadow, b.Shadow)
	c.Overlay = or(c.Overlay, b.Overlay)
	c.Preview = or(c.Preview, b.Preview)
	c.Badge = or(c.Badge, b.Badge)
	c.OnBadge = or(c.OnBadge, b.OnBadge)
	c.Success = or(c.Success, b.Success)
	t.Colors = c
	if t.Material == (Material{}) {
		t.Material = base.Material
	} else {
		if t.Material.HeaderTint == "" {
			t.Material.HeaderTint = base.Material.HeaderTint
		}
		if t.Material.ButtonRadius == nil {
			t.Material.ButtonRadius = base.Material.ButtonRadius
		}
	}
	return t
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func sortThemes(themes []*Theme) {
	sort.SliceStable(themes, func(i, j int) bool {
		if themes[i].Builtin != themes[j].Builtin {
			return themes[i].Builtin
		}
		if themes[i].Appearance != themes[j].Appearance {
			return themes[i].Appearance == Light
		}
		return themes[i].ID < themes[j].ID
	})
}

// ParseColor parses a theme colour token, accepting #rgb, #rrggbb and
// #rrggbbaa.
func ParseColor(s string) (core.Color, error) {
	s = strings.TrimSpace(s)
	switch {
	case len(s) == 4 && s[0] == '#':
		s = "#" + string([]byte{s[1], s[1], s[2], s[2], s[3], s[3]})
	case len(s) == 5 && s[0] == '#':
		s = "#" + string([]byte{s[1], s[1], s[2], s[2], s[3], s[3], s[4], s[4]})
	}
	if len(s) != 7 && len(s) != 9 {
		return 0, fmt.Errorf("invalid colour %q", s)
	}
	if s[0] != '#' {
		return 0, fmt.Errorf("invalid colour %q", s)
	}
	return core.ParseColor(s)
}

// FormatColor writes a colour as #rrggbb, or #rrggbbaa when translucent.
func FormatColor(c core.Color) string {
	r, g, b, a := c.Components()
	if a >= 0.999 {
		return fmt.Sprintf("#%02x%02x%02x", uint8(r*255+0.5), uint8(g*255+0.5), uint8(b*255+0.5))
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", uint8(r*255+0.5), uint8(g*255+0.5), uint8(b*255+0.5), uint8(a*255+0.5))
}

// Validate reports the first colour token that cannot be parsed.
func (t *Theme) Validate() error {
	for name, value := range map[string]string{
		"window": t.Colors.Window, "background": t.Colors.Background,
		"sidebar": t.Colors.Sidebar, "surface": t.Colors.Surface,
		"surfaceAlt": t.Colors.SurfaceAlt, "text": t.Colors.Text,
		"textMuted": t.Colors.TextMuted, "textFaint": t.Colors.TextFaint,
		"border": t.Colors.Border, "hover": t.Colors.Hover,
		"pressed": t.Colors.Pressed, "accent": t.Colors.Accent,
		"onAccent": t.Colors.OnAccent, "danger": t.Colors.Danger,
		"warning": t.Colors.Warning, "info": t.Colors.Info,
		"star": t.Colors.Star, "starStrong": t.Colors.StarStrong,
		"shadow": t.Colors.Shadow, "overlay": t.Colors.Overlay,
		"preview": t.Colors.Preview, "badge": t.Colors.Badge,
		"onBadge": t.Colors.OnBadge, "success": t.Colors.Success,
	} {
		if _, err := ParseColor(value); err != nil {
			return fmt.Errorf("theme %q: %s: %w", t.ID, name, err)
		}
	}
	if _, err := ParseColor(t.Material.HeaderTint); err != nil {
		return fmt.Errorf("theme %q: headerTint: %w", t.ID, err)
	}
	return nil
}
