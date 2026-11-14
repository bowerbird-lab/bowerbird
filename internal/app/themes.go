package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"bowerbird/core"
	"bowerbird/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// themesDir is the user theme folder. Built-in defaults never live here, so
// the shipped palette always survives a user deleting or breaking a file.
func (a *App) themesDir() string {
	return filepath.Join(core.UserDataDir(), "themes")
}

// loadThemes scans the user theme folder and rebuilds both ready palettes.
func (a *App) loadThemes() {
	themes := theme.Load(a.themesDir())
	if len(themes) == 0 {
		// The embedded presets are the safety net; failing to read them is a
		// build error rather than something the UI can recover from.
		if builtins, err := theme.Builtins(); err == nil {
			themes = builtins
		} else {
			log.Printf("[Theme] no built-in themes: %v", err)
		}
	}
	a.themes = themes
	a.themeIndex = make(map[string]*theme.Theme, len(themes))
	for _, t := range themes {
		a.themeIndex[t.ID] = t
	}
	a.rebuildThemePalettes()
}

// rebuildThemePalettes resolves the selected light and dark themes into the
// palettes and ui.Themes the views use. It runs on the main thread only.
func (a *App) rebuildThemePalettes() {
	light := a.themeByID(a.prefs.LightTheme, theme.Light)
	dark := a.themeByID(a.prefs.DarkTheme, theme.Dark)
	a.paletteLight = paletteFor(light)
	a.paletteDark = paletteFor(dark)
	a.uiLight = uiThemeFor(a.paletteLight)
	a.uiDark = uiThemeFor(a.paletteDark)
}

// themeMode returns the stored colour mode; an empty value follows the system.
func (a *App) themeMode() string {
	if a.svc != nil {
		if mode := a.svc.GetTheme(); mode != "" {
			return mode
		}
	}
	return core.ThemeSystem
}

// isDark reports which palette the current mode and system appearance select.
func (a *App) isDark(c *ui.Context) bool {
	switch a.themeMode() {
	case core.ThemeDark:
		return true
	case core.ThemeLight:
		return false
	default:
		return c.Theme().Dark
	}
}

// themeByID finds a theme of the given appearance, falling back to the
// embedded default and then to any theme of that appearance.
func (a *App) themeByID(id string, appearance theme.Appearance) *theme.Theme {
	if t := a.themeIndex[id]; t != nil && t.Appearance == appearance {
		return t
	}
	if t := a.themeIndex["default-"+string(appearance)]; t != nil {
		return t
	}
	for _, t := range a.themes {
		if t.Appearance == appearance {
			return t
		}
	}
	// loadThemes always has the embedded presets; this keeps a zero App safe.
	return &theme.Theme{ID: "default-" + string(appearance), Appearance: appearance}
}

// palette returns the resolved colours for the current variant. It is cached,
// so views can call it freely.
func (a *App) palette(c *ui.Context) palette {
	if a.isDark(c) {
		return a.paletteDark
	}
	return a.paletteLight
}

// uiTheme returns mygo's control theme for the current variant.
func (a *App) uiTheme(c *ui.Context) *ui.Theme {
	if a.isDark(c) {
		return a.uiDark
	}
	return a.uiLight
}

// activeTheme is the theme variant that currently paints the windows.
func (a *App) activeTheme(c *ui.Context) *theme.Theme {
	if a.isDark(c) {
		return a.themeByID(a.prefs.DarkTheme, theme.Dark)
	}
	return a.themeByID(a.prefs.LightTheme, theme.Light)
}

// WindowBackgroundColor is the native window fill, following both variants so
// the first paint matches which theme the system appearance selects.
func (a *App) WindowBackgroundColor() string {
	light := a.themeByID(a.prefs.LightTheme, theme.Light)
	dark := a.themeByID(a.prefs.DarkTheme, theme.Dark)
	lw, dw := light.Colors.Window, dark.Colors.Window
	if lw == "" {
		lw = "#ffffff"
	}
	if dw == "" {
		dw = "#06070f"
	}
	if lw == dw {
		return lw
	}
	return fmt.Sprintf("light-dark(%s, %s)", lw, dw)
}

// themesOf lists the themes in one appearance, for the settings pickers.
func (a *App) themesOf(appearance theme.Appearance) []*theme.Theme {
	out := make([]*theme.Theme, 0, len(a.themes))
	for _, t := range a.themes {
		if t.Appearance == appearance {
			out = append(out, t)
		}
	}
	return out
}

// applyThemeSelection persists changes to the selected variants and repaints.
func (a *App) applyThemeSelection() {
	a.savePrefs()
	a.rebuildThemePalettes()
	a.pushTheme()
}

// pushTheme updates every open window with the new window fill and repaints.
func (a *App) pushTheme() {
	bg := a.WindowBackgroundColor()
	for _, w := range []*mygo.Window{a.win, a.settingsWin, a.logsWin, a.setupWin} {
		if w == nil {
			continue
		}
		_ = w.SetBackgroundColor(bg)
		w.Invalidate()
	}
}

// reloadThemes re-reads the theme folder, keeping the current selection when
// that theme still exists.
func (a *App) reloadThemes() {
	a.loadThemes()
	a.pushTheme()
}

// ensureThemesDir creates the user theme folder so the settings page can open
// it before any custom theme exists.
func (a *App) ensureThemesDir() string {
	dir := a.themesDir()
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// themeOptions lists a variant's themes for the pickers.
func (a *App) themeOptions(appearance theme.Appearance) []SelectOption {
	list := a.themesOf(appearance)
	out := make([]SelectOption, 0, len(list))
	for _, t := range list {
		out = append(out, SelectOption{Value: t.ID, Label: t.ID})
	}
	return out
}

// themeEditorCopy returns the draft for a custom theme, loading it once per
// selection. Built-in themes are handled by duplicateTheme instead.
func (a *App) themeEditorCopy(t *theme.Theme) *theme.Theme {
	if a.themeDraft != nil && a.themeEditID == t.ID {
		return a.themeDraft
	}
	copy := *t
	copy.Builtin = false
	a.themeDraft = &copy
	a.themeEditID = t.ID
	a.themeDirty = false
	return a.themeDraft
}

// nextThemeID picks a free custom-<n>-light/dark ID.
func (a *App) nextThemeID(appearance theme.Appearance) string {
	for i := 1; ; i++ {
		id := "custom-" + string(appearance)
		if i > 1 {
			id = fmt.Sprintf("custom-%d-%s", i, appearance)
		}
		if a.themeIndex[id] == nil {
			return id
		}
	}
}

// duplicateTheme copies a read-only built-in into a user theme, selects it and
// opens it in the editor. The shipped palette stays untouched.
func (a *App) duplicateTheme(src *theme.Theme) {
	if src == nil {
		return
	}
	id := a.nextThemeID(src.Appearance)
	dst := &theme.Theme{ID: id, Appearance: src.Appearance, Colors: src.Colors, Material: src.Material}
	if err := a.writeThemeFile(dst); err != nil {
		a.showToast(err.Error())
		return
	}
	if src.Appearance == theme.Dark {
		a.prefs.DarkTheme = id
	} else {
		a.prefs.LightTheme = id
	}
	a.loadThemes()
	a.savePrefs()
	a.themeEditID = id
	a.themeDraft = nil
	a.pushTheme()
	a.showToast(a.t("settings.themes.saved"))
}

// writeThemeFile stores one theme as JSON next to the other user themes.
func (a *App) writeThemeFile(t *theme.Theme) error {
	dir := a.ensureThemesDir()
	body := struct {
		Colors   theme.Colors   `json:"colors"`
		Material theme.Material `json:"material"`
	}{t.Colors, t.Material}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	if err := t.Validate(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, t.ID+".json"), append(data, '\n'), 0o644)
}

// saveThemeDraft writes the edited copy and reloads the registry.
func (a *App) saveThemeDraft() {
	if a.themeDraft == nil {
		return
	}
	if err := a.writeThemeFile(a.themeDraft); err != nil {
		a.showToast(err.Error())
		return
	}
	a.loadThemes()
	a.themeDraft = nil
	a.themeDirty = false
	a.pushTheme()
	a.showToast(a.t("settings.themes.saved"))
}

// resetThemeDraft restores the editable copy to the embedded default of its
// appearance, while leaving the file untouched until the user saves.
func (a *App) resetThemeDraft() {
	if a.themeDraft == nil {
		return
	}
	base := a.themeIndex["default-"+string(a.themeDraft.Appearance)]
	if base == nil {
		return
	}
	a.themeDraft.Colors = base.Colors
	a.themeDraft.Material = base.Material
	a.themeDirty = true
}

// themeColorToken maps one editable colour token to its JSON field and i18n
// label suffix.
type themeColorToken struct {
	Key   string
	Label string
	Field func(*theme.Colors) *string
}

var themeColorTokens = []themeColorToken{
	{"window", "window", func(c *theme.Colors) *string { return &c.Window }},
	{"background", "background", func(c *theme.Colors) *string { return &c.Background }},
	{"sidebar", "sidebar", func(c *theme.Colors) *string { return &c.Sidebar }},
	{"surface", "surface", func(c *theme.Colors) *string { return &c.Surface }},
	{"surfaceAlt", "surfaceAlt", func(c *theme.Colors) *string { return &c.SurfaceAlt }},
	{"text", "text", func(c *theme.Colors) *string { return &c.Text }},
	{"textMuted", "textMuted", func(c *theme.Colors) *string { return &c.TextMuted }},
	{"textFaint", "textFaint", func(c *theme.Colors) *string { return &c.TextFaint }},
	{"border", "border", func(c *theme.Colors) *string { return &c.Border }},
	{"hover", "hover", func(c *theme.Colors) *string { return &c.Hover }},
	{"pressed", "pressed", func(c *theme.Colors) *string { return &c.Pressed }},
	{"accent", "accent", func(c *theme.Colors) *string { return &c.Accent }},
	{"onAccent", "onAccent", func(c *theme.Colors) *string { return &c.OnAccent }},
	{"danger", "danger", func(c *theme.Colors) *string { return &c.Danger }},
	{"warning", "warning", func(c *theme.Colors) *string { return &c.Warning }},
	{"info", "info", func(c *theme.Colors) *string { return &c.Info }},
	{"star", "star", func(c *theme.Colors) *string { return &c.Star }},
	{"starStrong", "starStrong", func(c *theme.Colors) *string { return &c.StarStrong }},
	{"shadow", "shadow", func(c *theme.Colors) *string { return &c.Shadow }},
	{"overlay", "overlay", func(c *theme.Colors) *string { return &c.Overlay }},
	{"preview", "preview", func(c *theme.Colors) *string { return &c.Preview }},
	{"badge", "badge", func(c *theme.Colors) *string { return &c.Badge }},
	{"onBadge", "onBadge", func(c *theme.Colors) *string { return &c.OnBadge }},
}
