package app

import (
	"bowerbird/core"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"path/filepath"
	"sort"
	"strings"
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

func (a *App) invalidateVariantGroupCache() {
	a.variantGroupCacheID = ""
	a.variantGroupPrimary = nil
	a.variantGroupItems = nil
	a.variantGroupPending = ""
}

func (a *App) cachedVariantGroup(itemID string) (*core.Item, []core.Item) {
	if itemID == "" || a.variantGroupPrimary == nil {
		return nil, nil
	}
	if a.variantGroupCacheID == itemID || a.variantGroupPrimary.ID == itemID {
		return a.variantGroupPrimary, a.variantGroupItems
	}
	for _, v := range a.variantGroupItems {
		if v.ID == itemID {
			return a.variantGroupPrimary, a.variantGroupItems
		}
	}
	return nil, nil
}

func (a *App) ensureVariantGroupLoaded(itemID string) (*core.Item, []core.Item) {
	if a.svc == nil || itemID == "" {
		return nil, nil
	}
	if primary, variants := a.cachedVariantGroup(itemID); primary != nil {
		return primary, variants
	}
	if a.variantGroupPending == itemID {
		return a.variantGroupPrimary, a.variantGroupItems
	}
	a.variantGroupPending = itemID
	go func() {
		primary, variants, err := a.svc.GetVariantGroup(itemID)
		a.post(func() {
			if a.variantGroupPending == itemID {
				a.variantGroupPending = ""
			}
			if err != nil || primary == nil {
				return
			}
			a.variantGroupCacheID = itemID
			a.variantGroupPrimary = primary
			a.variantGroupItems = variants
		})
	}()
	return nil, nil
}

func (a *App) toggleVariantGroupExpanded(itemID string) {
	if a.expandedVariantGroups == nil {
		a.expandedVariantGroups = make(map[string]bool)
	}
	if a.expandedVariantGroups[itemID] {
		delete(a.expandedVariantGroups, itemID)
	} else {
		a.expandedVariantGroups[itemID] = true
	}
	a.refreshWorkspace()
}

func (a *App) groupSelectedAsVariants(primaryID string) {
	if a.svc == nil || primaryID == "" {
		return
	}
	ids := a.selectedIDs()
	variants := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && id != primaryID {
			variants = append(variants, id)
		}
	}
	if len(variants) == 0 {
		return
	}
	go func() {
		updated, err := a.svc.LinkVariants(primaryID, variants)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.invalidateVariantGroupCache()
			if updated != nil {
				a.activeItem = updated
				a.activeItemID = updated.ID
				a.selected = map[string]bool{updated.ID: true}
			}
			a.showToast(a.t("variants.grouped"))
			a.refreshAll()
		})
	}()
}

func (a *App) linkItemsAsVariants(targetID string, draggedIDs []string) {
	if a.svc == nil || targetID == "" || len(draggedIDs) == 0 {
		return
	}
	variants := make([]string, 0, len(draggedIDs))
	for _, id := range draggedIDs {
		if id != "" && id != targetID {
			variants = append(variants, id)
		}
	}
	if len(variants) == 0 {
		return
	}
	go func() {
		updated, err := a.svc.LinkVariants(targetID, variants)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.invalidateVariantGroupCache()
			if updated != nil {
				a.activeItem = updated
				a.activeItemID = updated.ID
				a.selected = map[string]bool{updated.ID: true}
			}
			a.showToast(a.t("variants.grouped"))
			a.refreshAll()
		})
	}()
}

func (a *App) addVariantFiles(parentID string, paths []string) {
	if a.svc == nil || parentID == "" || len(paths) == 0 {
		return
	}
	go func() {
		var lastErr error
		added := 0
		for _, p := range paths {
			if _, err := a.svc.AddVariantFromFile(parentID, p); err != nil {
				lastErr = err
			} else {
				added++
			}
		}
		a.post(func() {
			a.invalidateVariantGroupCache()
			if lastErr != nil && added == 0 {
				a.showToast(lastErr.Error())
				return
			}
			a.showToast(a.t("variants.added"))
			a.refreshAll()
		})
	}()
}

func (a *App) pickAndAddVariantFile(parentID string) {
	if a.svc == nil || parentID == "" {
		return
	}
	title := a.t("variants.addFile")
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:    title,
			Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		var lastErr error
		added := 0
		for _, p := range paths {
			if _, aErr := a.svc.AddVariantFromFile(parentID, p); aErr != nil {
				lastErr = aErr
			} else {
				added++
			}
		}
		a.post(func() {
			a.invalidateVariantGroupCache()
			if lastErr != nil && added == 0 {
				a.showToast(lastErr.Error())
				return
			}
			a.showToast(a.t("variants.added"))
			a.refreshAll()
		})
	}()
}

func (a *App) setPrimaryVariant(parentID, newPrimaryID string) {
	if a.svc == nil || parentID == "" || newPrimaryID == "" {
		return
	}
	go func() {
		updated, err := a.svc.SetPrimaryVariant(parentID, newPrimaryID)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.invalidateVariantGroupCache()
			if a.expandedVariantGroups != nil {
				delete(a.expandedVariantGroups, parentID)
			}
			if updated != nil {
				a.activeItem = updated
				a.activeItemID = updated.ID
				a.selected = map[string]bool{updated.ID: true}
				if a.previewOpen {
					a.previewVariantID = updated.ID
					a.loadPreviewContent()
				}
			}
			a.showToast(a.t("variants.primaryUpdated"))
			a.refreshAll()
		})
	}()
}

func (a *App) unlinkVariant(parentID, variantID string) {
	if a.svc == nil || parentID == "" || variantID == "" {
		return
	}
	go func() {
		updated, err := a.svc.UnlinkVariant(parentID, variantID)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.invalidateVariantGroupCache()
			if updated != nil && a.activeItem != nil && a.activeItem.ID == updated.ID {
				a.activeItem = updated
			}
			a.showToast(a.t("variants.unlinked"))
			a.refreshAll()
		})
	}()
}

func (a *App) selectOrPreviewVariant(v core.Item) {
	vCopy := v
	a.activeItem = &vCopy
	a.activeItemID = v.ID
	a.scrollPreviewVariantID = v.ID
	a.scrollInspectorVariantID = v.ID
	if a.previewOpen {
		a.previewVariantID = v.ID
		a.previewZoom = 1
		a.previewPan = ui.Point{}
		a.loadPreviewContent()
		return
	}
	// If the variant is currently visible in the gallery, select it as well.
	for i := range a.items {
		if a.items[i].ID == v.ID {
			a.selected = map[string]bool{v.ID: true}
			return
		}
	}
}

// previewPagesForItem returns the ordered pages (primary followed by variants)
// for a multi-page preview item, or nil if the item is single-page or unloaded.
func (a *App) previewPagesForItem(item core.Item) []core.Item {
	if len(item.Variants) == 0 && item.VariantOf == "" {
		return nil
	}
	primary, variants := a.cachedVariantGroup(item.ID)
	if primary == nil && a.svc != nil {
		if p, vs, err := a.svc.GetVariantGroup(item.ID); err == nil && p != nil {
			a.variantGroupCacheID = item.ID
			a.variantGroupPrimary = p
			a.variantGroupItems = vs
			primary, variants = p, vs
		}
	}
	if primary == nil {
		return nil
	}
	all := make([]core.Item, 0, 1+len(variants))
	all = append(all, *primary)
	all = append(all, variants...)
	if len(all) <= 1 {
		return nil
	}
	return all
}

// movePreviewPage switches to the previous or next page of a multi-page item
// while preview is open, returning true when a page switch occurred.
func (a *App) movePreviewPage(delta int) bool {
	if !a.previewOpen || len(a.items) == 0 {
		return false
	}
	item := a.currentPreviewItem()
	pages := a.previewPagesForItem(item)
	if len(pages) <= 1 {
		return false
	}
	curIdx := 0
	for i := range pages {
		if pages[i].ID == item.ID {
			curIdx = i
			break
		}
	}
	next := (curIdx + delta) % len(pages)
	if next < 0 {
		next += len(pages)
	}
	a.selectOrPreviewVariant(pages[next])
	return true
}

func (a *App) variantItemThumb(c *ui.Context, v core.Item, size float32) {
	p := a.palette(c)
	thumbPath := ""
	if v.HasThumbnail && v.ItemPath != "" {
		thumbPath = core.ResolveThumbnailPath(v.ItemPath)
	} else if categoryOf(v) == "image" && !a.canGenerateThumbnail(v) {
		thumbPath = v.FilePath
	}
	var bmp *ui.Bitmap
	if thumbPath != "" {
		bmp = a.thumbs.get(thumbPath, int(size*2))
	}
	var sysIcon *ui.Bitmap
	if bmp == nil && thumbPath == "" && v.FilePath != "" && categoryOf(v) != "image" {
		sysIcon = a.thumbs.getSystemIcon(v.Extension, v.FilePath)
	}
	box := ui.Box(c).Size(size, size).Radius(6).Background(p.bg).Clip().Shrink(0)
	box.Children(func() {
		if bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover)
		} else if sysIcon != nil {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Image(c, sysIcon).Size(size*0.65, size*0.65).Fit(ui.Contain)
			})
		} else {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Icon(c, categoryIcon(v)).Size(16, 16).TextColor(p.faint)
			})
		}
	})
}

type variantPageDrag struct {
	PrimaryID string
	ItemID    string
	Index     int
}

type variantGroupSortState struct {
	Mode        string
	Desc        bool
	ManualOrder []string
}

func (a *App) setVariantGroupSortState(primaryID string, st variantGroupSortState) {
	if primaryID == "" {
		return
	}
	if a.variantGroupSort == nil {
		a.variantGroupSort = make(map[string]variantGroupSortState)
	}
	a.variantGroupSort[primaryID] = st
}

func (a *App) variantGroupSortStateFor(primaryID string, all []core.Item) (mode string, desc bool) {
	if st, ok := a.variantGroupSort[primaryID]; ok && st.Mode != "" {
		return st.Mode, st.Desc
	}
	return "manual", false
}

func variantGroupItemIDs(all []core.Item) []string {
	ids := make([]string, len(all))
	for i, it := range all {
		ids[i] = it.ID
	}
	return ids
}

func (a *App) reorderVariantPages(parentID string, orderedIDs []string) {
	a.reorderVariantPagesWithSort(parentID, orderedIDs, variantGroupSortState{
		Mode:        "manual",
		Desc:        false,
		ManualOrder: append([]string(nil), orderedIDs...),
	})
}

func (a *App) reorderVariantPagesWithSort(parentID string, orderedIDs []string, sortState variantGroupSortState) {
	if parentID == "" || len(orderedIDs) == 0 {
		return
	}
	a.setVariantGroupSortState(parentID, sortState)
	if a.svc == nil {
		return
	}
	go func() {
		updated, err := a.svc.ReorderVariants(parentID, orderedIDs)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.invalidateVariantGroupCache()
			if a.expandedVariantGroups != nil {
				delete(a.expandedVariantGroups, parentID)
			}
			if updated != nil {
				if updated.ID != parentID && a.variantGroupSort != nil {
					if st, ok := a.variantGroupSort[parentID]; ok {
						a.variantGroupSort[updated.ID] = st
						delete(a.variantGroupSort, parentID)
					}
				}
				a.activeItem = updated
				a.activeItemID = updated.ID
				a.scrollInspectorVariantID = updated.ID
				a.selected = map[string]bool{updated.ID: true}
				if a.previewOpen {
					if a.previewVariantID != "" {
						found := false
						for _, id := range orderedIDs {
							if id == a.previewVariantID {
								found = true
								break
							}
						}
						if !found {
							a.previewVariantID = updated.ID
						}
					} else {
						a.previewVariantID = updated.ID
					}
					a.scrollPreviewVariantID = a.previewVariantID
					a.loadPreviewContent()
				}
			}
			a.showToast(a.t("variants.orderUpdated"))
			a.refreshAll()
		})
	}()
}

func detectVariantGroupSort(all []core.Item) (mode string, desc bool) {
	if len(all) <= 1 {
		return "manual", false
	}
	isAsc := true
	for i := 0; i < len(all)-1; i++ {
		if core.NaturalLess(all[i+1].Name, all[i].Name) {
			isAsc = false
			break
		}
	}
	isDesc := true
	for i := 0; i < len(all)-1; i++ {
		if core.NaturalLess(all[i].Name, all[i+1].Name) {
			isDesc = false
			break
		}
	}
	if isAsc && !isDesc {
		return "name", false
	}
	if isDesc && !isAsc {
		return "name", true
	}
	if isAsc && isDesc {
		return "name", false
	}
	return "manual", false
}

func (a *App) switchVariantGroupToManual(parentID string) {
	if parentID == "" {
		return
	}
	st := a.variantGroupSort[parentID]
	manualOrder := append([]string(nil), st.ManualOrder...)
	nextState := variantGroupSortState{
		Mode:        "manual",
		Desc:        false,
		ManualOrder: manualOrder,
	}
	if len(manualOrder) > 1 && a.svc != nil {
		a.reorderVariantPagesWithSort(parentID, manualOrder, nextState)
		return
	}
	a.setVariantGroupSortState(parentID, nextState)
	a.showToast(a.t("variants.sortManual"))
	a.refreshWorkspace()
}

func (a *App) sortVariantGroupAlphanumeric(parentID string, desc bool) {
	if a.svc == nil || parentID == "" {
		return
	}
	primary, variants, err := a.svc.GetVariantGroup(parentID)
	if err != nil || primary == nil {
		return
	}
	all := make([]core.Item, 0, 1+len(variants))
	all = append(all, *primary)
	all = append(all, variants...)
	if len(all) <= 1 {
		return
	}
	st := a.variantGroupSort[primary.ID]
	manualOrder := st.ManualOrder
	if st.Mode != "name" || len(manualOrder) == 0 {
		manualOrder = variantGroupItemIDs(all)
	}
	sort.SliceStable(all, func(i, j int) bool {
		nameI := all[i].Name
		nameJ := all[j].Name
		if nameI != nameJ {
			if desc {
				return core.NaturalLess(nameJ, nameI)
			}
			return core.NaturalLess(nameI, nameJ)
		}
		if desc {
			return all[j].ID < all[i].ID
		}
		return all[i].ID < all[j].ID
	})
	orderedIDs := variantGroupItemIDs(all)
	a.reorderVariantPagesWithSort(primary.ID, orderedIDs, variantGroupSortState{
		Mode:        "name",
		Desc:        desc,
		ManualOrder: manualOrder,
	})
}

func (a *App) reverseVariantGroupOrder(parentID string) {
	if a.svc == nil || parentID == "" {
		return
	}
	primary, variants, err := a.svc.GetVariantGroup(parentID)
	if err != nil || primary == nil {
		return
	}
	all := make([]core.Item, 0, 1+len(variants))
	all = append(all, *primary)
	all = append(all, variants...)
	if len(all) <= 1 {
		return
	}
	orderedIDs := make([]string, len(all))
	for i := range all {
		orderedIDs[i] = all[len(all)-1-i].ID
	}
	st := a.variantGroupSort[primary.ID]
	if st.Mode == "name" {
		st.Desc = !st.Desc
	} else {
		st.Mode = "manual"
		st.Desc = !st.Desc
		st.ManualOrder = append([]string(nil), orderedIDs...)
	}
	a.reorderVariantPagesWithSort(primary.ID, orderedIDs, st)
}

func (a *App) toggleVariantGroupOrder(parentID string, all []core.Item) {
	if len(all) <= 1 {
		return
	}
	mode, desc := a.variantGroupSortStateFor(parentID, all)
	if mode == "name" {
		a.sortVariantGroupAlphanumeric(parentID, !desc)
	} else {
		a.reverseVariantGroupOrder(parentID)
	}
}

func (a *App) moveVariantPage(primaryID string, all []core.Item, itemID string, delta int) {
	if len(all) <= 1 || itemID == "" {
		return
	}
	for i, it := range all {
		if it.ID == itemID {
			a.moveVariantPageToPosition(primaryID, all, itemID, i+delta)
			return
		}
	}
}

func (a *App) moveVariantPageToEdge(primaryID string, all []core.Item, itemID string, toStart bool) {
	if len(all) <= 1 || itemID == "" {
		return
	}
	orderedIDs := make([]string, 0, len(all))
	for _, it := range all {
		if it.ID != itemID {
			orderedIDs = append(orderedIDs, it.ID)
		}
	}
	if toStart {
		orderedIDs = append([]string{itemID}, orderedIDs...)
	} else {
		orderedIDs = append(orderedIDs, itemID)
	}
	a.reorderVariantPages(primaryID, orderedIDs)
}

func (a *App) moveVariantPageToPosition(primaryID string, all []core.Item, itemID string, targetIdx int) {
	if len(all) <= 1 || itemID == "" || targetIdx < 0 || targetIdx >= len(all) {
		return
	}
	curIdx := -1
	for i, it := range all {
		if it.ID == itemID {
			curIdx = i
			break
		}
	}
	if curIdx < 0 || curIdx == targetIdx {
		return
	}
	remaining := make([]string, 0, len(all)-1)
	for _, it := range all {
		if it.ID != itemID {
			remaining = append(remaining, it.ID)
		}
	}
	finalIDs := make([]string, 0, len(all))
	finalIDs = append(finalIDs, remaining[:targetIdx]...)
	finalIDs = append(finalIDs, itemID)
	finalIDs = append(finalIDs, remaining[targetIdx:]...)
	a.reorderVariantPages(primaryID, finalIDs)
}

func (a *App) pageItemContextMenu(m *ui.Menu, primaryID string, all []core.Item, v core.Item, idx int) {
	isPrimary := idx == 0
	if !isPrimary {
		if m.Item(a.t("variants.setPrimary")).Chosen() {
			a.setPrimaryVariant(primaryID, v.ID)
		}
	}
	if len(all) > 1 {
		if idx > 0 {
			if m.Item(a.t("variants.moveUp")).Chosen() {
				a.moveVariantPage(primaryID, all, v.ID, -1)
			}
		}
		if idx < len(all)-1 {
			if m.Item(a.t("variants.moveDown")).Chosen() {
				a.moveVariantPage(primaryID, all, v.ID, 1)
			}
		}
		if idx > 0 && !isPrimary {
			if m.Item(a.t("variants.moveToTop")).Chosen() {
				a.moveVariantPageToEdge(primaryID, all, v.ID, true)
			}
		}
		if idx < len(all)-1 {
			if m.Item(a.t("variants.moveToBottom")).Chosen() {
				a.moveVariantPageToEdge(primaryID, all, v.ID, false)
			}
		}
		m.Separator()
		sortMode, isDesc := a.variantGroupSortStateFor(primaryID, all)
		if m.Item(a.t("variants.sortManual")).Radio(sortMode == "manual").Chosen() {
			a.switchVariantGroupToManual(primaryID)
		}
		if m.Item(a.t("variants.sortNameAsc")).Radio(sortMode == "name" && !isDesc).Chosen() {
			a.sortVariantGroupAlphanumeric(primaryID, false)
		}
		if m.Item(a.t("variants.sortNameDesc")).Radio(sortMode == "name" && isDesc).Chosen() {
			a.sortVariantGroupAlphanumeric(primaryID, true)
		}
		if m.Item(a.t("variants.reverseOrder")).Chosen() {
			a.reverseVariantGroupOrder(primaryID)
		}
		m.Separator()
	}
	if !isPrimary {
		if m.Item(a.t("variants.unlink")).Chosen() {
			a.unlinkVariant(primaryID, v.ID)
		}
		m.Separator()
	}
	if m.Item(a.t("variants.addFile")).Chosen() {
		a.pickAndAddVariantFile(primaryID)
	}
}

func (a *App) viewVariantSortControls(c *ui.Context, keyPrefix, primaryID string, all []core.Item) {
	sortMode, isDesc := a.variantGroupSortStateFor(primaryID, all)
	dirIcon := iconArrowUp
	if isDesc {
		dirIcon = iconArrowDown
	}
	a.iconButton(c, keyPrefix+"-sort-dir", dirIcon, false, a.t("variants.toggleOrder"), func() {
		a.toggleVariantGroupOrder(primaryID, all)
	})
	a.iconButtonMenu(c, keyPrefix+"-sort-menu", iconSort, a.t("variants.sort"), func(m *ui.Menu) {
		if m.Item(a.t("variants.sortManual")).Radio(sortMode == "manual").Chosen() {
			a.switchVariantGroupToManual(primaryID)
		}
		if m.Item(a.t("variants.sortNameAsc")).Radio(sortMode == "name" && !isDesc).Chosen() {
			a.sortVariantGroupAlphanumeric(primaryID, false)
		}
		if m.Item(a.t("variants.sortNameDesc")).Radio(sortMode == "name" && isDesc).Chosen() {
			a.sortVariantGroupAlphanumeric(primaryID, true)
		}
		m.Separator()
		if m.Item(a.t("variants.reverseOrder")).Chosen() {
			a.reverseVariantGroupOrder(primaryID)
		}
	})
	a.iconButton(c, keyPrefix+"-add-file", iconPlus, false, a.t("variants.addFile"), func() {
		a.pickAndAddVariantFile(primaryID)
	})
}

func (a *App) viewInspectorVariants(c *ui.Context, item core.Item) {
	if len(item.Variants) == 0 && item.VariantOf == "" {
		return
	}
	primary, variants := a.ensureVariantGroupLoaded(item.ID)
	if primary == nil {
		return
	}
	p := a.palette(c)
	a.inspectorDivider(c)

	all := make([]core.Item, 0, 1+len(variants))
	all = append(all, *primary)
	all = append(all, variants...)

	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			a.sectionLabel(c, a.tf("variants.titleCount", map[string]any{"count": len(all)}))
		})
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			a.viewVariantSortControls(c, "variant", primary.ID, all)
		})
	})

	// Horizontal scrollable strip of all pages in the group.
	ui.ScrollHorizontal(c.Key("inspector-variants-scroll")).FillWidth().Height(92).Children(func() {
		ui.Row(c).Gap(8).Padding(2, 0).AlignItems(ui.Start).Children(func() {
			for idx, v := range all {
				idx := idx
				v := v
				isPrimary := idx == 0
				isCurrent := v.ID == item.ID
				tile := ui.ButtonBase(c.Key("var-hcard-" + v.ID)).Width(68).Column().Gap(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				tile.Drag(variantPageDrag{PrimaryID: primary.ID, ItemID: v.ID, Index: idx})
				dragged, isOver := ui.DragOver[variantPageDrag](tile)
				dragOverValid := isOver && dragged.PrimaryID == primary.ID && dragged.ItemID != v.ID
				tile.Children(func() {
					thumbBox := ui.Box(c).Size(64, 64).Radius(8).Background(p.surface2).Clip()
					if dragOverValid {
						thumbBox.Border(2, p.accent).Background(p.hover)
					} else if isCurrent {
						thumbBox.Border(2, p.accent)
					} else {
						thumbBox.Border(1, p.hairline)
					}
					thumbBox.Children(func() {
						a.variantItemThumb(c, v, 64)
						if isPrimary {
							ui.Box(c).Absolute().Top(3).Left(3).Padding(1, 4).Radius(3).Background(p.accent).Children(func() {
								ui.Text(c, a.t("variants.primaryBadge")).FontSize(8).FontWeight(700).TextColor(p.onAccent)
							})
						} else {
							ui.Box(c).Absolute().Top(3).Left(3).Padding(1, 4).Radius(3).Background(p.badge).Children(func() {
								ui.Textf(c, "#%d", idx+1).FontSize(8).FontWeight(700).TextColor(p.onBadge)
							})
						}
						if v.Extension != "" {
							ui.Box(c).Absolute().Bottom(3).Right(3).Padding(1, 4).Radius(3).Background(p.badge).Children(func() {
								ui.Text(c, strings.ToUpper(v.Extension)).FontSize(7.5).FontWeight(650).TextColor(p.onBadge)
							})
						}
					})
					ui.Text(c, v.Name).FontSize(9.5).TextColor(p.muted).FillWidth().SingleLine().TextAlign(ui.Center)
				})
				if tile.Clicked() {
					a.selectOrPreviewVariant(v)
				}
				if isCurrent && a.scrollInspectorVariantID == v.ID {
					tile.ScrollIntoView(ui.ScrollIntoViewOptions{Inline: ui.ScrollNearest, Animation: &ui.Smooth})
					a.scrollInspectorVariantID = ""
				}
				tile.ContextMenu(func(m *ui.Menu) {
					a.pageItemContextMenu(m, primary.ID, all, v, idx)
				})
				if dropped, ok := ui.Drop[variantPageDrag](tile); ok && dropped.PrimaryID == primary.ID && dropped.ItemID != v.ID {
					a.moveVariantPageToPosition(primary.ID, all, dropped.ItemID, idx)
				}
			}
		})
	})
}

func (a *App) viewPreviewVariantSidebar(c *ui.Context, item core.Item) {
	if a.previewVariantsCollapsed {
		return
	}
	primary, variants := a.ensureVariantGroupLoaded(item.ID)
	p := a.palette(c)

	ui.Box(c.Key("pv-var-border")).Width(1).FillHeight().Background(p.hairline)
	panel := ui.Column(c.Key("pv-var-panel")).Width(248).FillHeight().Clip()
	if a.vibrancyShows(c) && (a.prefs.VibrancySidebar || a.prefs.VibrancyHeader) {
		panel.Material(glass.Blur{Radius: p.headerBlur})
		tint := p.headerTint
		panel.Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(r, tint, 0)
		})
	} else {
		panel.Background(p.sidebar)
	}
	panel.Children(func() {
		if primary == nil {
			ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
			return
		}

		all := make([]core.Item, 0, 1+len(variants))
		all = append(all, *primary)
		all = append(all, variants...)

		// Top header for preview variant sidebar
		ui.Row(c.Key("pv-var-header")).FillWidth().Height(38).Padding(0, 10, 0, 12).AlignItems(ui.Center).Gap(2).Children(func() {
			ui.Text(c, a.tf("variants.titleCount", map[string]any{"count": len(all)})).FontSize(11.5).FontWeight(650).TextColor(p.text).Grow(1).SingleLine()
			a.viewVariantSortControls(c, "pv-var", primary.ID, all)
		})
		ui.Box(c).FillWidth().Height(1).Background(p.hairline)

		ui.Scroll(c.Key("pv-variant-sidebar-scroll")).Grow(1).Shrink(1).FillWidth().Padding(10, 12).Children(func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() {
				for idx, v := range all {
					idx := idx
					v := v
					isPrimary := idx == 0
					isCurrent := v.ID == item.ID
					card := ui.Column(c.Key("pvs-card-" + v.ID)).FillWidth().Gap(5)
					card.Children(func() {
						// Clickable thumbnail / file icon preview
						pick := ui.ButtonBase(c.Key("pvs-pick-" + v.ID)).FillWidth().Height(116).Radius(7).Background(p.bg).Clip().Cursor(ui.CursorPointer)
						pick.Drag(variantPageDrag{PrimaryID: primary.ID, ItemID: v.ID, Index: idx})
						dragged, isOver := ui.DragOver[variantPageDrag](pick)
						dragOverValid := isOver && dragged.PrimaryID == primary.ID && dragged.ItemID != v.ID
						pick.Children(func() {
							thumbPath := ""
							if v.HasThumbnail && v.ItemPath != "" {
								thumbPath = core.ResolveThumbnailPath(v.ItemPath)
							} else if categoryOf(v) == "image" && !a.canGenerateThumbnail(v) {
								thumbPath = v.FilePath
							}
							var bmp *ui.Bitmap
							if thumbPath != "" {
								bmp = a.thumbs.get(thumbPath, 360)
							}
							var sysIcon *ui.Bitmap
							if bmp == nil && thumbPath == "" && v.FilePath != "" && categoryOf(v) != "image" {
								sysIcon = a.thumbs.getSystemIcon(v.Extension, v.FilePath)
							}
							if bmp != nil {
								ui.Image(c, bmp).Fill().Fit(ui.Cover)
							} else if sysIcon != nil {
								ui.Column(c).Fill().Center().Children(func() {
									ui.Image(c, sysIcon).Size(48, 48).Fit(ui.Contain)
								})
							} else {
								ui.Column(c).Fill().Center().Children(func() {
									ui.Icon(c, categoryIcon(v)).Size(26, 26).TextColor(p.faint)
								})
							}
							if isPrimary {
								ui.Box(c).Absolute().Top(6).Left(6).Padding(2, 6).Radius(4).Background(p.accent).Children(func() {
									ui.Text(c, a.t("variants.primaryBadge")).FontSize(9).FontWeight(700).TextColor(p.onAccent)
								})
							} else {
								ui.Box(c).Absolute().Top(6).Left(6).Padding(2, 6).Radius(4).Background(p.badge).Children(func() {
									ui.Textf(c, "#%d", idx+1).FontSize(9).FontWeight(700).TextColor(p.onBadge)
								})
							}
							if v.Extension != "" {
								ui.Box(c).Absolute().Bottom(6).Right(6).Padding(2, 6).Radius(4).Background(p.badge).Children(func() {
									ui.Text(c, strings.ToUpper(v.Extension)).FontSize(9).FontWeight(650).TextColor(p.onBadge)
								})
							}
							if dragOverValid {
								ui.Box(c).Absolute().Fill().Radius(7).Border(2, p.accent).Background(p.hover)
							} else if isCurrent {
								ui.Box(c).Absolute().Fill().Radius(7).Border(2, p.accent)
							}
						})
						if pick.Clicked() {
							a.selectOrPreviewVariant(v)
						}
						pick.ContextMenu(func(m *ui.Menu) {
							a.pageItemContextMenu(m, primary.ID, all, v, idx)
						})
						if dropped, ok := ui.Drop[variantPageDrag](pick); ok && dropped.PrimaryID == primary.ID && dropped.ItemID != v.ID {
							a.moveVariantPageToPosition(primary.ID, all, dropped.ItemID, idx)
						}

						// Name only
						ui.Text(c, v.Name).FontSize(11).FontWeight(600).TextColor(p.text).FillWidth().SingleLine()
					})
					if isCurrent && a.scrollPreviewVariantID == v.ID {
						card.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollNearest, Animation: &ui.Smooth})
						a.scrollPreviewVariantID = ""
					}
				}
			})
		})
	})
}
