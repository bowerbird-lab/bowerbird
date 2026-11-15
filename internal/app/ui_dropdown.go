package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// SelectOption is one choice of a Dropdown: Value is what the app stores and
// Label is the text the user sees. Keeping the two apart lets translated
// labels change without touching persisted values.
type SelectOption struct {
	Value string
	Label string
}

// Dropdown draws a reusable drop-down select: a trigger showing the current
// choice, and a popup listing the options. The widget keeps no selection of
// its own: the caller owns the value and gets the newly chosen one back.
//
//	lang, changed := a.dropdown(c, "settings-language", a.languageValue, []SelectOption{
//		{Value: "system", Label: a.t("settings.language.auto")},
//		{Value: "en", Label: a.t("settings.language.en")},
//		{Value: "zh", Label: a.t("settings.language.zh")},
//	})
//	if changed {
//		a.setLanguage(lang)
//	}
//
// The key keeps the popup's open/highlight state stable between frames; it must
// be unique among the widgets of the window that are visible at the same time.
func (a *App) dropdown(c *ui.Context, key any, selected string, options []SelectOption) (string, bool) {
	if len(options) == 0 {
		return selected, false
	}
	p := a.palette(c)
	value := selected
	label := selected
	for _, opt := range options {
		if opt.Value == selected {
			label = opt.Label
			break
		}
	}

	sel := ui.SelectBase(c.Key(key), &value)
	trigger := sel.Trigger
	trigger.Height(30).PaddingX(11).Radius(8).FocusRing(false).
		AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).MinWidth(120)
	bg := p.surface2
	if trigger.Hovered() || sel.Open() {
		bg = p.press
	}
	trigger.Border(1, p.hairline).Background(trigger.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	trigger.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		ui.Icon(c, iconChevron).Size(13, 13).TextColor(p.faint)
	})

	sel.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(10).Background(p.surface).Border(1, p.hairline).
			Shadow(0, 18, 44, -14, p.cardShade)
		for _, opt := range options {
			opt := opt
			item := sel.Item(opt.Value).FillWidth().Height(30).PaddingX(9).Radius(6).
				AlignItems(ui.Center).Gap(8)
			switch {
			case item.Highlighted():
				item.Background(p.press)
			case opt.Value == selected:
				item.Background(p.hover)
			}
			item.Children(func() {
				ui.Text(c, opt.Label).FontSize(12).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				if opt.Value == selected {
					ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.accent)
				}
			})
		}
	})

	if value != selected {
		return value, true
	}
	return selected, false
}
