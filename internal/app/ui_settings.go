package app

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bowerbird/core"
	"bowerbird/internal/i18n"
	"bowerbird/internal/theme"

	"github.com/egoist/mygo/ui"
)

// refreshSettings reloads values shown by the settings window.
func (a *App) refreshSettings() {
	if a.svc == nil {
		return
	}
	a.syncLanguage()
	a.themeValue = a.svc.GetTheme()
	if a.themeValue == "" {
		a.themeValue = core.ThemeSystem
	}
	a.languageValue = a.svc.GetLanguage()
	switch a.languageValue {
	case "", "system":
		a.languageValue = "system"
	default:
		// Older builds persisted full locale tags such as "zh-CN"; the pills
		// use the short dictionary code.
		a.languageValue = i18n.Resolve(a.languageValue)
	}
	a.vibrancyValue = a.svc.GetVibrancyEnabled()
	a.importModeValue = a.svc.GetImportMode()
	if a.importModeValue == "" {
		a.importModeValue = core.ImportModeCopy
	}
	a.network = a.svc.GetNetworkSettings()
	a.shortcuts = a.svc.GetShortcutSettings()
	a.rebuildShortcutPairs()
	if status, err := a.svc.GetMobileServerStatus(); err == nil {
		a.mobile = status
	}
	if devices, err := a.svc.ListMobileDevices(); err == nil {
		a.mobileDevices = devices
	}
	if url, err := a.svc.GetMobilePairingURL(); err == nil && url != "" {
		if url != a.mobilePairingURL {
			a.mobilePairingURL = url
			a.mobileQR = mobileQRBitmap(url)
		}
	} else {
		a.mobilePairingURL = ""
		a.mobileQR = nil
	}
	if a.mobile != nil && a.mobilePort == "" {
		a.mobilePort = strconv.Itoa(a.mobile.Port)
	}
	if plugins, err := a.svc.ListInstalledPlugins(); err == nil {
		a.plugins = a.pluginRows(plugins)
	}
}

// viewSettings builds the independent native settings window.
func (a *App) viewSettings(c *ui.Context) {
	p := a.palette(c)
	a.applyVibrancyRoot(c)
	a.handleSettingsShortcuts(c)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		bar := c.TitleBar()
		// Section rail. Its header matches the main window's sidebar header:
		// the title sits right of the traffic lights, on the same line and
		// with the same inset as the sidebar toggle there.
		rail := ui.Column(c).Width(defaultSidebarWidth).FillHeight()
		a.applySidebarSurface(c, rail)
		rail.Children(func() {
			ui.Row(c).Height(headerHeight).FillWidth().Padding(0, 8, 0, bar.Left+gallerySidePadding).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, a.t("settings.title")).FontSize(14).FontWeight(650).TextColor(p.text)
			})
			ui.Column(c).FillWidth().Grow(1).Shrink(1).Padding(10, 10, 10, 10).Children(func() {
				labels := a.settingsSectionLabels()
				for i, label := range labels {
					i, label := i, label
					b := ui.ButtonBase(c.Key(fmt.Sprintf("settings-nav-%d", i))).FillWidth().Height(32).PaddingX(10).Radius(7).FocusRing(false).AlignItems(ui.Center).Justify(ui.Start)
					bg := ui.Transparent
					if a.settingsSection == i {
						bg = p.press
					} else if b.Hovered() {
						bg = p.hover
					}
					b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
					b.Children(func() { ui.Text(c, label).FontSize(12.5).TextColor(p.text) })
					if b.Clicked() {
						if a.settingsSection != i {
							a.settingsSection = i
							a.settingsScroll.Y = 0
						}
					}
				}
				ui.Box(c).Grow(1)
				ui.Textf(c, "v%s", "1.0.0-alpha-1").FontSize(10).TextColor(p.faint).Padding(10, 8)
			})
		})
		ui.Box(c).Width(1).FillHeight().Background(p.hairline)
		content := ui.Column(c).FillHeight().Grow(1).Shrink(1).MinWidth(0)
		// As in the main window, the header paints its own strip at rest and
		// blurs the content once it scrolls underneath.
		a.paintBelowHeader(c, content, headerHeight)
		content.Children(func() {
			// The content scrolls under one floating header, as the gallery
			// does under the main toolbar. Its top inset keeps the first card
			// clear of the header until the user scrolls.
			ui.Scroll(c).Fill().TrackScroll(&a.settingsScroll).Padding(headerHeight+settingsContentTopPadding, 24, 28, 24).ScrollbarInsets(headerHeight+settingsContentTopPadding, 0, 28, 0).Children(func() {
				switch a.settingsSection {
				case 0:
					a.settingsGeneral(c)
				case 1:
					a.settingsAppearance(c)
				case 2:
					a.settingsThemes(c)
				case 3:
					a.settingsShortcuts(c)
				case 4:
					a.settingsPlugins(c)
				case 5:
					a.settingsMobile(c)
				case 6:
					a.settingsNetwork(c)
				case 7:
					a.settingsStorage(c)
				default:
					a.settingsAbout(c)
				}
			})
			a.viewSettingsHeader(c)
		})
		if a.shortcutEditID != "" {
			a.viewShortcutDialog(c)
		}
	})
}

// settingsContentTopPadding is the space above the first settings card. Once
// the scroll passes it, the content has reached the floating header.
const settingsContentTopPadding = 20

func (a *App) settingsHeaderHasContentUnder() bool {
	return a.settingsScroll.Y > settingsContentTopPadding
}

// viewSettingsHeader draws the shared settings header over the scrolling
// content, with the same material, drag area and divider behaviour as the main
// toolbar.
func (a *App) viewSettingsHeader(c *ui.Context) {
	p := a.palette(c)
	header := ui.Column(c).FillWidth().Height(headerHeight).Absolute().Top(0).Left(0).Right(0)
	headerBlurActive := a.applyHeaderSurface(c, header, a.settingsHeaderHasContentUnder())
	header.Children(func() {
		bar := c.TitleBar()
		r := bar.Right
		if r == 0 && runtime.GOOS == "windows" {
			r = 138
		}
		ui.Row(c).FillWidth().Height(headerHeight).Padding(0, r+24, 0, 24).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, a.settingsSectionTitle(a.settingsSection)).FontSize(16).FontWeight(650).TextColor(p.text)
		})
		if headerBlurActive {
			ui.Box(c).Absolute().Bottom(0).FillWidth().Height(1).Background(p.hairline)
		}
	})
}

func (a *App) settingsSectionLabels() []string {
	return []string{
		a.t("settings.nav.general"),
		a.t("settings.nav.appearance"),
		a.t("settings.nav.themes"),
		a.t("settings.nav.shortcuts"),
		a.t("settings.nav.plugins"),
		a.t("settings.nav.mobile"),
		a.t("settings.nav.network"),
		a.t("settings.nav.data"),
		a.t("settings.about"),
	}
}

func (a *App) settingsSectionTitle(section int) string {
	labels := a.settingsSectionLabels()
	if section < 0 || section >= len(labels) {
		return a.t("settings.title")
	}
	return labels[section]
}

func (a *App) handleSettingsShortcuts(c *ui.Context) {
	if a.shortcutMatched(c, core.ShortcutCloseWindow) && a.settingsWin != nil {
		a.settingsWin.Close()
	}
	if a.shortcutMatched(c, core.ShortcutFullscreen) && a.settingsWin != nil {
		a.settingsWin.ToggleFullScreen()
	}
}

func (a *App) settingsCard(c *ui.Context, title string, fn func()) {
	a.settingsCardHelp(c, title, "", fn)
}

// settingsCardHelp is settingsCard with an info button beside the title; its
// tooltip carries the explanatory copy that would otherwise be a paragraph.
func (a *App) settingsCardHelp(c *ui.Context, title, help string, fn func()) {
	p := a.palette(c)
	ui.Column(c).FillWidth().Padding(14).Radius(10).Background(p.surface).Border(1, p.hairline).Gap(10).Children(func() {
		if title != "" {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, title).FontSize(11).FontWeight(650).LetterSpacing(0.4).TextColor(p.muted)
				a.infoIcon(c, help)
			})
		}
		fn()
	})
	ui.Box(c).Height(10)
}

// infoIcon is the small help affordance used by settings rows and cards.
func (a *App) infoIcon(c *ui.Context, help string) {
	if help == "" {
		return
	}
	p := a.palette(c)
	icon := ui.Icon(c, iconInfo).Size(13, 13).Shrink(0).Tooltip(help)
	if icon.Hovered() {
		icon.TextColor(p.muted)
	} else {
		icon.TextColor(p.faint)
	}
}

func (a *App) settingsRow(c *ui.Context, label, help string, control func()) {
	p := a.palette(c)
	ui.Row(c).FillWidth().MinHeight(36).AlignItems(ui.Center).Gap(14).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(p.text)
			if help != "" {
				ui.Text(c, help).FontSize(10.5).TextColor(p.faint).Wrap()
			}
		})
		control()
	})
}

// settingsRowInfo is settingsRow with the help copy behind a small info icon
// instead of under the label, so dense settings keep one line per control.
func (a *App) settingsRowInfo(c *ui.Context, label, help string, control func()) {
	p := a.palette(c)
	ui.Row(c).FillWidth().MinHeight(36).AlignItems(ui.Center).Gap(14).Children(func() {
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(p.text).Shrink(1).SingleLine()
			a.infoIcon(c, help)
		})
		control()
	})
}

func (a *App) settingsGeneral(c *ui.Context) {
	a.settingsCard(c, "", func() {
		a.settingsRowInfo(c, a.t("settings.language.title"), a.t("settings.language.description"), func() {
			options := []SelectOption{
				{Value: "system", Label: a.t("settings.language.auto")},
				{Value: "en", Label: a.t("settings.language.en")},
				{Value: "zh", Label: a.t("settings.language.zh")},
			}
			if lang, changed := a.dropdown(c, "settings-language", a.languageValue, options); changed {
				a.setLanguage(lang)
			}
		})
	})
	a.settingsCard(c, a.t("settings.behavior"), func() {
		importMode := a.importModeValue
		if importMode == "" {
			importMode = core.ImportModeCopy
		}
		a.settingsRowInfo(c, a.t("settings.importMode.title"), a.t("settings.importMode.description"), func() {
			options := []SelectOption{
				{Value: core.ImportModeCopy, Label: a.t("settings.importMode.copy")},
				{Value: core.ImportModeMove, Label: a.t("settings.importMode.move")},
			}
			if mode, changed := a.dropdown(c, "settings-import-mode", importMode, options); changed {
				if err := a.svc.SetImportMode(mode); err != nil {
					a.showToast(err.Error())
					return
				}
				a.importModeValue = mode
			}
		})
		enabled := a.svc.GetLightweightModeEnabled()
		a.settingsRowInfo(c, a.t("settings.lightweight.title"), a.t("settings.lightweight.description"), func() {
			on := enabled
			if ui.Switch(c.Key("lightweight"), &on).Changed() {
				go a.svc.SetLightweightModeEnabled(on)
			}
		})
		recordURL := a.prefs.RecordImportURL
		a.settingsRowInfo(c, a.t("settings.recordURL.title"), a.t("settings.recordURL.description"), func() {
			on := recordURL
			if ui.Switch(c.Key("record-import-url"), &on).Changed() {
				a.prefs.RecordImportURL = on
				a.savePrefs()
			}
		})
	})
}

// setLanguage persists a new UI language and re-applies it to every open
// window. Persisting happens off the UI thread; state and redraws come back
// through postSettings.
func (a *App) setLanguage(lang string) {
	if a.svc == nil {
		return
	}
	go func() {
		err := a.svc.SetLanguage(lang)
		a.postSettings(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.languageValue = lang
			a.syncLanguage()
			a.refreshAll()
		})
	}()
}

func (a *App) settingsThemeCard(c *ui.Context) {
	options := []SelectOption{
		{Value: core.ThemeSystem, Label: a.t("settings.appearance.auto")},
		{Value: core.ThemeLight, Label: a.t("settings.appearance.light")},
		{Value: core.ThemeDark, Label: a.t("settings.appearance.dark")},
	}
	a.settingsCard(c, "", func() {
		a.settingsRowInfo(c, a.t("settings.appearance.themeTitle"), a.t("settings.appearance.themeDescription"), func() {
			mode, changed := a.dropdown(c, "appearance-mode", a.themeValue, options)
			if !changed {
				return
			}
			// Persist synchronously: the write is a few hundred bytes, and the
			// user may quit right after picking a mode. Leaving it to a goroutine
			// can drop the selection before the process exits.
			if err := a.svc.SetTheme(mode); err != nil {
				a.showToast(err.Error())
				return
			}
			a.themeValue = mode
		})
	})
}

func (a *App) settingsAppearance(c *ui.Context) {
	p := a.palette(c)
	a.settingsThemeCard(c)
	a.settingsThemeSelection(c)
	a.settingsCard(c, a.t("settings.vibrancy.title"), func() {
		a.settingsRowInfo(c, a.t("settings.vibrancy.enable"), a.t("settings.vibrancy.enableDescription"), func() {
			on := a.vibrancyValue
			if ui.Switch(c.Key("vibrancy"), &on).Changed() {
				a.vibrancyValue = on
				go func() {
					err := a.svc.SetVibrancyEnabled(on)
					a.postSettings(func() {
						if err != nil {
							a.showToast(err.Error())
						}
						a.refreshAll()
					})
				}()
			}
		})
		// The detailed choices only exist while the material itself is on;
		// with the switch off every surface is opaque, so they would be moot.
		if a.vibrancyValue {
			ui.Box(c).FillWidth().Height(1).Background(p.hairline)
			a.settingsRowInfo(c, a.t("settings.vibrancy.sidebar"), a.t("settings.vibrancy.sidebarDescription"), func() {
				on := a.prefs.VibrancySidebar
				if ui.Switch(c.Key("vibrancy-sidebar"), &on).Changed() {
					a.prefs.VibrancySidebar = on
					a.savePrefs()
					a.invalidateWindows()
				}
			})
			a.settingsRowInfo(c, a.t("settings.vibrancy.titlebar"), a.t("settings.vibrancy.titlebarDescription"), func() {
				on := a.prefs.VibrancyHeader
				if ui.Switch(c.Key("vibrancy-titlebar"), &on).Changed() {
					a.prefs.VibrancyHeader = on
					a.savePrefs()
					a.invalidateWindows()
				}
			})
		}
	})
	a.settingsCard(c, a.t("settings.layout"), func() {
		a.settingsRow(c, a.t("settings.rowHeight"), "", func() {
			val := float64(a.rowHeight)
			ui.Slider(c.Key("row-height"), &val, 90, 420).Width(180)
			if int(val) != a.rowHeight {
				a.setRowHeight(int(val))
			}
		})
		a.settingsRow(c, a.t("contextMenu.showSidebar"), "", func() {
			on := a.prefs.SidebarOpen
			if ui.Switch(c.Key("show-sidebar"), &on).Changed() {
				a.prefs.SidebarOpen = on
				a.savePrefs()
			}
		})
		a.settingsRow(c, a.t("contextMenu.showInspector"), "", func() {
			on := a.prefs.InspectorOpen
			if ui.Switch(c.Key("show-inspector"), &on).Changed() {
				a.prefs.InspectorOpen = on
				a.savePrefs()
			}
		})
		a.settingsRow(c, a.t("contextMenu.showName"), "", func() {
			on := a.prefs.ShowNames
			if ui.Switch(c.Key("show-names"), &on).Changed() {
				a.prefs.ShowNames = on
				a.savePrefs()
			}
		})
		a.settingsRow(c, a.t("contextMenu.showExtension"), "", func() {
			on := a.prefs.ShowExt
			if ui.Switch(c.Key("show-ext"), &on).Changed() {
				a.prefs.ShowExt = on
				a.savePrefs()
			}
		})
		a.settingsRowInfo(c, a.t("settings.appearance.variantDisplay"), a.t("settings.appearance.variantDisplayDescription"), func() {
			current := a.prefs.VariantDisplayMode
			if current == "" {
				current = "stacked"
			}
			variantOptions := []SelectOption{
				{Value: "stacked", Label: a.t("settings.appearance.variantStacked")},
				{Value: "flat", Label: a.t("settings.appearance.variantFlat")},
			}
			mode, changed := a.dropdown(c, "variant-display-mode", current, variantOptions)
			if changed && mode != current {
				a.prefs.VariantDisplayMode = mode
				a.savePrefs()
				a.refreshAll()
			}
		})
	})
}

// settingsThemeSelection draws the two variant pickers shared by the
// Appearance and Themes pages.
func (a *App) settingsThemeSelection(c *ui.Context) {
	a.settingsCardHelp(c, a.t("settings.themes.selection"), a.t("settings.themes.followHint"), func() {
		a.settingsRowInfo(c, a.t("settings.themes.light"), "", func() {
			if id, changed := a.dropdown(c, "theme-light", a.prefs.LightTheme, a.themeOptions(theme.Light)); changed {
				a.prefs.LightTheme = id
				a.applyThemeSelection()
			}
		})
		a.settingsRowInfo(c, a.t("settings.themes.dark"), "", func() {
			if id, changed := a.dropdown(c, "theme-dark", a.prefs.DarkTheme, a.themeOptions(theme.Dark)); changed {
				a.prefs.DarkTheme = id
				a.applyThemeSelection()
			}
		})
		a.settingsRowInfo(c, a.t("settings.themes.systemAccent"), a.t("settings.themes.systemAccentDescription"), func() {
			on := a.prefs.SystemAccent
			if ui.Switch(c.Key("system-accent"), &on).Changed() {
				a.prefs.SystemAccent = on
				a.savePrefs()
				a.syncSystemAccent(c)
				a.pushTheme()
			}
		})
	})
}

// settingsThemes is the theme page: choice of light and dark variants, the
// JSON folder, and a colour editor for user themes. Built-in defaults are
// read-only so the shipped palette cannot be deleted or replaced.
func (a *App) settingsThemes(c *ui.Context) {
	a.settingsThemeSelection(c)
	a.settingsCardHelp(c, a.t("settings.themes.files"), a.t("settings.themes.filesHint"), func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("themes-open"), a.t("settings.themes.openFolder")).Clicked() {
				if a.svc != nil {
					_ = a.svc.OpenFolder(a.ensureThemesDir())
				}
			}
			if ui.Button(c.Key("themes-reload"), a.t("settings.themes.reload")).Clicked() {
				a.reloadThemes()
				a.showToast(a.t("settings.themes.reloaded"))
			}
		})
	})
	if a.themeEditID == "" {
		a.themeEditID = a.activeTheme(c).ID
	}
	selected := a.themeIndex[a.themeEditID]
	editHelp := a.t("settings.themes.editHint")
	if selected != nil && selected.Builtin {
		editHelp = a.t("settings.themes.builtin")
	}
	a.settingsCardHelp(c, a.t("settings.themes.edit"), editHelp, func() {
		a.settingsRowInfo(c, a.t("settings.themes.editTheme"), "", func() {
			options := make([]SelectOption, 0, len(a.themes))
			for _, t := range a.themes {
				options = append(options, SelectOption{Value: t.ID, Label: t.ID})
			}
			if id, changed := a.dropdown(c, "theme-pick", a.themeEditID, options); changed {
				a.themeEditID = id
				a.themeDraft = nil
				a.themeDirty = false
			}
		})
		if selected == nil {
			return
		}
		if selected.Builtin {
			if ui.Button(c.Key("theme-duplicate"), a.t("settings.themes.duplicate")).Clicked() {
				a.duplicateTheme(selected)
			}
			return
		}
		draft := a.themeEditorCopy(selected)
		for _, tok := range themeColorTokens {
			tok := tok
			control := func() {
				value := tok.Field(&draft.Colors)
				col, err := theme.ParseColor(*value)
				if err != nil {
					col = ui.Transparent
				}
				well := ui.ColorWell(c, &col)
				if well.Changed() {
					*value = theme.FormatColor(col)
					a.themeDirty = true
				}
			}
			label := a.t("settings.themes.colors." + tok.Label)
			if tok.Key == "accent" && a.prefs.SystemAccent {
				a.settingsRowInfo(c, label, a.t("settings.themes.accentOverridden"), control)
			} else {
				a.settingsRow(c, label, "", control)
			}
		}
		a.settingsRow(c, a.t("settings.themes.material.sidebarOpacity"), "", func() {
			val := float64(draft.Material.SidebarOpacity)
			ui.Slider(c.Key("theme-sidebar-opacity"), &val, 0, 1).Width(180)
			if float32(val) != draft.Material.SidebarOpacity {
				draft.Material.SidebarOpacity = float32(val)
				a.themeDirty = true
			}
		})
		a.settingsRow(c, a.t("settings.themes.material.headerTint"), "", func() {
			col, err := theme.ParseColor(draft.Material.HeaderTint)
			if err != nil {
				col = ui.Transparent
			}
			well := ui.ColorWell(c, &col)
			if well.Changed() {
				draft.Material.HeaderTint = theme.FormatColor(col)
				a.themeDirty = true
			}
		})
		a.settingsRow(c, a.t("settings.themes.material.headerBlur"), "", func() {
			val := float64(draft.Material.HeaderBlur)
			ui.Slider(c.Key("theme-header-blur"), &val, 0, 40).Width(180)
			if float32(val) != draft.Material.HeaderBlur {
				draft.Material.HeaderBlur = float32(val)
				a.themeDirty = true
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c.Key("theme-reset"), a.t("settings.themes.reset")).Clicked() {
				a.resetThemeDraft()
			}
			if ui.Button(c.Key("theme-revert"), a.t("settings.themes.revert")).Clicked() {
				a.themeDraft = nil
				a.themeDirty = false
			}
			if ui.PrimaryButton(c.Key("theme-save"), a.t("settings.themes.save")).Clicked() {
				a.saveThemeDraft()
			}
			if a.themeDirty {
				a.infoIcon(c, a.t("settings.themes.unsaved"))
			}
		})
	})
}

// settingsShortcuts renders the whole keyboard map: global hotkeys, the
// rebindable in-app actions, and the fixed basic bindings. Fixed rows show
// their keys but offer no field, so the list doubles as documentation.
func (a *App) settingsShortcuts(c *ui.Context) {
	p := a.palette(c)
	a.settingsCard(c, a.t("settings.shortcuts.title"), func() {
		ui.Text(c, a.t("settings.shortcuts.description")).FontSize(11).TextColor(p.faint).Wrap()
	})
	for _, section := range core.ShortcutSections() {
		section := section
		a.settingsCard(c, a.t("settings.shortcuts."+section.Key+"Group"), func() {
			for _, def := range section.Defs {
				def := def
				a.shortcutRow(c, def)
			}
		})
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, a.t("settings.shortcutsReset")).Clicked() {
			go func() {
				err := a.svc.SetShortcutSettings(core.DefaultShortcutSettings())
				a.postSettings(func() {
					if err != nil {
						a.showToast(err.Error())
					} else {
						a.refreshSettings()
					}
				})
			}()
		}
		ui.Text(c, a.t("settings.shortcutsHelp")).FontSize(10.5).TextColor(p.faint)
	})
}

// shortcutRow draws one catalog entry: a button that records a new
// combination, or the read-only badge of a fixed binding.
func (a *App) shortcutRow(c *ui.Context, def core.ShortcutDef) {
	label := a.shortcutLabel(def.ID)
	p := a.palette(c)
	if def.Fixed {
		a.settingsRow(c, label, "", func() { a.shortcutValue(c, def.ID, 11) })
		return
	}
	a.settingsRow(c, label, "", func() {
		record := ui.ButtonBase(c.Key("shortcut-" + def.ID)).Height(30).MinWidth(80).PaddingX(4).Radius(7).Background(ui.Transparent).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center)
		if record.Hovered() {
			record.Background(p.hover)
		}
		record.Children(func() { a.shortcutValue(c, def.ID, 11) })
		if record.Clicked() {
			a.openShortcutDialog(def.ID)
		}
	})
}

// shortcutValue draws one binding as separate key caps, or the "Not set" chip
// when the action has no shortcut.
func (a *App) shortcutValue(c *ui.Context, id string, fontSize float32) {
	accel := a.shortcuts.Accelerator(id)
	if core.AcceleratorDisplay(accel) == "" {
		a.shortcutNotSetChip(c, fontSize)
		return
	}
	a.shortcutCaps(c, accel, fontSize)
}

// shortcutCaps draws an accelerator as one small square per key, joined by
// plus signs. An empty accelerator draws nothing.
func (a *App) shortcutCaps(c *ui.Context, accel string, fontSize float32) {
	keys := core.AcceleratorKeys(accel)
	if len(keys) == 0 {
		return
	}
	p := a.palette(c)
	height := fontSize + 13
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		for i, key := range keys {
			if i > 0 {
				ui.Text(c, "+").FontSize(fontSize).TextColor(p.faint)
			}
			cap := ui.Box(c).MinWidth(height).Height(height).PaddingX(5).Radius(5).Background(p.surface2).Border(1, p.hairline).AlignItems(ui.Center).Justify(ui.Center)
			cap.Children(func() {
				ui.Text(c, key).FontSize(fontSize).TextColor(p.muted).FontFeatures("tnum")
			})
		}
	})
}

// shortcutNotSetChip is the placeholder for an action without a shortcut.
func (a *App) shortcutNotSetChip(c *ui.Context, fontSize float32) {
	p := a.palette(c)
	height := fontSize + 13
	ui.Box(c).Height(height).PaddingX(8).Radius(5).Background(ui.Transparent).Border(1, p.hairline).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Text(c, a.t("settings.shortcuts.unset")).FontSize(fontSize).TextColor(p.faint)
	})
}

func (a *App) openShortcutDialog(id string) {
	a.shortcutEditID = id
	a.shortcutDraft = a.shortcuts.Accelerator(id)
	a.checkShortcutDraft()
}

// checkShortcutDraft reports the other action already using the recorded
// combination, as soon as the keys come in rather than at save time.
func (a *App) checkShortcutDraft() {
	a.shortcutEditError = ""
	if a.shortcutEditID == "" || a.shortcutDraft == "" {
		return
	}
	other := core.ShortcutConflict(a.shortcuts, a.shortcutEditID, a.shortcutDraft)
	if other == "" {
		return
	}
	a.shortcutEditError = a.tf("settings.shortcuts.conflict", map[string]any{
		"action": a.shortcutLabel(other),
	})
}

func (a *App) cancelShortcutDialog() {
	a.shortcutEditID = ""
	a.shortcutDraft = ""
	a.shortcutEditError = ""
}

// applyShortcutDraft saves the recorded accelerator, or clears the binding
// when accel is empty. A conflict keeps the recorder open with the reason.
func (a *App) applyShortcutDraft(accel string) {
	id := a.shortcutEditID
	if id == "" {
		return
	}
	// The recorder already warned while the keys came in; saving a conflict
	// would only fail in the service, so stop here.
	a.shortcutDraft = core.NormalizeAccelerator(accel)
	a.checkShortcutDraft()
	if a.shortcutEditError != "" {
		return
	}
	next := core.NormalizeShortcutSettings(a.shortcuts)
	next.Bindings[id] = a.shortcutDraft
	if a.svc != nil {
		if err := a.svc.SetShortcutSettings(next); err != nil {
			a.shortcutEditError = err.Error()
			return
		}
	}
	a.shortcuts = next
	a.rebuildShortcutPairs()
	a.cancelShortcutDialog()
	a.showToast(a.t("common.saved"))
}

// viewShortcutDialog is the recorder: the pressed keys show as they come in,
// Esc cancels, No shortcut clears the binding and Save keeps what was pressed.
func (a *App) viewShortcutDialog(c *ui.Context) {
	p := a.palette(c)
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		panel.Width(400).Padding(22).Radius(14).Background(p.surface).Border(1, p.hairline).Shadow(0, 26, 64, -18, p.cardShade).Children(func() {
			ui.Column(c).FillWidth().Gap(14).Children(func() {
				ui.Text(c, a.t("settings.shortcuts.editTitle")).FontSize(17).FontWeight(650).TextColor(p.text)
				ui.Text(c, a.shortcutLabel(a.shortcutEditID)).FontSize(12).TextColor(p.muted)
				border := p.accent
				if a.shortcutEditError != "" {
					border = p.danger
				}
				capture := ui.Box(c.Key("shortcut-capture-"+a.shortcutEditID)).FillWidth().Height(64).Radius(10).Background(p.surface2).Border(1, border).AlignItems(ui.Center).Justify(ui.Center).Focusable().Focus()
				capture.HandleInput(func(ev ui.InputEvent) bool {
					switch ev.Kind {
					case ui.InputKeyDown:
						if ev.Key == ui.KeyEscape {
							a.cancelShortcutDialog()
							open = false
							return true
						}
						if accel, ok := acceleratorFromUI(ev.Mods, ev.Key); ok {
							a.shortcutDraft = accel
							a.checkShortcutDraft()
						}
						return true
					case ui.InputKeyUp:
						// Check again once the keys settle too: the warning
						// follows the last combination pressed.
						a.checkShortcutDraft()
						return true
					case ui.InputText, ui.InputCompose, ui.InputCommand:
						// Recording takes every key, so nothing behind the
						// recorder acts on what is pressed.
						return true
					}
					return false
				})
				capture.Children(func() {
					accel := shortcutPreviewAccel(c.Modifiers(), a.shortcutDraft)
					if accel == "" {
						ui.Text(c, a.t("settings.shortcuts.recording")).FontSize(13).TextColor(p.faint)
						return
					}
					a.shortcutCaps(c, accel, 13)
				})
				ui.Text(c, a.t("settings.shortcuts.editHint")).FontSize(10.5).TextColor(p.faint).Wrap()
				if a.shortcutEditError != "" {
					ui.Text(c, a.shortcutEditError).FontSize(11).TextColor(p.danger).Wrap()
				}
				ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
					if ui.Button(c, a.t("settings.shortcuts.unsetAction")).Clicked() {
						a.applyShortcutDraft("")
					}
					if ui.Button(c, a.t("common.cancel")).Clicked() {
						a.cancelShortcutDialog()
					}
					if ui.PrimaryButton(c, a.t("common.save")).Disabled(a.shortcutDraft == "" || a.shortcutEditError != "").Clicked() {
						a.applyShortcutDraft(a.shortcutDraft)
					}
				})
			})
		})
	})
	if !open {
		a.cancelShortcutDialog()
	}
}

func (a *App) shortcutLabel(id string) string {
	key := "shortcuts." + id
	if label := a.t(key); label != key {
		return label
	}
	return id
}

func (a *App) settingsPlugins(c *ui.Context) {
	p := a.palette(c)
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c.Key("plugin-install"), a.t("settings.plugins.install")).Clicked() {
			a.dialog = dialogPluginInstall
			a.dialogValue = ""
			a.pendingInstall = a.svc.GetPendingPluginInstall()
		}
		if ui.Button(c.Key("plugin-refresh"), a.t("settings.plugins.refresh")).Disabled(a.pluginScanning || a.pluginHost == nil).Clicked() {
			a.rescanPlugins()
		}
		if ui.Button(c, a.t("settings.plugins.openFolder")).Clicked() {
			_ = a.svc.OpenPluginsFolder()
		}
		if ui.Button(c, a.t("settings.plugins.marketplace")).Clicked() {
			_ = a.svc.OpenMarketplace()
		}
	})
	ui.Box(c).Height(8)
	for i := range a.plugins {
		pl := &a.plugins[i]
		a.settingsCard(c, pl.Name, func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, pl.ID).FontSize(11).TextColor(p.muted)
					if pl.Native != "" {
						ui.Text(c, a.t("settings.plugins.native")).FontSize(10).TextColor(p.faint)
					}
				})
				if pl.Builtin {
					ui.Badge(c, a.t("settings.plugins.builtin")).FontSize(9)
				}
				ui.Text(c, "v"+pl.Version).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
				// Bind the switch to the row stored in a.plugins, not to a
				// loop copy. A copy made the switch snap back to its old
				// value on the next frame, which looked like the plugin
				// could not be disabled.
				on := pl.Enabled
				if ui.Switch(c.Key("plugin-"+pl.ID), &on).Changed() {
					if pl.Hosted && a.pluginHost != nil {
						a.pluginHost.SetEnabled(pl.ID, on)
					} else if a.svc != nil {
						_ = a.svc.SetPluginEnabled(pl.ID, on)
					}
					pl.Enabled = on
					if a.win != nil {
						a.win.Invalidate()
					}
					if on {
						// Enabling a preview plugin may unlock cover generation for
						// items already loaded on the page.
						a.post(func() { a.backfillMissingThumbnails(a.items) })
					}
				}
			})
			// Plugin settings rendered from the manifest.
			if a.pluginHost != nil {
				if hosted, ok := a.pluginHost.Get(pl.ID); ok {
					if len(hosted.Manifest.UI.Windows)+len(hosted.Manifest.UI.Panels) > 0 {
						if len(hosted.Manifest.Permissions.Network) > 0 {
							on := a.pluginHost.UIGranted(pl.ID, "network")
							a.settingsRow(c, a.t("settings.plugins.allowNetwork"), strings.Join(hosted.Manifest.Permissions.Network, ", "), func() {
								if ui.Switch(c.Key("plugin-network-"+pl.ID), &on).Changed() {
									a.pluginHost.SetUIGrant(pl.ID, "network", on)
								}
							})
						}
						for _, capability := range hosted.Manifest.Permissions.Assets {
							capability := capability
							grant := "assets." + capability
							on := a.pluginHost.UIGranted(pl.ID, grant)
							label := a.t("settings.plugins.allowAssetRead")
							if capability == "write" {
								label = a.t("settings.plugins.allowAssetWrite")
							}
							a.settingsRow(c, label, "", func() {
								if ui.Switch(c.Key("plugin-grant-"+pl.ID+grant), &on).Changed() {
									a.pluginHost.SetUIGrant(pl.ID, grant, on)
								}
							})
						}
					}
					for _, setting := range hosted.Manifest.Settings {
						setting := setting
						val, exists := hosted.Settings[setting.Key]
						if !exists {
							val = setting.Default
						}
						fieldID := pl.ID + ":" + setting.Key
						a.settingsRow(c, setting.Label, setting.Help, func() {
							ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
								input := ui.TextInput(c.Key("plugin-setting-"+fieldID), &val).Width(220).FontSize(12)
								if setting.Placeholder != "" {
									input.Placeholder(setting.Placeholder)
								}
								if setting.Secret && !a.pluginSecretVisible[fieldID] {
									input.Password()
								}
								input.OnChange(func() {
									a.pluginHost.SetSetting(pl.ID, setting.Key, val)
								})
								if setting.Secret {
									label := a.t("settings.plugins.showSecret")
									if a.pluginSecretVisible[fieldID] {
										label = a.t("settings.plugins.hideSecret")
									}
									a.iconButton(c, "plugin-secret-"+fieldID, iconEye, a.pluginSecretVisible[fieldID], label, func() {
										if a.pluginSecretVisible == nil {
											a.pluginSecretVisible = make(map[string]bool)
										}
										a.pluginSecretVisible[fieldID] = !a.pluginSecretVisible[fieldID]
									})
								}
							})
						})
					}
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				if a.pluginHost != nil {
					if hosted, ok := a.pluginHost.Get(pl.ID); ok && hosted.Enabled {
						for _, window := range hosted.Manifest.UI.Windows {
							window := window
							label := window.Title
							if label == "" {
								label = a.t("settings.plugins.openPluginWindow")
							}
							if ui.Button(c.Key("plugin-window-"+pl.ID+"-"+window.ID), label).Clicked() {
								if err := a.openPluginUI(pl.ID, window); err != nil {
									a.showToast(err.Error())
								}
							}
						}
					}
				}
				if ui.Button(c, a.t("settings.plugins.openStore")).Clicked() {
					a.openPluginStore(pl.ID)
				}
				if pl.CanUninstall && ui.Button(c, a.t("settings.plugins.uninstall")).Clicked() {
					a.uninstallPlugin(pl.ID)
				}
			})
		})
	}
}

// rescanPlugins discovers manually added or removed plugins without blocking the settings UI.
func (a *App) rescanPlugins() {
	if a.pluginHost == nil || a.pluginScanning {
		return
	}
	a.pluginScanning = true
	go func() {
		a.pluginHost.Scan()
		var installed []core.InstalledPlugin
		if a.svc != nil {
			installed, _ = a.svc.ListInstalledPlugins()
		}
		a.postSettings(func() {
			a.plugins = a.pluginRows(installed)
			a.pluginScanning = false
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}()
}

// openPluginStore opens the marketplace detail page for one plugin.
func (a *App) openPluginStore(id string) {
	if a.svc == nil {
		return
	}
	go func() {
		if err := a.svc.OpenPluginMarketplace(id); err != nil {
			a.postSettings(func() { a.showToast(err.Error()) })
		}
	}()
}

// uninstallPlugin removes both the native host copy and the legacy plugin
// registry entry. Either side may be absent, so "not installed" is ignored.
func (a *App) uninstallPlugin(id string) {
	go func() {
		var failure error
		if a.pluginHost != nil && a.pluginHost.CanUninstall(id) {
			if err := a.pluginHost.Uninstall(id); err != nil {
				failure = err
			}
		}
		if a.svc != nil {
			if err := a.svc.UninstallPlugin(id); err != nil && failure == nil &&
				!strings.Contains(strings.ToLower(err.Error()), "not installed") &&
				!strings.Contains(strings.ToLower(err.Error()), "not found") {
				failure = err
			}
		}
		a.postSettings(func() {
			if failure != nil {
				a.showToast(failure.Error())
				return
			}
			a.showToast(a.t("settings.plugins.uninstalled"))
			a.refreshSettings()
			a.refreshAll()
		})
	}()
}

func (a *App) settingsMobile(c *ui.Context) {
	p := a.palette(c)
	status := a.mobile
	a.settingsCard(c, a.t("settings.mobile.server"), func() {
		a.settingsRow(c, a.t("settings.mobile.enabled"), a.t("settings.mobile.enabledHelp"), func() {
			on := status != nil && status.Enabled
			if ui.Switch(c.Key("mobile-enabled"), &on).Changed() {
				go func() {
					err := a.svc.SetMobileServerEnabled(on)
					a.postSettings(func() {
						if err != nil {
							a.showToast(err.Error())
						}
						a.refreshSettings()
					})
				}()
			}
		})
		a.settingsRow(c, a.t("settings.mobile.shareNotify"), a.t("settings.mobile.shareNotifyHelp"), func() {
			notifyOn := a.svc != nil && a.svc.IsMobileShareNotifyEnabled()
			if ui.Switch(c.Key("mobile-share-notify"), &notifyOn).Changed() {
				go func() {
					err := a.svc.SetMobileShareNotifyEnabled(notifyOn)
					a.postSettings(func() {
						if err != nil {
							a.showToast(err.Error())
						}
						a.refreshSettings()
					})
				}()
			}
		})
		if status != nil {
			label := a.t("settings.mobile.stopped")
			if status.Running {
				label = a.tf("settings.mobile.running", map[string]any{"port": status.Port})
			}
			a.settingsRow(c, a.t("settings.mobile.status"), "", func() {
				ui.Text(c, label).FontSize(11).TextColor(p.muted).FontFeatures("tnum")
			})
			a.settingsRow(c, a.t("settings.mobile.port"), a.t("settings.mobile.portHelp"), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c.Key("mobile-port"), &a.mobilePort).Width(88).FontSize(12).FontFeatures("tnum").Placeholder("17878").OnSubmit(func() {
						a.applyMobilePort()
					})
					if ui.Button(c, a.t("settings.mobile.applyPort")).Clicked() {
						a.applyMobilePort()
					}
				})
			})
			a.settingsRow(c, a.t("settings.mobile.pairingCode"), "", func() {
				ui.Text(c, status.PairingCode).FontSize(16).FontWeight(700).FontFeatures("tnum").TextColor(p.text)
				if ui.Button(c, a.t("settings.mobile.regenerate")).Clicked() {
					go func() {
						_, err := a.svc.RegenerateMobilePairingCode()
						a.postSettings(func() {
							if err != nil {
								a.showToast(err.Error())
							}
							a.refreshSettings()
						})
					}()
				}
			})
			a.settingsRow(c, a.t("settings.mobile.addresses"), "", func() {
				ui.Text(c, strings.Join(status.Addresses, "  ")).FontSize(11).TextColor(p.muted).MaxWidth(360).Wrap()
			})
		}
	})
	a.settingsCardHelp(c, a.t("settings.mobile.discoveryNetworks"), a.t("settings.mobile.discoveryNetworksDesc"), func() {
		networks := []core.NetworkInterfaceInfo{}
		if status != nil {
			networks = status.AvailableNetworks
		}
		if len(networks) == 0 {
			ui.Text(c, a.t("settings.mobile.noNetworksFound")).FontSize(11.5).TextColor(p.faint)
			return
		}
		for _, network := range networks {
			network := network
			label := fmt.Sprintf("%s · %s", network.ID, strings.Join(network.IPs, ", "))
			a.settingsRow(c, label, a.mobileNetworkTypeLabel(network.Type), func() {
				on := network.Selected
				if ui.Switch(c.Key("mobile-network-"+network.ID), &on).Changed() {
					a.setMobileNetworkEnabled(network.ID, on)
				}
			})
		}
	})
	a.settingsCard(c, a.t("settings.mobile.qrTitle"), func() {
		ui.Row(c).FillWidth().Gap(20).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).Gap(8).Children(func() {
				ui.Text(c, a.t("settings.mobile.qrDesc")).FontSize(11.5).TextColor(p.muted).Wrap()
				if a.mobilePairingURL != "" {
					if ui.Button(c, a.t("settings.mobile.copyUrl")).Clicked() {
						_ = a.svc.WriteClipboardText(a.mobilePairingURL)
						a.showToast(a.t("settings.mobile.copiedUrl"))
					}
				}
				ui.Text(c, a.t("settings.mobile.qrTip")).FontSize(10.5).TextColor(p.faint).Wrap()
			})
			if status != nil && status.Running && len(status.Addresses) > 0 && a.mobileQR != nil {
				ui.Box(c).Padding(8).Radius(8).Background(ui.RGB(255, 255, 255)).Children(func() {
					ui.Image(c, a.mobileQR).Size(176, 176).Fit(ui.Contain)
				})
			} else if status != nil && status.Running {
				ui.Text(c, a.t("settings.mobile.noNetworksSelected")).FontSize(11.5).TextColor(p.warning).MaxWidth(220).Wrap()
			} else {
				ui.Text(c, a.t("settings.mobile.qrUnavailable")).FontSize(11.5).TextColor(p.faint).MaxWidth(220).Wrap()
			}
		})
	})
	a.settingsCard(c, a.t("settings.mobile.devices"), func() {
		if len(a.mobileDevices) == 0 {
			ui.Text(c, a.t("settings.mobile.noDevices")).FontSize(11.5).TextColor(p.faint)
			return
		}
		for _, dev := range a.mobileDevices {
			dev := dev
			a.settingsRow(c, dev.Name, fmt.Sprintf("%s · %s", dev.TokenHint, dev.Source), func() {
				if ui.Button(c, a.t("settings.mobile.revoke")).Clicked() {
					go func() {
						err := a.svc.RevokeMobileDevice(dev.ID)
						a.postSettings(func() {
							if err != nil {
								a.showToast(err.Error())
							}
							a.refreshSettings()
						})
					}()
				}
			})
		}
	})
}

// mobileNetworkTypeLabel names the interface class shown in the discovery
// network list.
func (a *App) mobileNetworkTypeLabel(networkType string) string {
	switch networkType {
	case "lan":
		return a.t("settings.mobile.netLan")
	case "tailscale":
		return a.t("settings.mobile.netTailscale")
	default:
		return a.t("settings.mobile.netOther")
	}
}

// setMobileNetworkEnabled persists a new discovery-network selection and
// refreshes the pairing QR code so it only advertises the exposed addresses.
func (a *App) setMobileNetworkEnabled(ifaceID string, enabled bool) {
	if a.mobile == nil {
		return
	}
	allowed := make([]string, 0, len(a.mobile.AvailableNetworks))
	for i := range a.mobile.AvailableNetworks {
		network := &a.mobile.AvailableNetworks[i]
		selected := network.Selected
		if network.ID == ifaceID {
			selected = enabled
			network.Selected = enabled
		}
		if selected {
			allowed = append(allowed, network.ID)
		}
	}
	a.mobileNetworkWanted = append([]string{}, allowed...)
	if a.mobileNetworkSaving {
		return
	}
	a.mobileNetworkSaving = true
	a.saveMobileNetworks()
}

// saveMobileNetworks persists the latest discovery-network selection. A
// toggle that arrives while a save is in flight only replaces the wanted
// list; the worker applies it after the current request finishes.
func (a *App) saveMobileNetworks() {
	wanted := append([]string{}, a.mobileNetworkWanted...)
	go func() {
		err := a.svc.SetMobileAllowedNetworks(wanted)
		a.postSettings(func() {
			if !sameNetworkSelection(a.mobileNetworkWanted, wanted) {
				a.saveMobileNetworks()
				return
			}
			a.mobileNetworkSaving = false
			if err != nil {
				a.showToast(err.Error())
			}
			a.refreshSettings()
		})
	}()
}

func sameNetworkSelection(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// applyMobilePort validates the port field and switches the LAN listener.
func (a *App) applyMobilePort() {
	port, err := strconv.Atoi(strings.TrimSpace(a.mobilePort))
	if err != nil || port < 1 || port > 65535 {
		a.showToast(a.t("settings.mobile.portInvalid"))
		return
	}
	go func() {
		err := a.svc.SetMobileServerPort(port)
		a.postSettings(func() {
			if err != nil {
				a.showToast(err.Error())
				a.refreshSettings()
				if a.mobile != nil {
					a.mobilePort = strconv.Itoa(a.mobile.Port)
				}
				return
			}
			a.showToast(a.tf("settings.mobile.portChanged", map[string]any{"port": port}))
			a.refreshSettings()
			if a.mobile != nil {
				a.mobilePort = strconv.Itoa(a.mobile.Port)
			}
		})
	}()
}

func (a *App) settingsNetwork(c *ui.Context) {
	p := a.palette(c)
	a.settingsCard(c, a.t("settings.network.title"), func() {
		ui.Row(c).Gap(6).Children(func() {
			for _, mode := range []string{core.ProxyModeSystem, core.ProxyModeDirect, core.ProxyModeCustom} {
				mode := mode
				label := map[string]string{core.ProxyModeSystem: a.t("settings.network.system"), core.ProxyModeDirect: a.t("settings.network.direct"), core.ProxyModeCustom: a.t("settings.network.custom")}[mode]
				a.pillButton(c, "proxy-"+mode, label, a.network.ProxyMode == mode, func() {
					a.network.ProxyMode = mode
				})
			}
		})
		if a.network.ProxyMode == core.ProxyModeCustom {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.t("settings.network.address")).FontSize(11.5).TextColor(p.muted)
				ui.TextInput(c.Key("proxy-address"), &a.network.CustomProxy).Grow(1).FontSize(12).Placeholder("http://127.0.0.1:7890")
			})
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, a.t("common.save")).Clicked() {
				net := a.network
				go func() {
					err := a.svc.SetNetworkSettings(net)
					a.postSettings(func() {
						if err != nil {
							a.showToast(err.Error())
						} else {
							a.showToast(a.t("common.saved"))
						}
					})
				}()
			}
			if ui.Button(c, a.t("settings.network.test")).Clicked() {
				go func() {
					res, err := a.svc.TestNetworkConnection("https://www.google.com")
					a.postSettings(func() {
						if err != nil {
							a.showToast(err.Error())
						} else if res != nil && res.Success {
							a.showToast(fmt.Sprintf("%s · %dms", a.t("settings.network.success"), res.LatencyMs))
						} else if res != nil {
							a.showToast(res.Error)
						}
					})
				}()
			}
		})
	})
}

func (a *App) settingsStorage(c *ui.Context) {
	a.settingsCard(c, a.t("settings.storage.title"), func() {
		a.settingsRow(c, a.t("settings.storage.library"), a.libraryPath(), func() {
			if ui.Button(c, a.t("settings.storage.open")).Clicked() {
				if a.library != nil {
					_ = a.svc.OpenFolder(a.library.Path)
				}
			}
		})
		a.settingsRow(c, a.t("settings.storage.userData"), a.svc.GetUserDataDir(), func() {
			if ui.Button(c, a.t("settings.storage.open")).Clicked() {
				_ = a.svc.OpenUserDataFolder()
			}
		})
		a.settingsRow(c, a.t("settings.storage.plugins"), a.svc.GetPluginsDir(), func() {
			if ui.Button(c, a.t("settings.storage.open")).Clicked() {
				_ = a.svc.OpenPluginsFolder()
			}
		})
		a.settingsRow(c, a.t("settings.storage.logs"), a.svc.GetLogFilePath(), func() {
			if ui.Button(c, a.t("settings.storage.reveal")).Clicked() {
				_ = a.svc.RevealLogFile()
			}
		})
		if a.library != nil {
			a.settingsRow(c, a.t("settings.storage.items"), fmt.Sprintf("%d · %s", a.stats.Total, formatBytes(a.stats.TotalSize)), func() {})
		}
	})
}

func (a *App) libraryPath() string {
	if a.library == nil {
		return ""
	}
	return a.library.Path
}

func (a *App) settingsAbout(c *ui.Context) {
	p := a.palette(c)
	ui.Column(c).FillWidth().Center().Gap(10).PaddingY(30).Children(func() {
		ui.Box(c).Size(72, 72).Radius(18).Background(p.surface2).Center().Children(func() {
			ui.Icon(c, iconLibrary).Size(34, 34).TextColor(p.accent)
		})
		ui.Text(c, "Bowerbird").FontSize(22).FontWeight(700).TextColor(p.text)
		ui.Text(c, "1.0.0-alpha-1").FontSize(12).TextColor(p.muted)
		ui.Textf(c, "Go %s · %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH).FontSize(11).TextColor(p.faint)
		ui.Text(c, a.t("settings.aboutText")).FontSize(12).TextColor(p.muted).MaxWidth(420).TextAlign(ui.Center).Wrap()
	})
}
