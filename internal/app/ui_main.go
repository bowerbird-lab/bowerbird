package app

import (
	"fmt"
	"strings"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

// viewMain builds one frame of the primary workspace. It has a single root:
// mygo lays out the top-level children of a view as the window, so the
func (a *App) isImmersiveFullscreen() bool {
	return a.win != nil && a.win.IsFullScreen() && a.previewOpen
}

// workspace, toasts and dialogs must be children of one Box rather than
// sibling roots.
func (a *App) viewMain(c *ui.Context) {
	p := a.palette(c)
	// Keep the workspace's minimum by trimming the panels on a window too
	// narrow for them, then cap the seams to what is left.
	if w, _ := c.Size(); w > 0 {
		a.fitPanelWidths(w)
		a.updatePanelLimits(w)
	}
	a.handleMainShortcuts(c)
	a.applyVibrancyRoot(c)
	root := ui.Box(c).Fill()
	a.attachURLDropTarget(root)
	root.Children(func() {
		if a.isImmersiveFullscreen() && len(a.items) > 0 && a.previewIndex >= 0 && a.previewIndex < len(a.items) {
			ui.Box(c).Fill().Background(ui.RGB(0, 0, 0)).Children(func() {
				a.viewPreviewBody(c, a.currentPreviewItem())
			})
			return
		}
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			if a.prefs.SidebarOpen {
				a.viewSidebar(c)
				ui.Box(c).Width(1).FillHeight().Background(p.hairline)
			}
			ui.Column(c).FillHeight().Grow(1).Shrink(1).MinWidth(0).Children(func() {
				if a.previewOpen && a.previewMode == "inline" {
					inline := ui.Column(c.Key("inline-preview")).Fill().Focusable().AutoFocus()
					a.handlePreviewContainerInput(inline)
					inline.Children(func() {
						a.viewPreviewToolbar(c)
						ui.Box(c).FillWidth().Height(1).Background(p.hairline)
						a.mainBodySurface(c, 0, func() {
							if len(a.items) > 0 && a.previewIndex >= 0 && a.previewIndex < len(a.items) {
								a.viewPreviewBody(c, a.currentPreviewItem())
							}
						})
					})
				} else {
					a.mainBodySurface(c, mainHeaderHeight, func() {
						if a.library == nil {
							a.emptyState(c, iconLibrary, a.t("app.noActiveLibrary"), a.t("app.openSetup"))
							return
						}
						if a.nav == NavTags {
							a.viewTags(c)
							return
						}
						a.viewGallery(c)
					})
					a.viewHeader(c)
				}
				if a.prefs.SidebarOpen {
					a.viewSidebarResizer(c)
				}
			})
			if a.inspectorVisible() {
				ui.Box(c).Width(1).FillHeight().Background(p.hairline)
				a.viewInspector(c)
			}
		})
		if a.previewOpen && a.previewMode == "full" {
			ui.Overlay(c, func() {
				overlay := ui.Box(c).Absolute().Fill().Background(p.bg).Focusable().AutoFocus().Modal()
				a.handlePreviewContainerInput(overlay)
				a.registerPreviewOverlayShortcuts(overlay)
				overlay.Children(func() {
					a.viewPreview(c)
				})
			})
		}
		a.viewToast(c)
		a.viewDialog(c)
		a.viewCropDialog(c)
		a.viewIndexProgressDialog(c)
	})
}

// paintBelowHeader fills e with the fixed workspace colour below the strip
// that the floating header owns. At rest the header paints that strip itself;
// while content scrolls under it, the strip stays clear so the header's blur
// sees the content rather than an opaque fill.
func (a *App) paintBelowHeader(c *ui.Context, e ui.Element, inset float32) {
	p := a.palette(c)
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		top := minF(inset, r.H)
		if r.H > top {
			pt.Fill(ui.Rect{X: r.X, Y: r.Y + top, W: r.W, H: r.H - top}, p.bg, 0)
		}
	})
}

// mainBodySurface is the workspace column under the floating header. It uses a
// fixed opaque fill so only the sidebar and the header carry the system
// material; painting the workspace itself over the window material costs too
// much on every scroll and resize.
func (a *App) mainBodySurface(c *ui.Context, topInset float32, body func()) {
	e := ui.Box(c).FillWidth().Grow(1).Shrink(1)
	a.paintBelowHeader(c, e, topInset)
	e.ContextMenu(a.viewContextMenu)
	e.Children(body)
}

// handlePreviewContainerInput binds keyboard actions directly on the active
// preview container (both inline and full-window overlay) so Esc, Space,
// left/right navigation and zoom work reliably regardless of prior focus.
func (a *App) handlePreviewContainerInput(container ui.Element) {
	matchKey := func(ev ui.InputEvent, id string) bool {
		pair, ok := a.shortcutPairs[id]
		return ok && pair.ok && ev.Mods == pair.mods && ev.Key == pair.key
	}
	container.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown {
			return false
		}
		if matchKey(ev, core.ShortcutDismiss) {
			if ev.Repeat {
				return true
			}
			if a.isImmersiveFullscreen() {
				a.win.SetFullScreen(false)
				return true
			}
			a.closePreview()
			return true
		}
		if matchKey(ev, core.ShortcutTogglePreview) {
			if ev.Repeat {
				return true
			}
			a.closePreview()
			return true
		}
		if matchKey(ev, core.ShortcutMoveLeft) {
			a.movePreview(-1)
			return true
		}
		if matchKey(ev, core.ShortcutMoveRight) {
			a.movePreview(1)
			return true
		}
		if matchKey(ev, core.ShortcutMoveUp) {
			return a.movePreviewPage(-1)
		}
		if matchKey(ev, core.ShortcutMoveDown) {
			return a.movePreviewPage(1)
		}
		if matchKey(ev, core.ShortcutZoomIn) {
			a.previewZoom = minF(a.previewZoom*1.2, previewMaxZoom)
			return true
		}
		if matchKey(ev, core.ShortcutZoomOut) {
			a.previewZoom = maxF(a.previewZoom/1.2, previewMinZoom)
			return true
		}
		if matchKey(ev, core.ShortcutZoomReset) {
			a.previewZoom = 1
			a.previewPan = ui.Point{}
			return true
		}
		return false
	})
}

func (a *App) registerPreviewOverlayShortcuts(overlay ui.Element) {
	bind := func(id string, fn func()) {
		pair, ok := a.shortcutPairs[id]
		if !ok || !pair.ok {
			return
		}
		if overlay.Shortcut(pair.mods, pair.key) || overlay.OverlayShortcut(pair.mods, pair.key) {
			fn()
		}
	}
	bind(core.ShortcutTogglePreview, func() {
		if !a.previewOpen {
			return
		}
		a.closePreview()
	})
	bind(core.ShortcutDismiss, func() {
		if a.isImmersiveFullscreen() {
			a.win.SetFullScreen(false)
			return
		}
		a.closePreview()
	})
	bind(core.ShortcutMoveLeft, func() { a.movePreview(-1) })
	bind(core.ShortcutMoveRight, func() { a.movePreview(1) })
	bind(core.ShortcutMoveUp, func() { a.movePreviewPage(-1) })
	bind(core.ShortcutMoveDown, func() { a.movePreviewPage(1) })
	bind(core.ShortcutZoomIn, func() {
		a.previewZoom = minF(a.previewZoom*1.2, previewMaxZoom)
	})
	bind(core.ShortcutZoomOut, func() {
		a.previewZoom = maxF(a.previewZoom/1.2, previewMinZoom)
	})
	bind(core.ShortcutZoomReset, func() {
		a.previewZoom = 1
		a.previewPan = ui.Point{}
	})
}

// handleMainShortcuts runs the main window's keys. Every binding comes from
// core's catalog, so the list Settings shows is exactly what this window
// answers to, whether the binding is rebindable or fixed.
func (a *App) handleMainShortcuts(c *ui.Context) {
	// Every check reads the pre-resolved catalog, so a frame does one map
	// lookup per binding instead of re-parsing the settings map.
	matched := func(id string) bool {
		pair, ok := a.shortcutPairs[id]
		return ok && pair.ok && c.Shortcut(pair.mods, pair.key)
	}
	if matched(core.ShortcutSearch) && !a.previewOpen {
		a.searchBox.Focus(c)
	}
	if matched(core.ShortcutOpenSettings) {
		if a.OnOpenSettings != nil {
			a.OnOpenSettings()
		} else if a.svc != nil {
			_ = a.svc.OpenSettingsWindow()
		}
	}
	if matched(core.ShortcutLogs) {
		if a.OnOpenLogs != nil {
			a.OnOpenLogs()
		} else if a.svc != nil {
			_ = a.svc.OpenLogsWindow()
		}
	}
	if matched(core.ShortcutReload) {
		a.refreshAll()
	}
	if matched(core.ShortcutForceReload) {
		a.reloadIgnoringCache()
	}
	if matched(core.ShortcutFullscreen) && a.win != nil {
		a.win.ToggleFullScreen()
	}
	if matched(core.ShortcutCloseWindow) && a.win != nil {
		a.win.Close()
	}
	if matched(core.ShortcutNewWindow) && a.svc != nil {
		go func() { _ = a.svc.OpenNewWindow("") }()
	}
	if matched(core.ShortcutQuit) {
		a.quit()
	}
	// Ctrl+C copies even on macOS: the catalog binding is CmdOrCtrl+C, and
	// Ctrl+C is what long-time users of the app still reach for.
	if matched(core.ShortcutCopy) || c.Shortcut(ui.Ctrl, ui.KeyC) {
		a.copySelected()
	}
	if (!a.previewOpen || a.previewMode == "inline") && matched(core.ShortcutToggleSidebar) {
		a.prefs.SidebarOpen = !a.prefs.SidebarOpen
		a.savePrefs()
		a.rebuildRows()
	}
	if (!a.previewOpen || a.previewMode == "inline") && matched(core.ShortcutToggleInspector) {
		a.prefs.InspectorOpen = !a.prefs.InspectorOpen
		a.savePrefs()
	}
	if matched(core.ShortcutDismiss) {
		if a.isImmersiveFullscreen() {
			a.win.SetFullScreen(false)
		} else if a.previewOpen {
			a.closePreview()
		} else if a.dialog != dialogNone {
			a.dialog = dialogNone
			a.pendingDelete = nil
			a.pendingFolderDelete = nil
		} else if len(a.selected) > 0 {
			a.clearSelection()
		}
		return
	}
	if matched(core.ShortcutTogglePreview) && !c.ShortcutRepeated() {
		if a.previewOpen {
			a.closePreview()
		} else {
			targetID := a.activeItemID
			if targetID == "" && len(a.selected) > 0 {
				for id := range a.selected {
					targetID = id
					break
				}
			}
			if targetID != "" {
				a.openPreview(targetID, "full")
			}
		}
	}
	if a.previewOpen {
		if matched(core.ShortcutMoveLeft) {
			a.movePreview(-1)
		}
		if matched(core.ShortcutMoveRight) {
			a.movePreview(1)
		}
		if matched(core.ShortcutMoveUp) {
			a.movePreviewPage(-1)
		}
		if matched(core.ShortcutMoveDown) {
			a.movePreviewPage(1)
		}
		if matched(core.ShortcutZoomIn) {
			a.previewZoom = minF(a.previewZoom*1.2, previewMaxZoom)
		}
		if matched(core.ShortcutZoomOut) {
			a.previewZoom = maxF(a.previewZoom/1.2, previewMinZoom)
		}
		if matched(core.ShortcutZoomReset) {
			a.previewZoom = 1
			a.previewPan = ui.Point{}
		}
		return
	}
	if matched(core.ShortcutSelectAll) {
		for _, it := range a.items {
			a.selected[it.ID] = true
		}
		a.selectedAt = len(a.items) - 1
		if len(a.items) > 0 {
			a.activeItemID = a.items[0].ID
		}
		// Move the focus with the selection so the gallery keys, Delete
		// above all, act on what the user just selected.
		if !c.Focused(a.searchBox) {
			a.galleryBox.Focus(c)
		}
	}
	moved := false
	if matched(core.ShortcutMoveLeft) {
		a.moveGallerySelection(-1, 0)
		moved = true
	}
	if matched(core.ShortcutMoveRight) {
		a.moveGallerySelection(1, 0)
		moved = true
	}
	if matched(core.ShortcutMoveUp) {
		a.moveGallerySelection(0, -1)
		moved = true
	}
	if matched(core.ShortcutMoveDown) {
		a.moveGallerySelection(0, 1)
		moved = true
	}
	if moved && !c.Focused(a.searchBox) {
		a.galleryBox.Focus(c)
	}
	if matched(core.ShortcutZoomIn) {
		a.setRowHeight(a.rowHeight + 24)
	}
	if matched(core.ShortcutZoomOut) {
		a.setRowHeight(a.rowHeight - 24)
	}
	if matched(core.ShortcutZoomReset) {
		a.setRowHeight(190)
	}
}

// reloadIgnoringCache drops decoded thumbnails before refetching the library,
// the native-UI counterpart of a force reload.
func (a *App) reloadIgnoringCache() {
	if a.thumbs != nil {
		a.thumbs.clear()
	}
	a.refreshAll()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) setRowHeight(h int) {
	if h < 90 {
		h = 90
	}
	if h > 420 {
		h = 420
	}
	if h == a.rowHeight {
		return
	}
	a.rowHeight = h
	a.prefs.RowHeight = float64(h)
	a.savePrefs()
	a.rebuildRows()
}

// setSidebarWidth and setInspectorWidth clamp a side panel to the range its
// seam can drag and keep the new width in the prefs, which the drag saves once
// the pointer lets go.
func (a *App) setSidebarWidth(w float32) {
	w = minF(maxF(w, minSidebarWidth), a.sidebarLimit)
	if w == a.sidebarWidth {
		return
	}
	a.sidebarWidth = w
	a.prefs.SidebarWidth = float64(w)
}

func (a *App) setInspectorWidth(w float32) {
	w = minF(maxF(w, minInspectorWidth), a.inspectorLimit)
	if w == a.inspectorWidth {
		return
	}
	a.inspectorWidth = w
	a.prefs.InspectorWidth = float64(w)
}

// workspaceWidth is the main column's width in a window vw wide.
func (a *App) workspaceWidth(vw float32) float32 {
	w := vw
	if a.prefs.SidebarOpen {
		w -= a.sidebarWidth + panelDividerWidth
	}
	if a.inspectorVisible() {
		w -= a.inspectorWidth + panelDividerWidth
	}
	return w
}

// fitPanelWidths trims the side panels when the window is too narrow to leave
// the workspace its minimum, the inspector giving way before the sidebar. A
// panel never goes below its own minimum, so a window smaller than all three
// minima leaves the workspace that much narrower.
func (a *App) fitPanelWidths(vw float32) {
	main := a.workspaceWidth(vw)
	if main >= mainMinWidth {
		return
	}
	if a.inspectorVisible() {
		if give := minF(a.inspectorWidth-minInspectorWidth, mainMinWidth-main); give > 0 {
			a.inspectorWidth -= give
			a.prefs.InspectorWidth = float64(a.inspectorWidth)
			main += give
		}
	}
	if main < mainMinWidth && a.prefs.SidebarOpen {
		if give := minF(a.sidebarWidth-minSidebarWidth, mainMinWidth-main); give > 0 {
			a.sidebarWidth -= give
			a.prefs.SidebarWidth = float64(a.sidebarWidth)
		}
	}
}

// updatePanelLimits caps each seam so dragging it cannot squeeze the workspace
// under mainMinWidth. The other panel keeps the width it has.
func (a *App) updatePanelLimits(vw float32) {
	a.sidebarLimit, a.inspectorLimit = maxSidebarWidth, maxInspectorWidth
	if vw <= 0 {
		return
	}
	sidebarRoom := vw - mainMinWidth - panelDividerWidth
	if a.inspectorVisible() {
		sidebarRoom -= a.inspectorWidth + panelDividerWidth
	}
	a.sidebarLimit = maxF(minSidebarWidth, minF(maxSidebarWidth, sidebarRoom))

	inspectorRoom := vw - mainMinWidth - panelDividerWidth
	if a.prefs.SidebarOpen {
		inspectorRoom -= a.sidebarWidth + panelDividerWidth
	}
	a.inspectorLimit = maxF(minInspectorWidth, minF(maxInspectorWidth, inspectorRoom))
}

// panelResizeHandle is the strip over a panel seam. It is built inside the
// panel that comes later in the layout, so it paints and takes the pointer over
// the one before it; the divider sits two DIPs in from its left edge.
func (a *App) panelResizeHandle(c *ui.Context, key string) ui.Element {
	return ui.Box(c.Key(key)).Absolute().Left(-resizerGrip / 2).Top(0).Width(resizerGrip).
		FillHeight().Cursor(ui.CursorResizeEW)
}

// viewSidebarResizer lets the pointer drag the sidebar's edge. Built inside the
// workspace column, after its content, so the grip wins over both panels.
func (a *App) viewSidebarResizer(c *ui.Context) {
	a.panelSeam(c, "sidebar-resizer", &a.resizingSidebar, func(dx float32) {
		a.setSidebarWidth(a.sidebarWidth + dx)
	})
}

// viewInspectorResizer lets the pointer drag the inspector's edge; dragging
// left widens the panel on the right.
func (a *App) viewInspectorResizer(c *ui.Context) {
	a.panelSeam(c, "inspector-resizer", &a.resizingInspector, func(dx float32) {
		a.setInspectorWidth(a.inspectorWidth - dx)
	})
}

// panelSeam applies a drag on a panel edge; resizing records the drag so the
// width is saved once, when the pointer lets go.
func (a *App) panelSeam(c *ui.Context, key string, resizing *bool, apply func(dx float32)) {
	p := a.palette(c)
	h := a.panelResizeHandle(c, key)
	dx, _, held := h.Dragged()
	if held {
		if dx != 0 {
			*resizing = true
			apply(dx)
			// The panes were built with the old width, so ask for the frame
			// that paints the new one even if the pointer stops here.
			c.After(0)
		}
	} else if *resizing {
		*resizing = false
		a.savePrefs()
	}
	if held || h.Hovered() {
		h.Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(ui.Rect{X: r.X + resizerGrip/2 - 1, Y: r.Y, W: 1, H: r.H}, p.accent, 0)
		})
	}
}

func (a *App) clearSelection() {
	a.selected = map[string]bool{}
	a.selectedOrder = nil
	a.selectedAt = -1
	a.selectedAnchor = -1
	a.activeItemID = ""
	a.activeItem = nil
}

func (a *App) viewToast(c *ui.Context) {
	if !a.toastActive() {
		return
	}
	p := a.palette(c)
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(24).Justify(ui.Center).Children(func() {
		ui.Row(c).PaddingX(16).Height(38).Radius(999).Background(p.text).Shadow(0, 12, 30, -8, p.cardShade).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, iconCheck).Size(15, 15).TextColor(p.bg)
			ui.Text(c, a.toast).FontSize(12).FontWeight(550).TextColor(p.bg).MaxWidth(560).SingleLine()
		})
	})
	// Keep frames coming while the toast is visible so it disappears on time.
	c.After(time.Until(a.toastUntil))
}

func (a *App) viewDialog(c *ui.Context) {
	if a.dialog == dialogNone {
		return
	}
	open := true
	p := a.palette(c)
	backdropFill := func(backdrop ui.Element) { backdrop.Background(p.overlay) }
	show := func(title string, body func(), confirmLabel string, confirm func()) {
		ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
			backdropFill(backdrop)
			a.dialogPanel(c, panel, title, body, confirmLabel, confirm)
		})
	}
	switch a.dialog {
	case dialogNewLibrary:
		show(a.t("sidebar.createLibrary"), func() {
			p := a.palette(c)
			ui.Text(c, a.t("setup.createDescription")).FontSize(12).TextColor(p.muted)
			ui.TextInput(c, &a.dialogValue).Placeholder(a.t("setup.libraryName")).AutoFocus()
			if a.dialogError != "" {
				ui.Text(c, a.dialogError).FontSize(11).TextColor(p.danger)
			}
		}, a.t("common.create"), a.createLibraryFromDialog)
	case dialogNewFolder:
		title := a.t("sidebar.newFolder")
		if a.newFolderParentID != "" {
			title = a.t("sidebar.newSubfolder")
		}
		show(title, func() {
			ui.TextInput(c, &a.dialogValue).Placeholder(a.t("sidebar.folderName")).AutoFocus()
		}, a.t("common.create"), a.createFolderFromDialog)
	case dialogRenameFolder:
		show(a.t("common.rename"), func() {
			ui.TextInput(c, &a.dialogValue).AutoFocus()
		}, a.t("common.save"), a.renameFolderFromDialog)
	case dialogDeleteFolder:
		show(a.t("common.deleteFolder"), func() {
			ui.Text(c, a.dialogValue).FontSize(12)
		}, a.t("common.delete"), a.deleteFolderFromDialog)
	case dialogPluginInstall:
		show(a.t("settings.plugins.installTitle"), func() {
			p := a.palette(c)
			req := a.pendingInstall
			if req != nil {
				ui.Text(c, req.Name).FontSize(15).FontWeight(600).TextColor(p.text)
				ui.Text(c, req.ManifestURL).FontSize(11).TextColor(p.muted).Wrap()
			} else {
				ui.Text(c, a.t("settings.plugins.installHint")).FontSize(12).TextColor(p.muted)
				ui.TextInput(c, &a.dialogValue).Placeholder("https://…/plugin.json")
			}
		}, a.t("common.confirm"), a.installPluginFromDialog)
	case dialogTag:
		show(a.t("sidebar.tags"), func() {
			ui.TextInput(c, &a.dialogValue).Placeholder(a.t("tags.searchOrCreate")).AutoFocus()
		}, a.t("common.save"), a.addTagToActive)
	case dialogDeleteItems:
		a.viewDeleteDialog(c, &open)
	case dialogDeleteLastFolder:
		a.viewLastFolderDeleteDialog(c, &open)
	case dialogFolderPicker:
		a.viewFolderPickerDialog(c, &open)
	case dialogTagPicker:
		a.viewTagPickerDialog(c, &open)
	case dialogRescanLibrary:
		show(a.t("sidebar.rescanLibraryTitle"), func() {
			p := a.palette(c)
			ui.Text(c, a.tf("sidebar.rescanLibraryConfirm", map[string]any{"name": a.dialogValue})).FontSize(12).TextColor(p.muted).Wrap()
		}, a.t("common.confirm"), a.rescanLibraryFromDialog)
	}
	if !open {
		if a.dialog == dialogNewFolder {
			a.newFolderParentID = ""
		}
		if a.dialog == dialogRenameFolder || a.dialog == dialogDeleteFolder {
			a.folderDialogID = ""
		}
		if a.dialog == dialogFolderPicker {
			a.folderPickerItems = nil
			a.folderPickerQuery = ""
		}
		if a.dialog == dialogTagPicker {
			a.tagPickerItems = nil
			a.tagPickerQuery = ""
		}
		a.dialog = dialogNone
		a.dialogError = ""
		a.pendingDelete = nil
		a.pendingFolderDelete = nil
	}
}

// viewDeleteDialog shows the check for a smart-view delete: where the
// selection lives, and the two ways out of those folders (detach or trash).
func (a *App) viewDeleteDialog(c *ui.Context, open *bool) {
	req := a.pendingDelete
	if req == nil {
		a.dialog = dialogNone
		return
	}
	p := a.palette(c)
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		a.dialogPanelActions(c, panel, a.t("workspace.deleteTitle"), func() {
			ui.Text(c, a.tf("workspace.deleteConfirmMessage", map[string]any{"count": len(req.ids)})).
				FontSize(12).TextColor(p.muted).Wrap()
			if len(req.folderNames) > 0 {
				ui.Text(c, a.t("workspace.deleteConfirmFoldersHint")).
					FontSize(11.5).TextColor(p.muted).Margin(10, 0, 0, 0)
				ui.Text(c, strings.Join(req.folderNames, ", ")).
					FontSize(12).TextColor(p.text).Wrap()
			}
		}, func() {
			if ui.Button(c, a.t("common.cancel")).Clicked() {
				a.dialog = dialogNone
				a.pendingDelete = nil
			}
			if len(req.folderIDs) > 0 {
				if ui.Button(c, a.t("workspace.removeFromAllFolders")).Clicked() {
					a.removePendingDeleteFromFolders()
				}
			}
			if ui.PrimaryButton(c, a.t("contextMenu.moveToTrash")).Clicked() {
				a.confirmPendingDeleteToTrash()
			}
		})
	})
}

// viewLastFolderDeleteDialog warns that the current folder is the last place
// holding these items: confirming deletes the folder links and moves those
// items to the trash.
func (a *App) viewLastFolderDeleteDialog(c *ui.Context, open *bool) {
	req := a.pendingFolderDelete
	if req == nil {
		a.dialog = dialogNone
		return
	}
	p := a.palette(c)
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		a.dialogPanelActions(c, panel, a.t("workspace.deleteLastFolderTitle"), func() {
			ui.Text(c, a.t("workspace.deleteLastFolderMessage")).
				FontSize(12).TextColor(p.muted).Wrap()
			if len(req.plan.unlinkIDs) > 0 {
				ui.Text(c, a.t("workspace.deleteLastFolderOtherItems")).
					FontSize(11.5).TextColor(p.muted).Margin(10, 0, 0, 0).Wrap()
			}
		}, func() {
			if ui.Button(c, a.t("common.cancel")).Clicked() {
				a.dialog = dialogNone
				a.pendingFolderDelete = nil
			}
			if ui.PrimaryButton(c, a.t("common.delete")).Clicked() {
				a.confirmPendingFolderDelete()
			}
		})
	})
}

func (a *App) dialogPanel(c *ui.Context, panel ui.Element, title string, body func(), confirmLabel string, confirm func()) {
	a.dialogPanelActions(c, panel, title, body, func() {
		if ui.Button(c, a.t("common.cancel")).Clicked() {
			a.dialog = dialogNone
		}
		if ui.PrimaryButton(c, confirmLabel).Clicked() {
			confirm()
		}
	})
}

func (a *App) dialogPanelActions(c *ui.Context, panel ui.Element, title string, body func(), actions func()) {
	p := a.palette(c)
	panel.Width(420).Padding(22).Radius(14).Background(p.surface).Border(1, p.hairline).Shadow(0, 26, 64, -18, p.cardShade).Children(func() {
		ui.Column(c).FillWidth().Gap(14).Children(func() {
			ui.Text(c, title).FontSize(17).FontWeight(650).TextColor(p.text)
			body()
			ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(actions)
		})
	})
}

func (a *App) viewIndexProgressDialog(c *ui.Context) {
	if !a.indexProgressActive {
		return
	}
	open := true
	p := a.palette(c)
	current := a.indexProgressCurrent
	total := a.indexProgressTotal
	var ratio float32
	percent := 0
	if total > 0 {
		if current > total {
			current = total
		}
		ratio = float32(current) / float32(total)
		percent = int(ratio * 100)
	}
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		panel.Width(400).Padding(22).Radius(14).Background(p.surface).Border(1, p.hairline).Shadow(0, 26, 64, -18, p.cardShade).Children(func() {
			ui.Column(c).FillWidth().Gap(12).Children(func() {
				ui.Text(c, a.t("sidebar.indexingTitle")).FontSize(16).FontWeight(650).TextColor(p.text)
				ui.Text(c, a.t("sidebar.indexingDescription")).FontSize(12).TextColor(p.muted).Wrap()
				ui.Box(c).FillWidth().Height(8).Radius(4).Draw(func(pt *ui.Painter, r ui.Rect) {
					pt.Fill(r, p.hairline, 4)
					if ratio > 0 {
						fillW := r.W * ratio
						if fillW < 6 {
							fillW = 6
						}
						if fillW > r.W {
							fillW = r.W
						}
						pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: fillW, H: r.H}, p.accent, 4)
					}
				})
				ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
					ui.Text(c, a.tf("sidebar.indexingProgress", map[string]any{
						"current": current,
						"total":   total,
						"percent": percent,
					})).FontSize(11.5).TextColor(p.muted)
				})
			})
		})
	})
	// Keep modal open until indexing completes.
	_ = open
}

func (a *App) closePreview() {
	if trace := a.previewTrace; trace != nil {
		trace.record("info", "closed", trace.started, fmt.Sprintf("display_ready=%t", a.previewImageShown))
	}
	if a.previewImages != nil {
		a.previewImages.clear()
	}
	if a.filmstripImages != nil {
		a.filmstripImages.clear()
	}
	a.filmstripTrace = nil
	a.previewTrace = nil
	a.previewImageShown = false
	a.previewSlowLogged = false
	a.previewImagePath = ""
	a.previewSeq++
	a.previewBitmap = nil
	a.psdInfo = nil
	a.closeVideo()
	a.closePreviewWebView()
	a.previewOpen = false
	a.previewVariantID = ""
	a.previewText = ""
	a.previewError = ""
	if a.player != nil {
		a.player.Stop()
		a.player = nil
	}
}

func (a *App) openPreview(id string, mode string) {
	if mode == "" {
		mode = "inline"
	}
	a.activeItemID = id
	a.previewVariantID = ""
	a.scrollPreviewVariantID = id
	a.previewIndex = a.indexFor(id)
	a.previewOpen = true
	a.previewMode = mode
	a.previewZoom = 1
	a.previewPan = ui.Point{}
	a.loadPreviewContent()
}

func (a *App) indexFor(id string) int {
	for i := range a.items {
		if a.items[i].ID == id {
			return i
		}
	}
	return 0
}

func (a *App) movePreview(delta int) {
	if len(a.items) == 0 {
		return
	}
	n := a.previewIndex + delta
	if n < 0 {
		n = len(a.items) - 1
	}
	if n >= len(a.items) {
		n = 0
	}
	a.previewVariantID = ""
	a.previewIndex = n
	a.activeItemID = a.items[n].ID
	a.scrollPreviewVariantID = a.items[n].ID
	a.activeItem = &a.items[n]
	a.previewZoom = 1
	a.previewPan = ui.Point{}
	a.loadPreviewContent()
}
