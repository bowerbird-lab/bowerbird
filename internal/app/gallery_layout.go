package app

import "bowerbird/core"

type galleryLayoutKey struct {
	firstItem   *core.Item
	firstFolder *core.Folder
	count       int
	folders     int
	width       int
	sidebar     float32
	inspector   float32
	rowHeight   int
	mode        string
	folder      string
	nav         Nav
	showNames   bool
	collapsed   bool
}

func (a *App) galleryLayoutKey() galleryLayoutKey {
	key := galleryLayoutKey{
		count: len(a.items), folders: len(a.folders), width: 900,
		rowHeight: a.rowHeight, mode: a.prefs.LayoutMode,
		folder: a.folderID, nav: a.nav, showNames: a.prefs.ShowNames,
		collapsed: a.subfoldersCollapsed,
	}
	if len(a.items) > 0 {
		key.firstItem = &a.items[0]
	}
	if len(a.folders) > 0 {
		key.firstFolder = &a.folders[0]
	}
	if a.win != nil {
		if width, _ := a.win.ContentSize(); width > 0 {
			key.width = width
		}
	}
	if a.prefs.SidebarOpen {
		key.sidebar = a.sidebarWidth
	}
	if a.inspectorVisible() {
		key.inspector = a.inspectorWidth
	}
	return key
}

func (a *App) ensureRows() {
	if a.galleryLayoutKey() != a.rowsKey {
		a.rebuildRows()
	}
}
