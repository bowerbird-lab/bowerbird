package app

import (
	"strings"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

// The folder picker is the shared "add to folder" dialog: the sidebar's folder
// tree with a search field, opened for one item or a whole selection. It only
// links the items to a folder, since an item can live in many; the inspector
// shows the folders they are already in beside the plus that opens it.

// folderTreeIndent is how far each level of the folder tree is indented, as
// the sidebar's rows are.
const folderTreeIndent float32 = 14

// openFolderPicker targets items, resetting the dialog's search. Its tree has
// its own expansion state, kept between openings: the sidebar's tree keeps
// its own too, so opening the picker never folds or unfolds the sidebar.
func (a *App) openFolderPicker(items []core.Item) {
	if len(items) == 0 {
		return
	}
	a.folderPickerItems = append(a.folderPickerItems[:0], items...)
	a.folderPickerQuery = ""
	if a.folderPickerExpanded == nil {
		a.folderPickerExpanded = make(map[string]bool)
	}
	a.dialog = dialogFolderPicker
}

func (a *App) closeFolderPicker() {
	a.dialog = dialogNone
	a.folderPickerItems = nil
	a.folderPickerQuery = ""
}

// hasTargetFolder reports whether every item the picker targets already lives
// in folderID, which the tree marks and clicking leaves as it is.
func (a *App) hasTargetFolder(folderID string) bool {
	if len(a.folderPickerItems) == 0 {
		return false
	}
	for _, item := range a.folderPickerItems {
		if !contains(item.Folders, folderID) {
			return false
		}
	}
	return true
}

// addTargetsToFolder links every target to folderID and closes the dialog.
func (a *App) addTargetsToFolder(folderID string) {
	items := a.folderPickerItems
	a.closeFolderPicker()
	if a.svc == nil || folderID == "" || len(items) == 0 {
		return
	}
	go func() {
		var failure error
		for _, item := range items {
			if err := a.svc.AddItemToFolder(item.ID, folderID); err != nil {
				failure = err
			}
		}
		a.post(func() {
			if failure != nil {
				a.showToast(failure.Error())
				return
			}
			a.showToast(a.t("workspace.itemMoved"))
			a.refreshAll()
		})
	}()
}

// folderPickerShown returns the folders the tree draws for query: all of them
// when it is empty, else the matches and their ancestors, so a match keeps its
// place in the tree instead of flattening into a list of paths.
func (a *App) folderPickerShown(query string) map[string]bool {
	index := a.folderIndex()
	shown := make(map[string]bool, len(index.all))
	if query == "" {
		for _, folder := range index.all {
			shown[folder.ID] = true
		}
		return shown
	}
	for _, folder := range index.all {
		if !strings.Contains(strings.ToLower(folder.Name), query) {
			continue
		}
		for id := folder.ID; id != "" && !shown[id]; {
			shown[id] = true
			node, ok := index.byID[id]
			if !ok {
				break
			}
			id = node.ParentID
		}
	}
	return shown
}

// viewFolderPickerDialog draws the tree and its search, and adds the targets
// to the folder a row is clicked for.
func (a *App) viewFolderPickerDialog(c *ui.Context, open *bool) {
	p := a.palette(c)
	if len(a.folderPickerItems) == 0 {
		a.dialog = dialogNone
		return
	}
	query := strings.ToLower(strings.TrimSpace(a.folderPickerQuery))
	shown := a.folderPickerShown(query)
	a.pickerDialog(c, open,
		a.t("inspector.addToFolder"),
		a.t("inspector.selectTargetFolder"),
		a.t("contextMenu.searchFolderPlaceholder"),
		"folder-picker-search",
		&a.folderPickerQuery,
		func() {
			switch {
			case len(a.folders) == 0:
				ui.Text(c, a.t("sidebar.noFolders")).FontSize(11.5).TextColor(p.faint).Padding(4, 2)
			case len(shown) == 0:
				ui.Text(c, a.t("inspector.noFolderMatch")).FontSize(11.5).TextColor(p.faint).Padding(4, 2)
			default:
				a.folderPickerRows(c, "", 0, shown, query != "")
			}
		},
		a.closeFolderPicker)
}

// folderPickerRows draws the children of parentID that the search keeps.
func (a *App) folderPickerRows(c *ui.Context, parentID string, depth int, shown map[string]bool, searching bool) {
	for _, folder := range a.folderIndex().children[parentID] {
		if !shown[folder.ID] {
			continue
		}
		a.folderPickerRow(c, folder, depth, shown, searching)
	}
}

// folderPickerRow draws one folder of the tree: the expand arrow the sidebar
// uses, the name, how many items it holds, and a check when the targets are
// already in it. Clicking it adds them and closes the dialog. Its expansion is
// its own, so the sidebar keeps its tree as it was.
func (a *App) folderPickerRow(c *ui.Context, folder core.Folder, depth int, shown map[string]bool, searching bool) {
	p := a.palette(c)
	children := a.folderIndex().children[folder.ID]
	open := searching || a.folderPickerExpanded[folder.ID]
	row := ui.ButtonBase(c.Key("folder-picker-" + folder.ID)).FillWidth().Height(28).PaddingStart(6 + folderTreeIndent*float32(depth)).PaddingEnd(8).Radius(6).FocusRing(false).AlignItems(ui.Center).Gap(6)
	bg := ui.Transparent
	if row.Hovered() {
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	row.Children(func() {
		if len(children) > 0 && !searching {
			arrow := ui.ButtonBase(c.Key("folder-picker-arrow-"+folder.ID)).Size(16, 16).Radius(4).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center)
			arrow.Children(func() {
				icon := iconChevronR
				if open {
					icon = iconChevron
				}
				ui.Icon(c, icon).Size(11, 11).TextColor(p.faint)
			})
			if arrow.Clicked() {
				a.folderPickerExpanded[folder.ID] = !open
			}
		} else {
			ui.Box(c).Width(16)
		}
		ui.Icon(c, iconFolder).Size(13, 13).TextColor(p.muted)
		ui.Text(c, folder.Name).FontSize(12).Grow(1).Shrink(1).MinWidth(0).SingleLine().TextColor(p.text)
		if a.hasTargetFolder(folder.ID) {
			ui.Icon(c, iconCheck).Size(12, 12).TextColor(p.accent)
		} else if folder.ItemCount > 0 {
			ui.Textf(c, "%d", folder.ItemCount).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
		}
	})
	if row.Clicked() {
		a.addTargetsToFolder(folder.ID)
	}
	if len(children) > 0 && open {
		a.folderPickerRows(c, folder.ID, depth+1, shown, searching)
	}
}

// sharedFolders lists the folders every item lives in, which is what a
// selection can show as its own.
func sharedFolders(items []core.Item) []string {
	if len(items) == 0 {
		return nil
	}
	shared := make([]string, 0, len(items[0].Folders))
	for _, id := range items[0].Folders {
		every := true
		for _, item := range items[1:] {
			if !contains(item.Folders, id) {
				every = false
				break
			}
		}
		if every {
			shared = append(shared, id)
		}
	}
	return shared
}
