package app

import (
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

// viewSidebar draws the library switcher, smart views and the recursive folder
// tree. It is also the drop target for dragging assets onto a folder.
func (a *App) viewSidebar(c *ui.Context) {
	p := a.palette(c)
	a.dropFolderID = ""
	sidebar := ui.Column(c).Width(sidebarWidth).FillHeight()
	a.applySidebarSurface(c, sidebar)
	sidebar.Children(func() {
		// The sidebar header matches the main header's first row, so the
		// toggle and the main toolbar controls share one vertical line. The
		// toggle stays just right of the traffic lights, inside the safe area.
		bar := c.TitleBar()
		ui.Row(c).Height(headerHeight).FillWidth().Padding(0, 8, 0, bar.Left+gallerySidePadding).AlignItems(ui.Center).DragWindow().Children(func() {
			a.iconButton(c, "sidebar-toggle", iconSidebar, true, a.t("common.toggleSidebar"), a.toggleSidebar)
		})
		a.sidebarLibrarySwitcher(c)
		ui.Scroll(c).Grow(1).Shrink(1).Padding(4, 10, 12, 10).Children(func() {
			a.navRow(c, NavAll, iconAll, a.t("sidebar.allItems"), a.stats.Total)
			a.navRow(c, NavUntagged, iconInbox, a.t("sidebar.untagged"), a.stats.UntaggedCount)
			a.navRow(c, NavUncategorized, iconUncategorized, a.t("sidebar.uncategorized"), a.stats.UncategorizedCount)
			a.navRow(c, NavTags, iconTag, a.t("sidebar.tags"), len(a.tags))
			a.navRow(c, NavTrash, iconTrash, a.t("sidebar.trash"), a.trashCount)

			ui.Row(c).FillWidth().Height(28).PaddingX(4).Margin(12, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				a.sectionLabel(c, a.t("sidebar.folders"))
				ui.Box(c).Grow(1)
				a.iconButton(c, "new-folder", iconFolderPlus, false, a.t("sidebar.newFolder"), func() {
					a.dialog = dialogNewFolder
					a.dialogValue = ""
					a.newFolderParentID = ""
					if a.nav == NavFolder {
						a.newFolderParentID = a.folderID
					}
				})
			})
			if len(a.folders) == 0 {
				ui.Text(c, a.t("sidebar.noFolders")).FontSize(11).TextColor(p.faint).Padding(12, 6)
			} else {
				for _, f := range a.folderRoots() {
					a.folderRow(c, f, 0)
				}
			}
		})
	})
}

func (a *App) navRow(c *ui.Context, nav Nav, icon *ui.SVG, label string, count int) {
	p := a.palette(c)
	active := a.nav == nav
	b := ui.ButtonBase(c.Key("nav-" + string(nav))).FillWidth().Height(30).PaddingX(8).Radius(7).FocusRing(false).AlignItems(ui.Center).Gap(8)
	bg := ui.Transparent
	if active {
		bg = p.press
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(12.5).Grow(1).Shrink(1).SingleLine().TextColor(p.text)
		if count > 0 {
			ui.Textf(c, "%d", count).FontSize(10.5).FontFeatures("tnum").TextColor(p.faint)
		}
	})
	if b.Clicked() {
		a.navigate(nav, "")
	}
}

func (a *App) folderRow(c *ui.Context, f core.Folder, depth int) {
	p := a.palette(c)
	open := a.expanded[f.ID]
	children := childFolders(a.folders, f.ID)
	active := a.nav == NavFolder && a.folderID == f.ID
	// The expand chevron is a 12px glyph centred in an 18px button, so its
	// visible right edge sits 3px inside the row's content edge. Leaf rows get
	// the same inset for their count, keeping both kinds of trailing glyph on
	// one vertical line without reserving a whole empty chevron slot.
	paddingEnd := float32(8)
	if len(children) == 0 {
		paddingEnd += 3
	}
	b := ui.ButtonBase(c.Key("folder-" + f.ID)).FillWidth().Height(28).PaddingStart(float32(8 + depth*14)).PaddingEnd(paddingEnd).Radius(7).FocusRing(false).AlignItems(ui.Center).Gap(6)
	b.Drag(sidebarDrag{FolderID: f.ID, ParentID: f.ParentID})
	// A folder row takes two kinds of in-app drags: items to classify, and
	// sibling folders to reorder. The item payload also drives the existing
	// drop highlight; a folder drag shows an insertion line instead.
	dragged, dropOver := ui.DragOver[sidebarDrag](b)
	itemDropOver := dropOver && len(dragged.ItemIDs) > 0
	folderDropOver := dropOver && dragged.FolderID != "" && dragged.FolderID != f.ID && dragged.ParentID == f.ParentID
	dropAfter := folderDropOver && a.folderDropAfter(dragged.FolderID, f.ID)
	bg := ui.Transparent
	if itemDropOver || active {
		bg = p.press
	} else if b.Hovered() || folderDropOver {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	if b.Hovered() {
		a.dropFolderID = f.ID
	}
	b.Children(func() {
		ui.Icon(c, iconFolder).Size(14, 14).TextColor(p.muted)
		ui.Text(c, f.Name).FontSize(12.5).Grow(1).Shrink(1).MinWidth(0).SingleLine().TextColor(p.text)
		if f.ItemCount > 0 {
			ui.Textf(c, "%d", f.ItemCount).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
		}
		if len(children) > 0 {
			arrow := ui.ButtonBase(c.Key("fold-arrow-"+f.ID)).Size(18, 18).Radius(4).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center)
			arrow.Children(func() {
				ic := iconChevronR
				if open {
					ic = iconChevron
				}
				ui.Icon(c, ic).Size(12, 12).TextColor(p.faint)
			})
			if arrow.Clicked() {
				a.expanded[f.ID] = !open
			}
		}
		if folderDropOver {
			line := ui.Box(c).Absolute().Left(4).Right(4).Height(2).Radius(1).Background(p.accent)
			if dropAfter {
				line.Bottom(0)
			} else {
				line.Top(0)
			}
		}
	})
	if b.Clicked() {
		if f.ID != a.folderID || a.nav != NavFolder {
			a.navigate(NavFolder, f.ID)
		}
	}
	if dropped, ok := ui.Drop[sidebarDrag](b); ok {
		switch {
		case len(dropped.ItemIDs) > 0:
			a.addItemsToFolder(dropped.ItemIDs, f.ID)
		case dropped.FolderID != "" && dropped.FolderID != f.ID && dropped.ParentID == f.ParentID:
			a.reorderFolder(dropped.FolderID, f.ID)
		}
	}
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item(a.t("common.rename")).Chosen() {
			a.dialog = dialogRenameFolder
			a.dialogValue = f.Name
			a.folderID = f.ID
		}
		if m.Item(a.t("sidebar.newSubfolderMenu")).Chosen() {
			a.dialog = dialogNewFolder
			a.dialogValue = ""
			a.newFolderParentID = f.ID
		}
		if m.Item(a.t("contextMenu.includeSubfolders")).Checked(a.prefs.IncludeSubs).Chosen() {
			a.prefs.IncludeSubs = !a.prefs.IncludeSubs
			a.savePrefs()
			if a.nav == NavFolder && a.folderID == f.ID {
				a.refreshAll()
			}
		}
		m.Separator()
		if m.Item(a.t("common.delete")).Chosen() {
			a.dialog = dialogDeleteFolder
			a.dialogValue = f.Name
			a.folderID = f.ID
		}
	})
	if len(children) > 0 && open {
		for i := range children {
			a.folderRow(c, children[i], depth+1)
		}
	}
}

// folderDropAfter reports whether a drop of draggedID onto targetID should
// place it after the target: moving down inserts after, moving up before.
func (a *App) folderDropAfter(draggedID, targetID string) bool {
	dragged, ok := findFolder(a.folders, draggedID)
	if !ok {
		return false
	}
	siblings := childFolders(a.folders, dragged.ParentID)
	from, to := -1, -1
	for i := range siblings {
		if siblings[i].ID == draggedID {
			from = i
		}
		if siblings[i].ID == targetID {
			to = i
		}
	}
	return from >= 0 && to >= 0 && from < to
}

// reorderFolder moves draggedID next to targetID among their shared siblings
// and persists sort_order from zero in the new order.
func (a *App) reorderFolder(draggedID, targetID string) {
	if a.svc == nil || draggedID == "" || targetID == "" || draggedID == targetID {
		return
	}
	dragged, ok := findFolder(a.folders, draggedID)
	if !ok {
		return
	}
	siblings := childFolders(a.folders, dragged.ParentID)
	from, to := -1, -1
	for i := range siblings {
		if siblings[i].ID == draggedID {
			from = i
		}
		if siblings[i].ID == targetID {
			to = i
		}
	}
	if from < 0 || to < 0 {
		return
	}
	next := make([]string, 0, len(siblings))
	for _, sibling := range siblings {
		if sibling.ID != draggedID {
			next = append(next, sibling.ID)
		}
	}
	at := -1
	for i, id := range next {
		if id == targetID {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	if from < to {
		at++
	}
	next = append(next, "")
	copy(next[at+1:], next[at:])
	next[at] = draggedID
	if err := a.svc.SetFolderOrder(dragged.ParentID, next); err != nil {
		a.showToast(err.Error())
		return
	}
	a.refreshAll()
}

// folderRoots returns folders shown at the top of the sidebar tree. A folder
// whose parent is no longer present stays visible as a root instead of
// disappearing from the tree.
func (a *App) folderRoots() []core.Folder {
	all := flattenFolders(a.folders)
	known := make(map[string]bool, len(all))
	for _, f := range all {
		known[f.ID] = true
	}
	roots := make([]core.Folder, 0, len(a.folders))
	for _, f := range all {
		if f.ParentID == "" || !known[f.ParentID] {
			roots = append(roots, f)
		}
	}
	return roots
}
