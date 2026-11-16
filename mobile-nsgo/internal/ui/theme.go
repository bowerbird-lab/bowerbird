package ui

import (
	"io"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	apptheme "github.com/owocc/bowerbird/mobile-nsgo/internal/theme"
)

const (
	themeModeSystem = "system"
	themeModeLight  = "light"
	themeModeDark   = "dark"
)

// setupTheme loads the desktop-compatible theme files and keeps the NSGo
// palette in sync with the saved appearance settings. It runs once, before
// the first render, on the root scope.
func (a *App) setupTheme(cx *core.Scope) {
	if len(a.themes) == 0 {
		if builtins, err := apptheme.Builtins(); err == nil {
			a.themes = builtins
		} else {
			log.Printf("[Theme] no built-in themes: %v", err)
		}
	}
	a.themeIndex = make(map[string]*apptheme.Theme, len(a.themes))
	for _, t := range a.themes {
		a.themeIndex[t.ID] = t
	}
	if a.env.Accent != "" {
		if c, err := apptheme.ParseColor(a.env.Accent); err == nil {
			a.systemAccent = c
		}
	}
	if a.normalizeThemeSelection() {
		a.savePreferences()
	}
	// Paint the saved palette on the first frame; the effect then follows
	// every change made from the settings screen.
	a.applyTheme(cx)
	cx.Effect(func() { a.applyTheme(cx) })
}

// themeExists reports whether id names a loaded theme of the given appearance.
func (a *App) themeExists(id string, appearance apptheme.Appearance) bool {
	t := a.themeIndex[id]
	return t != nil && t.Appearance == appearance
}

// normalizeThemeSelection repairs a stored ID that no longer resolves. The
// returned flag tells the caller to persist the repaired values.
func (a *App) normalizeThemeSelection() bool {
	changed := false
	light := a.settingsLightTheme.Get()
	if !a.themeExists(light, apptheme.Light) {
		a.settingsLightTheme.Set(apptheme.DefaultLight)
		changed = true
	}
	dark := a.settingsDarkTheme.Get()
	if !a.themeExists(dark, apptheme.Dark) {
		a.settingsDarkTheme.Set(apptheme.DefaultDark)
		changed = true
	}
	switch a.settingsMode.Get() {
	case themeModeSystem, themeModeLight, themeModeDark:
	default:
		a.settingsMode.Set(themeModeSystem)
		changed = true
	}
	return changed
}

// themeByID finds a theme of the given appearance, falling back to the
// embedded default and then to any theme of that appearance.
func (a *App) themeByID(id string, appearance apptheme.Appearance) *apptheme.Theme {
	if t := a.themeIndex[id]; t != nil && t.Appearance == appearance {
		return t
	}
	if t := a.themeIndex[apptheme.DefaultID(appearance)]; t != nil {
		return t
	}
	for _, t := range a.themes {
		if t.Appearance == appearance {
			return t
		}
	}
	// setupTheme always has the embedded presets; this keeps a zero App safe.
	return apptheme.Default(appearance)
}

// themeColor parses a token, falling back to transparent rather than taking a
// window down when a hand-written theme carries a bad value.
func themeColor(s string) core.Color {
	c, err := apptheme.ParseColor(s)
	if err != nil {
		return 0
	}
	return c
}

// themeRole returns one of the theme's named roles. Named roles let the mobile
// UI use the same tokens as the desktop (sidebar, overlay, preview, badge…).
func themeRole(cx *core.Scope, name string) core.Color {
	p := cx.Palette()
	if c, ok := p.Color(name); ok {
		return c
	}
	return p.Background
}

// applyTheme resolves both variants, applies an optional system accent and
// gives the result to NSGo. Signals read here make it rerun on a settings
// change.
func (a *App) applyTheme(cx *core.Scope) {
	mode := a.settingsMode.Get()
	light := a.themeByID(a.settingsLightTheme.Get(), apptheme.Light)
	dark := a.themeByID(a.settingsDarkTheme.Get(), apptheme.Dark)
	accent := core.Color(0)
	if a.settingsSystemAccent.Get() && a.systemAccent != 0 {
		accent = a.systemAccent
	}
	cx.SetTheme(buildCoreTheme(light, dark, accent))
	switch mode {
	case themeModeLight:
		cx.SetAppearance(core.AppearanceLight)
	case themeModeDark:
		cx.SetAppearance(core.AppearanceDark)
	default:
		cx.SetAppearance(core.AppearanceSystem)
	}
}

// buildCoreTheme maps the desktop theme tokens onto NSGo's palettes and named
// roles.
func buildCoreTheme(light, dark *apptheme.Theme, accent core.Color) *core.Theme {
	t := core.DefaultTheme()
	t.Light = paletteForTheme(light, accent)
	t.Dark = paletteForTheme(dark, accent)
	return t
}

func paletteForTheme(t *apptheme.Theme, accent core.Color) core.Palette {
	c := t.Colors
	text := themeColor(c.Text)
	bg := themeColor(c.Background)
	accentColor := themeColor(c.Accent)
	onAccent := themeColor(c.OnAccent)
	if accent != 0 {
		accentColor = accent
		onAccent = accentText(accent)
	}
	p := core.Palette{
		Background:     bg,
		Surface:        themeColor(c.Surface),
		SurfaceHover:   themeColor(c.Hover),
		SurfacePressed: themeColor(c.Pressed),
		Border:         themeColor(c.Border),
		Text:           text,
		TextMuted:      themeColor(c.TextMuted),
		Accent:         accentColor,
		AccentHover:    accentColor,
		AccentPressed:  accentColor,
		AccentText:     onAccent,
		Danger:         themeColor(c.Danger),
		Warning:        themeColor(c.Warning),
		Success:        themeColor(c.Success),
		Selection:      accentColor.Faded(0.22),
		Focus:          accentColor.Faded(0.55),
		Scrollbar:      themeColor(c.TextFaint),
	}
	p.Colors = map[string]core.Color{
		"window":      themeColor(c.Window),
		"sidebar":     themeColor(c.Sidebar),
		"surfaceAlt":  themeColor(c.SurfaceAlt),
		"inset":       themeColor(c.SurfaceAlt),
		"raised":      themeColor(c.Surface),
		"field":       themeColor(c.SurfaceAlt),
		"fieldBorder": themeColor(c.Border),
		"line":        themeColor(c.Border),
		"faint":       themeColor(c.TextFaint),
		"accentFill":  accentColor,
		"light":       text,
		"onLight":     bg,
		"track":       themeColor(c.Border),
		"knob":        themeColor(c.Surface),
		"code":        text,
		"info":        themeColor(c.Info),
		"down":        themeColor(c.Danger),
		"star":        themeColor(c.Star),
		"starStrong":  themeColor(c.StarStrong),
		"shadow":      themeColor(c.Shadow),
		"overlay":     themeColor(c.Overlay),
		"preview":     themeColor(c.Preview),
		"badge":       themeColor(c.Badge),
		"onBadge":     themeColor(c.OnBadge),
		"success":     themeColor(c.Success),
	}
	if t.Material.HeaderTint != "" {
		p.Colors["headerTint"] = themeColor(t.Material.HeaderTint)
	}
	return p
}

// accentText returns black or white, whichever reads best on accent.
func accentText(accent core.Color) core.Color {
	channel := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b, _ := accent.Components()
	lum := 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
	if lum > 0.5 {
		return core.RGB(0x000000)
	}
	return core.RGB(0xffffff)
}

// savePreferences persists the small appearance document synchronously so a
// user who quits immediately after a choice keeps it.
func (a *App) savePreferences() {
	if a.local == nil || a.settingsMode == nil {
		return
	}
	prefs := storage.Preferences{
		Version:       storage.PreferencesVersion,
		ThemeMode:     a.settingsMode.Get(),
		LightTheme:    a.settingsLightTheme.Get(),
		DarkTheme:     a.settingsDarkTheme.Get(),
		SystemAccent:  a.settingsSystemAccent.Get(),
		GridColumns:   a.gridCols.Get(),
		GalleryLayout: a.galleryLayout.Get(),
	}
	if err := a.local.SavePreferences(prefs); err != nil {
		log.Printf("[Settings] saving preferences: %v", err)
	}
}

func (a *App) setThemeMode(mode string) {
	switch mode {
	case themeModeLight, themeModeDark, themeModeSystem:
	default:
		return
	}
	if a.settingsMode.Get() == mode {
		return
	}
	a.settingsMode.Set(mode)
	a.savePreferences()
	a.bump()
}

func (a *App) setThemeSelection(appearance apptheme.Appearance, id string) {
	if !a.themeExists(id, appearance) {
		return
	}
	if appearance == apptheme.Dark {
		if a.settingsDarkTheme.Get() == id {
			return
		}
		a.settingsDarkTheme.Set(id)
	} else {
		if a.settingsLightTheme.Get() == id {
			return
		}
		a.settingsLightTheme.Set(id)
	}
	a.savePreferences()
	a.bump()
}

func (a *App) setSystemAccent(on bool) {
	if a.settingsSystemAccent.Get() == on {
		return
	}
	a.settingsSystemAccent.Set(on)
	a.savePreferences()
	a.bump()
}

func (a *App) resetThemeSettings() {
	a.settingsMode.Set(themeModeSystem)
	a.settingsLightTheme.Set(apptheme.DefaultLight)
	a.settingsDarkTheme.Set(apptheme.DefaultDark)
	a.settingsSystemAccent.Set(true)
	a.savePreferences()
	a.bump()
	a.showToast("已恢复默认外观")
}

func (a *App) reloadThemes() {
	a.themes = apptheme.Load(a.themesDir())
	a.themeIndex = make(map[string]*apptheme.Theme, len(a.themes))
	for _, t := range a.themes {
		a.themeIndex[t.ID] = t
	}
	if a.normalizeThemeSelection() {
		a.savePreferences()
	}
	a.applyTheme(a.scope)
	a.bump()
	a.showToast("已重新读取主题文件")
}

func (a *App) themesDir() string {
	return filepath.Join(a.env.DataDir, "themes")
}

// importTheme copies a desktop-compatible JSON theme into the app's themes
// folder, validates it, then selects it. The file name must end with
// -light.json or -dark.json.
func (a *App) importTheme(cx *core.Scope) {
	cx.PickFiles(core.PickFilesOptions{Types: []string{".json"}}, func(files []core.File, err error) {
		if err != nil {
			a.showToast("选择主题文件失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		f := files[0]
		if f.Temporary && f.Path != "" {
			defer os.Remove(f.Path)
		}
		id := apptheme.IDFor(f.Name)
		appearance, err := apptheme.AppearanceFor(id)
		if err != nil {
			a.showToast("主题文件名需以 -light.json 或 -dark.json 结尾")
			return
		}
		if apptheme.BuiltinIDs[id] {
			a.showToast("内置主题 ID 不能覆盖，请先重命名文件")
			return
		}
		rc, err := f.Open()
		if err != nil {
			a.showToast("读取主题文件失败: " + err.Error())
			return
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			a.showToast("读取主题文件失败: " + readErr.Error())
			return
		}
		parsed, err := apptheme.Parse(id, data)
		if err != nil {
			a.showToast("主题文件无法解析: " + err.Error())
			return
		}
		parsed = apptheme.Merge(apptheme.Default(appearance), parsed)
		if err := parsed.Validate(); err != nil {
			a.showToast("主题颜色无效: " + err.Error())
			return
		}
		dir := a.themesDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.showToast("无法创建主题文件夹: " + err.Error())
			return
		}
		if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0o644); err != nil {
			a.showToast("保存主题失败: " + err.Error())
			return
		}
		a.reloadThemes()
		a.setThemeSelection(appearance, id)
		a.showToast("已导入主题 " + themeLabel(parsed))
	})
}

func (a *App) openSettings(from string) {
	a.settingsBackScreen = from
	a.screen.Set(screenSettings)
	a.bump()
}

func (a *App) closeSettings() {
	target := a.settingsBackScreen
	if target == "" {
		if a.session != nil && a.session.Current() != nil {
			target = screenGallery
		} else {
			target = screenConnect
		}
	}
	a.screen.Set(target)
	a.bump()
}

func (a *App) themesFor(appearance apptheme.Appearance) []*apptheme.Theme {
	out := make([]*apptheme.Theme, 0, len(a.themes))
	for _, t := range a.themes {
		if t.Appearance == appearance {
			out = append(out, t)
		}
	}
	return out
}

func themeLabel(t *apptheme.Theme) string {
	switch t.ID {
	case apptheme.DefaultLight:
		return "默认（浅色）"
	case apptheme.DefaultDark:
		return "默认（深色）"
	case "sand-light":
		return "暖沙（浅色）"
	case "midnight-dark":
		return "午夜（深色）"
	}
	return t.ID
}

func themeSwatches(t *apptheme.Theme) []core.Color {
	return []core.Color{
		themeColor(t.Colors.Background),
		themeColor(t.Colors.SurfaceAlt),
		themeColor(t.Colors.Accent),
		themeColor(t.Colors.Text),
	}
}

func (a *App) currentThemeLabel(appearance apptheme.Appearance) string {
	if appearance == apptheme.Dark {
		return themeLabel(a.themeByID(a.settingsDarkTheme.Get(), apptheme.Dark))
	}
	return themeLabel(a.themeByID(a.settingsLightTheme.Get(), apptheme.Light))
}

func (a *App) openThemePicker(appearance apptheme.Appearance) {
	a.settingsThemePick = appearance
	a.bump()
	a.settingsThemeOpen.Set(true)
}

func (a *App) themePickerSheet(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.settingsThemeOpen.Get() },
		OnClose: func() { a.settingsThemeOpen.Set(false) },
		Title:   "选择主题",
		TestID:  "settings.themePicker",
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.settingsThemeOpen.Get()
				appearance := a.settingsThemePick
				title := "选择浅色主题"
				if appearance == apptheme.Dark {
					title = "选择深色主题"
				}
				list := a.themesFor(appearance)
				rows := make([]core.Node, 0, len(list))
				for _, th := range list {
					t := th
					selected := func() bool {
						if appearance == apptheme.Dark {
							return a.settingsDarkTheme.Get() == t.ID
						}
						return a.settingsLightTheme.Get() == t.ID
					}
					swatches := append(themeSwatches(t), themeColor(t.Colors.Surface))
					chips := make([]core.Node, 0, len(swatches))
					for _, c := range swatches {
						chips = append(chips, core.View{Style: core.Style{
							Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9,
							Background: c, BorderWidth: 1, BorderColor: colors.Border(),
						}})
					}
					rows = append(rows, core.View{
						TestID: "settings.theme." + t.ID,
						StyleFunc: func() core.Style {
							st := drawerRowStyle(colors)
							st.BorderWidth = 1
							if selected() {
								st.BorderColor = colors.Accent()
							}
							return st
						},
						OnPress: func(core.PressEvent) {
							a.setThemeSelection(appearance, t.ID)
							a.settingsThemeOpen.Set(false)
						},
						Children: []core.Node{
							core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: chips},
							core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
								core.Text{Content: themeLabel(t), MaxLines: 1, Style: nsui.TypeBody,
									StyleFunc: nsui.Colored(colors.Text)},
								core.Text{Content: t.ID, MaxLines: 1, Style: nsui.TypeCaption,
									StyleFunc: nsui.Colored(colors.Faint)},
							}},
							core.Show{When: selected, Then: func(cx *core.Scope) core.Node {
								return nsui.Icon(lucide.Check, 18, colors.Accent)
							}},
						},
					})
				}
				return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.Text{Content: title, Style: nsui.TypeLabel, StyleFunc: nsui.Colored(colors.Muted)},
					core.Scroll{
						Style:    core.Style{MaxHeight: core.Pt(420)},
						Children: []core.Node{core.View{Style: core.Style{Gap: 6}, Children: rows}},
					},
				}}
			}),
		},
	})
}

// settingsPage is the mobile appearance/settings screen.
func (a *App) settingsPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()
	accentAvailable := a.systemAccent != 0

	label := func(text string) core.Node {
		return core.Text{Content: text, Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})}
	}
	card := func(title string, children ...core.Node) core.Node {
		body := make([]core.Node, 0, len(children)+1)
		if title != "" {
			body = append(body, label(title))
		}
		body = append(body, children...)
		return core.View{
			StyleFunc: func() core.Style {
				return core.Style{Gap: 8, Padding: core.All(14), CornerRadius: 14,
					Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
			},
			Children: body,
		}
	}
	divider := core.View{StyleFunc: func() core.Style {
		return core.Style{Height: core.Pt(1), Background: colors.Line()}
	}}
	themeRow := func(appearance apptheme.Appearance) core.Node {
		name := "浅色主题"
		if appearance == apptheme.Dark {
			name = "深色主题"
		}
		return core.View{
			TestID: "settings." + string(appearance) + "Theme",
			StyleFunc: func() core.Style {
				return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10,
					Padding: core.Symmetric(8, 0)}
			},
			OnPress: func(core.PressEvent) { a.openThemePicker(appearance) },
			Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
					core.Text{Content: name, Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
					core.Text{ContentFunc: func() string { return a.currentThemeLabel(appearance) },
						MaxLines: 1, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
				}},
				core.Dynamic(func(cx *core.Scope) core.Node {
					_ = a.rev.Get()
					theme := a.themeByID(a.settingsLightTheme.Get(), apptheme.Light)
					if appearance == apptheme.Dark {
						theme = a.themeByID(a.settingsDarkTheme.Get(), apptheme.Dark)
					}
					chips := make([]core.Node, 0, 5)
					for _, c := range append(themeSwatches(theme), themeColor(theme.Colors.Surface)) {
						chips = append(chips, core.View{Style: core.Style{
							Width: core.Pt(15), Height: core.Pt(15), CornerRadius: 8,
							Background: c, BorderWidth: 1, BorderColor: colors.Border(),
						}})
					}
					return core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: chips}
				}),
				core.Icon{SVG: lucide.ChevronRight(), Style: core.Style{FontSize: 14, Color: colors.Faint()}},
			},
		}
	}

	// A compact live preview of the selected light and dark palettes.
	preview := func(appearance apptheme.Appearance, title string) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			_ = a.rev.Get()
			th := a.themeByID(a.settingsLightTheme.Get(), apptheme.Light)
			if appearance == apptheme.Dark {
				th = a.themeByID(a.settingsDarkTheme.Get(), apptheme.Dark)
			}
			bg := themeColor(th.Colors.Background)
			surface := themeColor(th.Colors.Surface)
			surfaceAlt := themeColor(th.Colors.SurfaceAlt)
			text := themeColor(th.Colors.Text)
			muted := themeColor(th.Colors.TextMuted)
			accent := themeColor(th.Colors.Accent)
			onAccent := themeColor(th.Colors.OnAccent)
			if a.settingsSystemAccent.Get() && a.systemAccent != 0 {
				accent = a.systemAccent
				onAccent = accentText(accent)
			}
			border := themeColor(th.Colors.Border)
			return core.View{
				StyleFunc: func() core.Style {
					return core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 8,
						Padding: core.All(12), CornerRadius: 12, Background: bg,
						BorderWidth: 1, BorderColor: border}
				},
				Children: []core.Node{
					core.Text{Content: title, Style: core.Style{FontSize: 10, LineHeight: 13, FontWeight: 600, Color: muted}},
					core.Text{Content: "素材库", Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: 650, Color: text}},
					core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}, Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(44), Height: core.Pt(24), CornerRadius: 7,
							Background: surface, BorderWidth: 1, BorderColor: border}},
						core.View{Style: core.Style{Width: core.Pt(38), Height: core.Pt(18), CornerRadius: 9,
							Background: surfaceAlt}},
						core.View{Style: core.Style{Padding: core.Symmetric(5, 10), CornerRadius: 999,
							Background: accent}, Children: []core.Node{
							core.Text{Content: "主要", Style: core.Style{FontSize: 10, LineHeight: 13, Color: onAccent}},
						}},
					}},
				},
			}
		})
	}

	modeOptions := []nsui.Option[string]{
		{Value: themeModeSystem, Label: "跟随系统"},
		{Value: themeModeLight, Label: "浅色"},
		{Value: themeModeDark, Label: "深色"},
	}
	layoutOptions := []nsui.Option[string]{
		{Value: storage.LayoutAuto, Label: "自动"},
		{Value: storage.LayoutWaterfall, Label: "瀑布流"},
		{Value: storage.LayoutGrid, Label: "网格"},
	}

	return core.View{
		Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Gap: 10},
		Children: []core.Node{
			core.Scroll{
				Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
				Children: []core.Node{
					core.View{StyleFunc: func() core.Style {
						safe := cx.Env().SafeArea()
						return core.Style{Gap: 10, Padding: core.Edges{
							Top: safe.Top + 56, Bottom: safe.Bottom + 20, Left: safe.Left + 12, Right: safe.Right + 12,
						}}
					}, Children: []core.Node{
						card("外观",
							core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
								core.Text{Content: "颜色模式", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
								core.Text{Content: "跟随系统，或固定使用浅色 / 深色主题。",
									Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
								nsui.Tabs[string]{Options: modeOptions, Value: func() string { return a.settingsMode.Get() },
									Equal: true, OnChange: a.setThemeMode, TestID: "settings.mode"},
							}},
						),
						card("浏览",
							core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
								core.Text{Content: "排版模式", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
								core.Text{Content: "自动模式在 2-3 列切换为瀑布流，4-5 列为网格。",
									Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
								nsui.Tabs[string]{Options: layoutOptions, Value: func() string { return a.galleryLayout.Get() },
									Equal: true, OnChange: func(v string) { a.setGalleryLayout(cx, v) }, TestID: "settings.layoutMode"},
							}},
							divider,
							core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
								core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
									core.Text{Content: "列数密度", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
									core.Text{ContentFunc: func() string { return a.layoutSummaryLabel() + " · 也支持双指捏合切换" },
										Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
								}},
								nsui.Button{Text: "-", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.effectiveGridCols() <= 2 },
									OnPress: func() { a.setGridColumns(cx, a.effectiveGridCols()-1) }, TestID: "settings.gridLess"},
								nsui.Button{Text: "+", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.effectiveGridCols() >= a.maxGridCols() },
									OnPress: func() { a.setGridColumns(cx, a.effectiveGridCols()+1) }, TestID: "settings.gridMore"},
							}},
						),
						card("主题",
							themeRow(apptheme.Light),
							divider,
							themeRow(apptheme.Dark),
						),
						card("预览",
							core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
								preview(apptheme.Light, "浅色"),
								preview(apptheme.Dark, "深色"),
							}},
						),
						card("强调色",
							core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
								core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
									core.Text{Content: "跟随系统强调色", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
									core.Text{ContentFunc: func() string {
										if accentAvailable {
											return "使用设备的系统强调色覆盖主题的强调色。"
										}
										return "当前设备未提供系统强调色。"
									}, MaxLines: 2, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
								}},
								core.Show{When: func() bool { return accentAvailable }, Then: func(cx *core.Scope) core.Node {
									return nsui.Switch{Checked: func() bool { return a.settingsSystemAccent.Get() },
										OnChange: a.setSystemAccent, Label: "跟随系统强调色", TestID: "settings.systemAccent"}
								}},
								core.Show{When: func() bool { return !accentAvailable }, Then: func(cx *core.Scope) core.Node {
									return core.View{Style: core.Style{Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 5,
										Background: colors.Faint()}}
								}},
							}},
						),
						card("主题文件",
							core.Text{Content: "与电脑端使用同一套 JSON 主题。把主题文件放入下面的文件夹后点“重新读取”。",
								Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
							core.View{
								StyleFunc: func() core.Style {
									return core.Style{Padding: core.Symmetric(8, 10), CornerRadius: 8,
										Background: colors.Inset(), BorderWidth: 1, BorderColor: colors.Border()}
								},
								Children: []core.Node{core.Text{Content: a.themesDir(), MaxLines: 3,
									Style: core.Style{FontSize: 11, LineHeight: 15, Color: colors.Faint()}}},
							},
							core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8}, Children: []core.Node{
								nsui.Button{Text: "导入主题", Variant: nsui.ButtonSecondary, OnPress: func() { a.importTheme(cx) },
									TestID: "settings.themes.import"},
								nsui.Button{Text: "重新读取", Variant: nsui.ButtonSecondary, OnPress: a.reloadThemes,
									TestID: "settings.themes.reload"},
								nsui.Button{Text: "恢复默认", Variant: nsui.ButtonGhost, OnPress: a.resetThemeSettings,
									TestID: "settings.themes.reset"},
							}},
						),
						core.Text{Content: "版本 1.1.0", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Faint(), TextAlign: core.TextAlignCenter, Padding: core.All(8)})},
					}},
				},
			},
			a.appHeader(cx, appHeaderSpec{
				Floating:         true,
				InsetSafeArea:    true,
				OmitLeftSafeArea: func() bool { return true },
				TestID:           "settings.header",
				Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
						appCircleButton(colors, "settings.back", lucide.ChevronLeft, func(core.PressEvent) { a.closeSettings() }),
						core.Text{Content: "设置", Role: core.RoleHeader,
							Style:     nsui.TypeTitle.Merge(core.Style{FlexGrow: 1, TextAlign: core.TextAlignCenter}),
							StyleFunc: nsui.Colored(colors.Text)},
						core.View{Style: core.Style{Width: core.Pt(42)}},
					}},
				},
			}),
			a.themePickerSheet(cx),
		},
	}
}
