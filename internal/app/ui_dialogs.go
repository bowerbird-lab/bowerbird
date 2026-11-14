package app

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// Dialog actions. They run the core calls off the main thread and post both the
// result and any error back through the app's normal state path.

func (a *App) createLibraryFromDialog() {
	name := strings.TrimSpace(a.dialogValue)
	if name == "" {
		a.dialogError = a.t("setup.libraryNameRequired")
		return
	}
	a.dialog = dialogNone
	go func() {
		parent, err := a.svc.SelectDirectory()
		if err != nil || parent == "" {
			return
		}
		_, err = a.svc.CreateLibrary(parent, name)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			a.refreshAll()
		})
	}()
}

func (a *App) createFolderFromDialog() {
	name := strings.TrimSpace(a.dialogValue)
	if name == "" {
		return
	}
	parent := a.newFolderParentID
	a.newFolderParentID = ""
	a.dialog = dialogNone
	go func() {
		_, err := a.svc.CreateFolder(name, parent)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			if parent != "" {
				a.expanded[parent] = true
			}
			a.refreshAll()
		})
	}()
}

func (a *App) renameFolderFromDialog() {
	name := strings.TrimSpace(a.dialogValue)
	if name == "" || a.folderID == "" {
		a.dialog = dialogNone
		return
	}
	id := a.folderID
	a.dialog = dialogNone
	go func() {
		err := a.svc.RenameFolder(id, name)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
			}
			a.refreshAll()
		})
	}()
}

func (a *App) deleteFolderFromDialog() {
	id := a.folderID
	a.dialog = dialogNone
	if id == "" {
		return
	}
	go func() {
		err := a.svc.DeleteFolder(id)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			if a.nav == NavFolder && a.folderID == id {
				a.navigate(NavAll, "")
				return
			}
			a.refreshAll()
		})
	}()
}

func (a *App) installPluginFromDialog() {
	req := a.pendingInstall
	target := ""
	if req != nil {
		target = req.ManifestURL
	} else {
		target = strings.TrimSpace(a.dialogValue)
	}
	a.dialog = dialogNone
	if target == "" {
		return
	}
	if a.svc != nil {
		_ = a.svc.ClearPendingPluginInstall()
	}
	a.pendingInstall = nil
	go func() {
		_, err := a.svc.InstallPlugin(target)
		if a.pluginHost != nil {
			a.pluginHost.Scan()
		}
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
			} else {
				a.showToast(a.t("settings.plugins.installed"))
			}
			a.refreshAll()
		})
	}()
}

// Workspace mutations shared by the context menu and inspector.

func (a *App) writeFilesToClipboard(paths []string) error {
	if a.OnWriteFiles != nil {
		return a.OnWriteFiles(paths)
	}
	return mygo.Clipboard.WriteFiles(paths...)
}

func (a *App) copySelected() {
	var ids []string
	if a.previewOpen && a.previewIndex >= 0 && a.previewIndex < len(a.items) {
		ids = []string{a.items[a.previewIndex].ID}
	} else {
		ids = a.selectedIDs()
		if len(ids) == 0 && a.activeItemID != "" {
			ids = []string{a.activeItemID}
		}
	}
	if len(ids) == 0 {
		return
	}

	if a.svc != nil {
		if err := a.svc.CopyItemsToClipboard(ids); err == nil {
			a.showToast(a.t("common.copied"))
			return
		}
	}

	paths := make([]string, 0, len(ids))
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	for _, it := range a.items {
		if idSet[it.ID] && it.FilePath != "" {
			absPath, err := filepath.Abs(it.FilePath)
			if err == nil {
				paths = append(paths, absPath)
			} else {
				paths = append(paths, it.FilePath)
			}
		}
	}
	if len(paths) > 0 {
		if err := a.writeFilesToClipboard(paths); err == nil {
			a.showToast(a.t("common.copied"))
		}
	}
}

func (a *App) restoreSelected() {
	ids := a.selectedIDs()
	if len(ids) == 0 {
		return
	}
	go func() {
		var err error
		if len(ids) == 1 {
			err = a.svc.RestoreFromTrash(ids[0])
		} else {
			err = a.svc.BatchRestoreFromTrash(ids)
		}
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
			}
			a.clearSelection()
			a.refreshAll()
		})
	}()
}

// deleteActionLabel names what Delete does in the current view. The label and
// deleteSelection share the same branches so the button never promises a
// folder unlink while the key sends the item to the trash.
func (a *App) deleteActionLabel() string {
	switch {
	case a.nav == NavTrash:
		return a.t("contextMenu.deletePermanently")
	case a.nav == NavFolder && a.folderID != "":
		return a.t("contextMenu.removeFromCurrentFolder")
	default:
		return a.t("common.delete")
	}
}

// deleteSelection is the Delete key and the delete action shared by the
// context menu and the inspector. The active view decides what deleting means:
//
//   - Trash: remove the item for good.
//   - A folder: unlink the item from the folder (and its visible subfolders).
//     Only items whose last folder is this one leave for the trash.
//   - Smart views (All, Untagged, Uncategorized, a tag, search): a dialog
//     reports the folders the selection lives in, then offers to detach it from
//     those folders or move it to the trash.
func (a *App) deleteSelection() {
	if a.svc == nil || len(a.selected) == 0 {
		return
	}
	switch {
	case a.nav == NavTrash:
		a.deleteSelectedPermanently()
	case a.nav == NavFolder && a.folderID != "":
		a.deleteFromCurrentFolder()
	default:
		a.requestLibraryDelete()
	}
}

// deleteFromCurrentFolder applies planFolderDelete. Items whose last folder is
// the current one get a warning before they leave for the trash; a selection
// that only loses folder links is applied immediately.
func (a *App) deleteFromCurrentFolder() {
	scope := a.folderScope()
	if len(scope) == 0 {
		scope = []string{a.folderID}
	}
	items := a.selectedItems()
	plan := planFolderDelete(items, scope)
	if len(plan.unlinkIDs) == 0 && len(plan.trashIDs) == 0 {
		return
	}

	if len(plan.trashIDs) > 0 {
		a.pendingFolderDelete = &pendingFolderDelete{
			plan: folderDeletePlan{
				unlinkIDs: append([]string(nil), plan.unlinkIDs...),
				trashIDs:  append([]string(nil), plan.trashIDs...),
			},
			scope: append([]string(nil), scope...),
		}
		a.dialog = dialogDeleteLastFolder
		return
	}

	a.applyFolderDelete(plan, scope)
}

// confirmPendingFolderDelete runs the plan the last-folder warning described.
func (a *App) confirmPendingFolderDelete() {
	req := a.pendingFolderDelete
	if req == nil {
		a.dialog = dialogNone
		return
	}
	plan := req.plan
	scope := req.scope
	a.pendingFolderDelete = nil
	a.dialog = dialogNone
	a.applyFolderDelete(plan, scope)
}

// applyFolderDelete unlinks the items that still have another folder and moves
// the items whose last home this is to the trash, each in one transaction.
func (a *App) applyFolderDelete(plan folderDeletePlan, scope []string) {
	if len(plan.unlinkIDs) == 0 && len(plan.trashIDs) == 0 {
		return
	}

	unlinkIDs := append([]string(nil), plan.unlinkIDs...)
	trashIDs := append([]string(nil), plan.trashIDs...)
	scope = append([]string(nil), scope...)
	go func() {
		var err error
		if len(unlinkIDs) > 0 {
			err = a.svc.BatchRemoveItemsFromFolders(unlinkIDs, scope)
		}
		if err == nil && len(trashIDs) > 0 {
			err = a.svc.BatchMoveToTrash(trashIDs)
		}
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
			} else if len(trashIDs) > 0 {
				a.showToast(a.t("workspace.movedToTrash"))
			} else {
				a.showToast(a.t("workspace.removedFromFolder"))
			}
			a.clearSelection()
			a.refreshAll()
		})
	}()
}

// requestLibraryDelete opens the check for a smart view. The item is not
// marked or moved yet: the dialog shows every folder it lives in and lets the
// user choose between detaching it from those folders and trashing it.
func (a *App) requestLibraryDelete() {
	items := a.selectedItems()
	if len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	folderIDs := itemFolderIDs(items)
	a.pendingDelete = &pendingDelete{
		ids:         ids,
		folderIDs:   folderIDs,
		folderNames: folderNamesForIDs(a.folders, folderIDs),
	}
	a.dialog = dialogDeleteItems
}

// removePendingDeleteFromFolders detaches the checked selection from every
// folder it belongs to. The items stay in the library and in the smart views.
func (a *App) removePendingDeleteFromFolders() {
	req := a.pendingDelete
	if req == nil {
		a.dialog = dialogNone
		return
	}
	ids := append([]string(nil), req.ids...)
	folderIDs := append([]string(nil), req.folderIDs...)
	a.pendingDelete = nil
	a.dialog = dialogNone
	go func() {
		err := a.svc.BatchRemoveItemsFromFolders(ids, folderIDs)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
			} else {
				a.showToast(a.t("workspace.removedFromFolder"))
			}
			a.clearSelection()
			a.refreshAll()
		})
	}()
}

// confirmPendingDeleteToTrash is the destructive choice in the same dialog.
func (a *App) confirmPendingDeleteToTrash() {
	req := a.pendingDelete
	if req == nil {
		a.dialog = dialogNone
		return
	}
	ids := append([]string(nil), req.ids...)
	a.pendingDelete = nil
	a.dialog = dialogNone
	go func() {
		err := a.svc.BatchMoveToTrash(ids)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
			} else {
				a.showToast(a.t("workspace.movedToTrash"))
			}
			a.clearSelection()
			a.refreshAll()
		})
	}()
}

func (a *App) deleteSelectedPermanently() {
	ids := a.selectedIDs()
	if len(ids) == 0 {
		return
	}
	go func() {
		for _, id := range ids {
			_ = a.svc.PermanentDeleteItem(id)
		}
		a.post(func() {
			a.clearSelection()
			a.refreshAll()
		})
	}()
}
