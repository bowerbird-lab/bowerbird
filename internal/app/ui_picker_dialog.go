package app

import (
	"github.com/egoist/mygo/ui"
)

// pickerDialog draws the frame the folder and tag pickers share: a title, a
// hint, a search field over the list, the list itself inside a bounded scroll,
// and Cancel. body fills the list, and cancel clears the picker's state when
// Cancel or Escape closes it.
func (a *App) pickerDialog(c *ui.Context, open *bool, title, hint, placeholder, searchKey string, query *string, body func(), cancel func()) {
	p := a.palette(c)
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		panel.Width(380).Padding(18).Radius(14).Background(p.surface).Border(1, p.hairline).Shadow(0, 26, 64, -18, p.cardShade).Children(func() {
			ui.Column(c).FillWidth().Gap(12).Children(func() {
				ui.Text(c, title).FontSize(16).FontWeight(650).TextColor(p.text)
				if hint != "" {
					ui.Text(c, hint).FontSize(11.5).TextColor(p.muted)
				}
				search := ui.Row(c).FillWidth().Height(30).PaddingX(9).Radius(8).Background(p.surface2).Border(1, p.hairline).AlignItems(ui.Center).Gap(7)
				search.Children(func() {
					ui.Icon(c, iconSearch).Size(13, 13).TextColor(p.faint)
					field := ui.TextInputBase(c.Key(searchKey), query).
						Placeholder(placeholder).
						PlaceholderColor(p.faint).
						TextColor(p.text).
						FontSize(12).
						Grow(1).Shrink(1).MinWidth(0).AutoFocus()
					if field.Focused() {
						search.Border(1, p.accent)
					}
				})
				ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
					ui.Column(c).FillWidth().Gap(1).Children(body)
				})
				ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
					if ui.Button(c, a.t("common.cancel")).Clicked() {
						cancel()
					}
				})
			})
		})
	})
}
