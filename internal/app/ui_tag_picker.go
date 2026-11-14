package app

import (
	"sort"
	"strings"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

// The tag picker is the shared "add tag" dialog, built like the folder picker:
// it lists every tag of the library by default, filters them as the user types,
// and offers to create the typed name when no tag has it. Unlike the folder
// tree it needs no expansion state: it is one flat, most-used-first list.

// openTagPicker targets items, resetting the dialog's search.
func (a *App) openTagPicker(items []core.Item) {
	if len(items) == 0 {
		return
	}
	a.tagPickerItems = append(a.tagPickerItems[:0], items...)
	a.tagPickerQuery = ""
	a.dialog = dialogTagPicker
}

func (a *App) closeTagPicker() {
	a.dialog = dialogNone
	a.tagPickerItems = nil
	a.tagPickerQuery = ""
}

// hasTargetTag reports whether every item the picker targets already carries
// name, which the list marks and clicking leaves as it is.
func (a *App) hasTargetTag(name string) bool {
	if len(a.tagPickerItems) == 0 {
		return false
	}
	for _, item := range a.tagPickerItems {
		if !contains(item.Tags, name) {
			return false
		}
	}
	return true
}

// addTagToTargets adds name to every target; the library creates the tag when
// it is new.
func (a *App) addTagToTargets(name string) {
	name = strings.TrimSpace(name)
	items := a.tagPickerItems
	a.closeTagPicker()
	if a.svc == nil || name == "" || len(items) == 0 {
		return
	}
	go func() {
		var failure error
		for _, item := range items {
			if err := a.svc.AddTagToItem(item.ID, name); err != nil {
				failure = err
			}
		}
		a.post(func() {
			if failure != nil {
				a.showToast(failure.Error())
				return
			}
			a.refreshAll()
		})
	}()
}

// tagPickerTags lists the tags the picker shows for query, with the ones the
// library uses most first, as the tag manager's count sort does.
func (a *App) tagPickerTags(query string) []core.Tag {
	tags := make([]core.Tag, 0, len(a.tags))
	for _, tag := range a.tags {
		if query != "" && !strings.Contains(strings.ToLower(tag.Name), query) {
			continue
		}
		tags = append(tags, tag)
	}
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].ItemCount == tags[j].ItemCount {
			return strings.ToLower(tags[i].Name) < strings.ToLower(tags[j].Name)
		}
		return tags[i].ItemCount > tags[j].ItemCount
	})
	return tags
}

// viewTagPickerDialog draws every tag, the search over them, and the row that
// creates the typed name when it matches none.
func (a *App) viewTagPickerDialog(c *ui.Context, open *bool) {
	p := a.palette(c)
	if len(a.tagPickerItems) == 0 {
		a.dialog = dialogNone
		return
	}
	query := strings.TrimSpace(a.tagPickerQuery)
	tags := a.tagPickerTags(strings.ToLower(query))
	exact := false
	for _, tag := range a.tags {
		if strings.EqualFold(tag.Name, query) {
			exact = true
			break
		}
	}
	a.pickerDialog(c, open,
		a.t("inspector.addTag"),
		a.tf("tags.subtitle", map[string]any{"count": len(a.tags)}),
		a.t("inspector.searchOrCreateTagPlaceholder"),
		"tag-picker-search",
		&a.tagPickerQuery,
		func() {
			if query != "" && !exact {
				a.tagPickerCreateRow(c, query)
			}
			switch {
			case len(tags) > 0:
				for _, tag := range tags {
					a.tagPickerRow(c, tag)
				}
			case len(a.tags) == 0:
				ui.Text(c, a.t("inspector.noTags")).FontSize(11.5).TextColor(p.faint).Padding(4, 2)
			case !exact:
				ui.Text(c, a.t("tags.noMatches")).FontSize(11.5).TextColor(p.faint).Padding(4, 2)
			}
		},
		a.closeTagPicker)
}

// tagPickerRow draws one tag: its name, how many items carry it, and a check
// when the targets already do. Clicking it adds the tag and closes the dialog.
func (a *App) tagPickerRow(c *ui.Context, tag core.Tag) {
	p := a.palette(c)
	applied := a.hasTargetTag(tag.Name)
	row := ui.ButtonBase(c.Key("tag-picker-" + tag.Name)).FillWidth().Height(28).PaddingX(8).Radius(6).FocusRing(false).AlignItems(ui.Center).Gap(6)
	bg := ui.Transparent
	if row.Hovered() {
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	row.Children(func() {
		ui.Icon(c, iconTag).Size(13, 13).TextColor(p.muted)
		ui.Text(c, tag.Name).FontSize(12).Grow(1).Shrink(1).MinWidth(0).SingleLine().TextColor(p.text)
		if applied {
			ui.Icon(c, iconCheck).Size(12, 12).TextColor(p.accent)
		} else {
			ui.Textf(c, "%d", tag.ItemCount).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
		}
	})
	if row.Clicked() {
		a.addTagToTargets(tag.Name)
	}
}

// tagPickerCreateRow offers the typed name as a new tag.
func (a *App) tagPickerCreateRow(c *ui.Context, name string) {
	p := a.palette(c)
	row := ui.ButtonBase(c.Key("tag-picker-create")).FillWidth().Height(28).PaddingX(8).Radius(6).FocusRing(false).AlignItems(ui.Center).Gap(6)
	bg := ui.Transparent
	if row.Hovered() {
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	row.Children(func() {
		ui.Icon(c, iconPlus).Size(12, 12).TextColor(p.accent)
		ui.Text(c, a.tf("inspector.createTag", map[string]any{"name": name})).
			FontSize(12).Grow(1).Shrink(1).MinWidth(0).SingleLine().TextColor(p.text)
	})
	if row.Clicked() {
		a.addTagToTargets(name)
	}
}

// tagAddButton is the plus that opens the tag picker, with its label when no
// tag chip beside it explains it.
func (a *App) tagAddButton(c *ui.Context, items []core.Item, withLabel bool) {
	p := a.palette(c)
	b := ui.ButtonBase(c.Key("tag-add")).Height(22).PaddingX(7).Radius(999).Background(p.hover).FocusRing(false).AlignItems(ui.Center).Gap(3)
	b.Tooltip(a.t("inspector.addTag"))
	b.Children(func() {
		ui.Icon(c, iconPlus).Size(10, 10).TextColor(p.muted)
		if withLabel {
			ui.Text(c, a.t("inspector.addTag")).FontSize(10.5).TextColor(p.muted)
		}
	})
	if b.Clicked() {
		a.openTagPicker(items)
	}
}
