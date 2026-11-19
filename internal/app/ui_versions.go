package app

import (
	"bowerbird/core"
	"fmt"
	"github.com/egoist/mygo/ui"
	"path/filepath"
)

func (a *App) invalidateItemBitmaps(item *core.Item) {
	if item == nil {
		return
	}
	paths := []string{
		item.FilePath,
		filepath.Join(item.ItemPath, "thumbnail.jpg"),
		filepath.Join(item.ItemPath, "thumbnail.png"),
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		a.thumbs.invalidate(p)
		a.previewImages.invalidate(p)
		a.filmstripImages.invalidate(p)
	}
}

func (a *App) restoreItemVersion(item core.Item, versionID string) {
	if a.svc == nil || item.ID == "" || versionID == "" {
		return
	}
	go func() {
		updated, err := a.svc.RestoreItemVersion(item.ID, versionID)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.invalidateItemBitmaps(&item)
			if updated != nil {
				a.invalidateItemBitmaps(updated)
				if a.activeItem != nil && a.activeItem.ID == updated.ID {
					a.activeItem = updated
					a.promptDraft = updated.Prompt
				}
			}
			a.showToast(a.t("versions.restored"))
			a.refreshAll()
		})
	}()
}

func (a *App) deleteItemVersion(item core.Item, versionID string) {
	if a.svc == nil || item.ID == "" || versionID == "" {
		return
	}
	go func() {
		updated, err := a.svc.DeleteItemVersion(item.ID, versionID)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			if updated != nil {
				if a.activeItem != nil && a.activeItem.ID == updated.ID {
					a.activeItem = updated
				}
			}
			a.showToast(a.t("versions.deleted"))
			a.refreshAll()
		})
	}()
}

func (a *App) viewInspectorVersions(c *ui.Context, item core.Item) {
	if len(item.Versions) == 0 {
		return
	}
	p := a.palette(c)
	a.inspectorDivider(c)
	a.sectionLabel(c, a.tf("versions.titleCount", map[string]any{"count": len(item.Versions)}))
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		for _, v := range item.Versions {
			v := v
			backupPath := filepath.Join(item.ItemPath, v.Filename)
			card := ui.Row(c.Key("ver-"+v.ID)).FillWidth().Padding(6).Radius(8).Background(p.surface2).Border(1, p.hairline).Gap(8).AlignItems(ui.Center)
			card.Children(func() {
				thumb := ui.Box(c).Size(44, 44).Radius(6).Background(p.bg).Clip().Shrink(0)
				if bmp := a.thumbs.get(backupPath, 96); bmp != nil {
					thumb.Children(func() {
						ui.Image(c, bmp).Fill().Fit(ui.Cover)
					})
				}
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, formatDate(v.CreatedAt)).FontSize(10.5).FontWeight(600).TextColor(p.text).SingleLine()
					dim := formatBytes(v.Size)
					if v.Width > 0 && v.Height > 0 {
						dim = fmt.Sprintf("%dx%d · %s", v.Width, v.Height, dim)
					}
					ui.Text(c, dim).FontSize(10).TextColor(p.faint).SingleLine()
					if v.Prompt != "" {
						ui.Text(c, v.Prompt).FontSize(10).TextColor(p.muted).SingleLine()
					}
				})
				ui.Row(c).Gap(4).Shrink(0).Children(func() {
					a.iconButton(c, "ver-restore-"+v.ID, iconRestore, false, a.t("versions.restore"), func() {
						a.restoreItemVersion(item, v.ID)
					})
					a.iconButton(c, "ver-delete-"+v.ID, iconTrash, false, a.t("versions.delete"), func() {
						a.deleteItemVersion(item, v.ID)
					})
				})
			})
		}
	})
}
