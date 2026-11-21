package app

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"bowerbird/core"
	"bowerbird/internal/plugin"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// handleGalleryZoomInput adjusts the gallery row height on wheel scroll (Ctrl on Windows, Command on macOS) or touchpad pinch.
func (a *App) handleGalleryZoomInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputZoom:
		if ev.Scale != 0 && ev.Scale != 1 {
			newH := int(math.Round(float64(float32(a.rowHeight) * ev.Scale)))
			a.setRowHeight(newH)
			return true
		}
	case ui.InputScroll:
		hasZoomMod := ev.Mods&ui.Ctrl != 0
		if runtime.GOOS == "darwin" {
			hasZoomMod = ev.Mods&ui.Super != 0
		}
		if hasZoomMod {
			if ev.DY != 0 {
				delta := -int(math.Round(float64(ev.DY * 0.25)))
				if delta == 0 {
					if ev.DY < 0 {
						delta = 2
					} else {
						delta = -2
					}
				}
				a.setRowHeight(a.rowHeight + delta)
				return true
			}
		}
	}
	return false
}

// galleryShortcuts binds the keys that act on the gallery selection. They live
// on the gallery element rather than the window so a focused text field
// elsewhere in the window (search, notes, tags) keeps Backspace and Delete for
// editing; while the focus is in the gallery, the same keys remove the
// selection.
func (a *App) galleryShortcuts(container ui.Element) {
	if a.previewOpen || a.dialog != dialogNone {
		return
	}
	for _, id := range []string{core.ShortcutDelete, core.ShortcutDeleteBackspace} {
		pair, ok := a.shortcutPairs[id]
		if !ok || !pair.ok {
			continue
		}
		if container.Shortcut(pair.mods, pair.key) {
			a.deleteSelection()
			return
		}
	}
}

func (a *App) galleryGap() float32 {
	if a.prefs.LayoutMode == "list" {
		return 4
	}
	return 8
}

// viewGallery is the virtualized justified gallery. Only the rows the list
// builds exist; each row builds only its own cards, so tens of thousands of
// items scroll at display refresh.
func (a *App) viewGallery(c *ui.Context) {
	a.ensureRows()
	if len(a.rows) == 0 {
		container := ui.Box(c.Key("gallery-container")).Fill().Clip().Bind(&a.galleryBox)
		container.HandleInput(a.handleGalleryZoomInput)
		container.ContextMenu(a.viewContextMenu)
		a.galleryShortcuts(container)
		container.Children(func() {
			if a.loading {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Spinner(c)
				})
			} else {
				a.emptyState(c, iconImage, a.t("gallery.empty"), a.t("gallery.emptyHint"))
			}
		})
		return
	}
	top := float32(mainHeaderHeight + galleryTopPadding)
	bottom := float32(24)
	p := a.palette(c)
	a.galleryPaintSampleOK = false

	container := ui.Box(c.Key("gallery-container")).Fill().Clip().Bind(&a.galleryBox)
	a.galleryShortcuts(container)
	container.HandleInput(func(ev ui.InputEvent) bool {
		if a.handleGalleryZoomInput(ev) {
			return true
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button == 0 {
				// 卡片自身负责单击选中、双击预览和原生拖拽。只有从空白处
				// 按下时才由容器接管，开始橡皮筋框选；文件夹卡片和标题行
				// 也在这里让出 presses。
				if a.itemAtPoint(ev.X, ev.Y, top) != -1 {
					return false
				}
				// 让 Delete 这类作用在选区上的键留着用：焦点在画廊里，
				// 窗口里的输入框才不会被它抢走。
				a.galleryBox.Focus(c)
				a.marqueeActive = true
				a.marqueeStartX, a.marqueeStartY = ev.X, ev.Y
				a.marqueeStartContentY = ev.Y - a.galleryRowsOriginY(top)
				a.marqueeCurX, a.marqueeCurY = ev.X, ev.Y
				additive := ev.Mods&ui.Cmd != 0 || ev.Mods&ui.Ctrl != 0 || ev.Mods&ui.Shift != 0
				a.marqueeAdditive = additive
				if additive {
					a.marqueeBase = make(map[string]bool, len(a.selected))
					for k, v := range a.selected {
						if v {
							a.marqueeBase[k] = true
						}
					}
				} else {
					a.marqueeBase = nil
					a.clearSelection()
				}
				a.updateMarqueeSelection(top)
				if a.win != nil {
					a.win.Invalidate()
				}
				return true
			}
		case ui.InputPointerMove:
			if a.marqueeActive {
				a.marqueeCurX, a.marqueeCurY = ev.X, ev.Y
				// 自动边缘滚动: 拖选拉到最下方或最上方时跟随滚动
				bottomEdge := float32(500)
				if a.galleryBoxRect.H > 120 {
					bottomEdge = a.galleryBoxRect.H - 40
				}
				if ev.Y > bottomEdge {
					a.galleryScroll.Y += (ev.Y - bottomEdge) * 0.15
					if a.galleryScroll.MaxY > 0 && a.galleryScroll.Y > a.galleryScroll.MaxY {
						a.galleryScroll.Y = a.galleryScroll.MaxY
					}
				} else if ev.Y < top && a.galleryScroll.Y > 0 {
					a.galleryScroll.Y -= (top - ev.Y) * 0.15
					if a.galleryScroll.Y < 0 {
						a.galleryScroll.Y = 0
					}
				}
				a.updateMarqueeSelection(top)
				if a.win != nil {
					a.win.Invalidate()
				}
				return true
			}
		case ui.InputPointerUp:
			if a.marqueeActive {
				startY := a.galleryRowsOriginY(top) + a.marqueeStartContentY
				dx := a.marqueeCurX - a.marqueeStartX
				dy := a.marqueeCurY - startY
				if dx*dx+dy*dy < 16 && !a.marqueeAdditive {
					clickedItem := a.itemAtPoint(ev.X, ev.Y, top)
					if clickedItem >= 0 {
						a.chooseItem(clickedItem, 0)
					} else {
						a.clearSelection()
					}
				}
				a.marqueeActive = false
				a.marqueeBase = nil
				if len(a.selected) > 0 && a.activeItem == nil {
					for _, it := range a.items {
						if a.selected[it.ID] {
							a.activeItemID = it.ID
							cp := it
							a.activeItem = &cp
							a.loadInspector(it.ID)
							break
						}
					}
				}
				if a.win != nil {
					a.win.Invalidate()
				}
				return true
			}
		}
		return false
	})

	a.galleryDragEdgeScroll(c, container, top)

	container.Children(func() {
		a.visibleThumbCount = 0
		list := ui.List(c.Key("gallery"), &a.galleryList, len(a.rows), func(row int) {
			if row < 0 || row >= len(a.rows) {
				return
			}
			a.visibleThumbCount += len(a.rows[row])
			a.galleryRow(c, row, a.rows[row])
		}).FillWidth().Grow(1).Shrink(1).TrackScroll(&a.galleryScroll).Gap(a.galleryGap()).Padding(top, gallerySidePadding, bottom, gallerySidePadding).ScrollbarInsets(top, 0, bottom, 0)
		if a.galleryScroll.Y != a.lastScrollY {
			a.lastScrollY = a.galleryScroll.Y
			a.scheduleMemoryIdleCleanup()
		}
		if list.Clicked() && len(a.selected) > 0 {
			a.clearSelection()
			if a.win != nil {
				a.win.Invalidate()
			}
		}
	})
	container.ContextMenu(a.viewContextMenu)

	container.DrawOver(func(pt *ui.Painter, r ui.Rect) {
		a.galleryBoxRect = r
		if a.galleryPaintSampleOK {
			a.galleryLayoutSampleOK = true
			a.galleryLayoutSampleIdx = a.galleryPaintSampleIdx
			a.galleryLayoutSampleRelY = a.galleryPaintSampleY - r.Y
			a.galleryLayoutSampleScrollY = a.galleryScroll.Y
		} else {
			a.galleryLayoutSampleOK = false
		}
		if !a.marqueeActive {
			return
		}
		x0 := a.marqueeStartX
		y0 := a.galleryRowsOriginY(top) + a.marqueeStartContentY
		x1, y1 := a.marqueeCurX, a.marqueeCurY
		bx := minF(x0, x1)
		by := minF(y0, y1)
		bw := maxF(x0, x1) - bx
		bh := maxF(y0, y1) - by
		if bw < 2 && bh < 2 {
			return
		}
		boxRect := ui.Rect{X: r.X + bx, Y: r.Y + by, W: bw, H: bh}
		pt.Fill(boxRect, p.accent.Alpha(0.18), 3)
		pt.Stroke(boxRect, p.accent.Alpha(0.85), 3, 1)
	})
}

func (a *App) galleryDragEdgeScroll(c *ui.Context, container ui.Element, top float32) {
	if !a.itemDragging && !a.rootDataDragOver {
		return
	}
	r := a.galleryBoxRect
	if r.W <= 0 || r.H <= 0 {
		return
	}
	px, py, _ := container.PointerPosition()
	if px < 0 || px > r.W || py < 0 || py > r.H {
		return
	}
	const scrollZone float32 = 48
	const maxStep float32 = 14
	topZoneEnd := top + scrollZone
	var dy float32
	if py < topZoneEnd && a.galleryScroll.Y > 0 {
		// The floating header covers [0, mainHeaderHeight), so mygo's built-in
		// dragScroll never sees the gallery's visible top edge. Scroll up
		// whenever the pointer enters the top zone or the header above it.
		t := (topZoneEnd - py) / (topZoneEnd - mainHeaderHeight)
		if t > 1 {
			t = 1
		}
		dy = -maxStep * t
	} else if r.H > 120 && py > r.H-scrollZone && py <= r.H-32 && a.galleryScroll.Y < a.galleryScroll.MaxY {
		// Smooth lead-in before mygo's built-in 32px bottom dragScroll kicks in.
		dy = maxStep * (py - (r.H - scrollZone)) / scrollZone
	}
	if dy == 0 {
		return
	}
	nextY := a.galleryScroll.Y + dy
	if nextY < 0 {
		nextY = 0
	}
	if a.galleryScroll.MaxY > 0 && nextY > a.galleryScroll.MaxY {
		nextY = a.galleryScroll.MaxY
	}
	if nextY != a.galleryScroll.Y {
		a.galleryScroll.Y = nextY
		c.Invalidate()
	}
}

func (a *App) galleryRowTopOffset(targetRow int) float32 {
	if targetRow <= 0 || len(a.rows) == 0 {
		return 0
	}
	if targetRow > len(a.rows) {
		targetRow = len(a.rows)
	}
	gap := a.galleryGap()
	var y float32
	for i := 0; i < targetRow; i++ {
		if len(a.rows[i]) == 0 {
			continue
		}
		y += float32(a.rows[i][0].height) + gap
	}
	return y
}

// galleryRowsOriginY returns the Y coordinate of row 0's top edge in
// galleryBox-local coordinates. When a rendered row was sampled during paint,
// it anchors to that row's actual painted position instead of ListState's
// estimated scrollY, which drifts from the sum of row heights on large lists.
func (a *App) galleryRowsOriginY(padTop float32) float32 {
	if a.galleryLayoutSampleOK && a.galleryLayoutSampleIdx >= 0 && a.galleryLayoutSampleIdx < len(a.rows) {
		return a.galleryLayoutSampleRelY - a.galleryRowTopOffset(a.galleryLayoutSampleIdx) - (a.galleryScroll.Y - a.galleryLayoutSampleScrollY)
	}
	return padTop - a.galleryScroll.Y
}

func (a *App) itemAtPoint(x, y, padTop float32) int {
	targetY := y
	gap := a.galleryGap()
	padLeft := float32(gallerySidePadding)
	curY := a.galleryRowsOriginY(padTop)

	for _, row := range a.rows {
		if len(row) == 0 {
			continue
		}
		rowH := float32(row[0].height)
		rowTop := curY
		rowBottom := curY + rowH
		curY = rowBottom + gap

		if targetY < rowTop || targetY > rowBottom {
			continue
		}

		curX := padLeft
		for _, cell := range row {
			cellW := float32(cell.width)
			cellLeft := curX
			cellRight := curX + cellW
			curX = cellRight + gap

			if x >= cellLeft && x <= cellRight {
				return cell.index
			}
		}
	}
	return -1
}

func (a *App) updateMarqueeSelection(padTop float32) {
	x0 := a.marqueeStartX
	y0 := a.galleryRowsOriginY(padTop) + a.marqueeStartContentY
	x1, y1 := a.marqueeCurX, a.marqueeCurY
	boxLeft := minF(x0, x1)
	boxRight := maxF(x0, x1)
	boxTop := minF(y0, y1)
	boxBottom := maxF(y0, y1)

	hit := make(map[string]bool)
	gap := a.galleryGap()
	padLeft := float32(gallerySidePadding)
	curY := a.galleryRowsOriginY(padTop)

	for _, row := range a.rows {
		if len(row) == 0 {
			continue
		}
		rowH := float32(row[0].height)
		rowTop := curY
		rowBottom := curY + rowH
		curY = rowBottom + gap

		if rowBottom < boxTop || rowTop > boxBottom {
			continue
		}

		curX := padLeft
		for _, cell := range row {
			cellW := float32(cell.width)
			cellLeft := curX
			cellRight := curX + cellW
			curX = cellRight + gap

			if cell.index < 0 || cell.index >= len(a.items) {
				continue
			}
			if !(cellRight < boxLeft || cellLeft > boxRight) {
				hit[a.items[cell.index].ID] = true
			}
		}
	}

	newSelected := make(map[string]bool)
	if a.marqueeAdditive && a.marqueeBase != nil {
		for id := range a.marqueeBase {
			newSelected[id] = true
		}
	}
	for id := range hit {
		newSelected[id] = true
	}
	a.selected = newSelected
	// Maintain FIFO selectedOrder: retain existing selected IDs in order, append newly hit IDs
	var nextOrder []string
	seen := make(map[string]bool)
	for _, id := range a.selectedOrder {
		if a.selected[id] {
			nextOrder = append(nextOrder, id)
			seen[id] = true
		}
	}
	for _, it := range a.items {
		if a.selected[it.ID] && !seen[it.ID] {
			nextOrder = append(nextOrder, it.ID)
			seen[it.ID] = true
		}
	}
	a.selectedOrder = nextOrder

	if len(a.selectedOrder) > 0 {
		lastID := a.selectedOrder[len(a.selectedOrder)-1]
		a.activeItemID = lastID
		for i, it := range a.items {
			if it.ID == lastID {
				cp := it
				a.activeItem = &cp
				a.selectedAt = i
				break
			}
		}
	} else {
		a.activeItemID = ""
		a.activeItem = nil
		a.selectedAt = -1
	}
}
func (a *App) recordGalleryRowPaint(rowIdx int, r ui.Rect) {
	if !a.galleryPaintSampleOK || rowIdx < a.galleryPaintSampleIdx {
		a.galleryPaintSampleOK = true
		a.galleryPaintSampleIdx = rowIdx
		a.galleryPaintSampleY = r.Y
	}
}

func (a *App) galleryRow(c *ui.Context, rowIdx int, row []galleryCell) {
	if len(row) == 0 {
		return
	}
	if row[0].folderHeader {
		a.folderSectionHeader(c, rowIdx, row[0].width)
		return
	}
	height := float32(row[0].height)
	rowEl := ui.Row(c).FillWidth().Height(height).Gap(a.galleryGap()).PassThrough()
	rowEl.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		a.recordGalleryRowPaint(rowIdx, r)
	})
	rowEl.Children(func() {
		for _, cell := range row {
			if cell.folderID != "" {
				a.folderCard(c, cell.folderID, float32(cell.width), float32(cell.height))
				continue
			}
			if cell.index < 0 || cell.index >= len(a.items) {
				continue
			}
			item := a.workspaceItem(cell.index)
			a.backfillMissingThumbnails([]core.Item{item})
			a.itemCard(c, item, cell.index, float32(cell.width), float32(cell.height))
		}
	})
}

// folderSectionRows puts the current folder's children ahead of its items.
// The rows take part in the gallery's normal list layout, so folder cards
// scroll and virtualize with the rest of the workspace.
func (a *App) folderSectionRows(width int) [][]galleryCell {
	if a.nav != NavFolder || a.folderID == "" {
		return nil
	}
	children := a.folderIndex().children[a.folderID]
	if len(children) == 0 {
		return nil
	}
	rows := [][]galleryCell{{
		{index: -2, width: width, height: 30, folderHeader: true},
	}}
	if a.subfoldersCollapsed {
		return rows
	}
	const gap = 8
	const targetW = 170
	const cardH = 168
	cols := (width + gap) / (targetW + gap)
	if cols < 1 {
		cols = 1
	}
	colW := (width - (cols-1)*gap) / cols
	if colW < 120 {
		colW = 120
	}
	var row []galleryCell
	for _, f := range children {
		row = append(row, galleryCell{index: -2, width: colW, height: cardH, folderID: f.ID})
		if len(row) == cols {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func (a *App) folderSectionHeader(c *ui.Context, rowIdx int, width int) {
	p := a.palette(c)
	children := a.folderIndex().children[a.folderID]
	b := ui.ButtonBase(c.Key("subfolders-toggle")).Width(float32(width)).Height(30).PaddingX(4).Radius(6).FocusRing(false).AlignItems(ui.Center).Gap(6)
	b.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		a.recordGalleryRowPaint(rowIdx, r)
	})
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ic := iconChevron
		if a.subfoldersCollapsed {
			ic = iconChevronR
		}
		ui.Icon(c, ic).Size(13, 13).TextColor(p.muted)
		ui.Text(c, a.tf("workspace.subfoldersCount", map[string]any{"count": len(children)})).FontSize(12).FontWeight(650).TextColor(p.text)
	})
	if b.Clicked() {
		a.subfoldersCollapsed = !a.subfoldersCollapsed
	}
}

func (a *App) folderCard(c *ui.Context, folderID string, w, h float32) {
	f, ok := a.folderIndex().byID[folderID]
	if !ok {
		return
	}
	p := a.palette(c)
	card := ui.ButtonBase(c.Key("folder-card-"+f.ID)).Width(w).Height(h).Padding(6).Radius(10).Border(1, p.hairline).FocusRing(false).Column().Gap(6)
	fileDropOpts := transfer.DropOptions{
		Formats:    []transfer.Format{transfer.FileList, transfer.URIList, transfer.Text, transfer.HTML},
		Operations: transfer.Copy | transfer.Move,
	}
	_, dataOver := ui.DataDragOver(card, fileDropOpts)
	nativeOver := dataOver || card.FileDragOver()
	bg := p.surface
	if card.Hovered() || nativeOver {
		bg = p.hover
		a.dropFolderID = f.ID
	}
	if dragged, over := ui.DragOver[sidebarDrag](card); (over && len(dragged.ItemIDs) > 0) || nativeOver {
		bg = p.press
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	card.Children(func() {
		preview := a.folderPreviews[f.ID]
		thumbH := h - 44
		if thumbH < 60 {
			thumbH = 60
		}
		ui.Box(c).FillWidth().Height(thumbH).Radius(7).Background(p.surface2).Clip().Children(func() {
			switch len(preview.Items) {
			case 0:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Icon(c, iconFolder).Size(28, 28).TextColor(p.accent)
				})
			case 1:
				a.folderPreviewImage(c, preview.Items[0], int(w*2))
			default:
				ui.Column(c).Fill().Gap(1).Children(func() {
					for r := 0; r < 2; r++ {
						ui.Row(c).FillWidth().Grow(1).Gap(1).Children(func() {
							for col := 0; col < 2; col++ {
								idx := r*2 + col
								slot := ui.Box(c).Grow(1).Shrink(1).FillHeight().Background(p.hover)
								slot.Children(func() {
									if idx < len(preview.Items) {
										a.folderPreviewImage(c, preview.Items[idx], int(w))
									} else {
										ui.Column(c).Fill().Center().Children(func() {
											ui.Icon(c, iconFolder).Size(14, 14).TextColor(p.faint.Alpha(0.55))
										})
									}
								})
							}
						})
					}
				})
			}
		})
		count := f.ItemCount
		if preview.Total > 0 {
			count = preview.Total
		}
		ui.Row(c).FillWidth().Height(20).AlignItems(ui.Center).Gap(6).PaddingX(2).Children(func() {
			ui.Icon(c, iconFolder).Size(14, 14).TextColor(p.muted)
			ui.Text(c, f.Name).FontSize(12).FontWeight(550).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			ui.Textf(c, "%d", count).FontSize(10.5).TextColor(p.faint).FontFeatures("tnum")
		})
	})
	if card.Clicked() {
		a.navigate(NavFolder, f.ID)
	}
	card.ContextMenu(func(m *ui.Menu) {
		a.folderContextMenu(m, f)
	})
	if dropped, ok := ui.Drop[sidebarDrag](card); ok && len(dropped.ItemIDs) > 0 {
		a.moveItemsToFolder(dropped.ItemIDs, f.ID)
	}
	if drop, ok := ui.DropData(card, fileDropOpts); ok {
		a.handleDropDataToFolder(drop.Data, f.ID)
	}
}

func (a *App) folderPreviewImage(c *ui.Context, item core.Item, targetPx int) {
	a.backfillMissingThumbnails([]core.Item{item})
	if bmp := a.itemThumbBitmap(item, targetPx); bmp != nil {
		ui.Image(c, bmp).Fill().Fit(ui.Cover)
		return
	}
	ui.Column(c).Fill().Center().Children(func() {
		ui.Icon(c, categoryIcon(item)).Size(16, 16).TextColor(a.palette(c).faint)
	})
}

func (a *App) itemThumbBitmap(item core.Item, targetPx int) *ui.Bitmap {
	path := ""
	if item.HasThumbnail && item.ItemPath != "" {
		path = core.ResolveThumbnailPath(item.ItemPath)
	} else if categoryOf(item) == "image" && !a.canGenerateThumbnail(item) {
		path = item.FilePath
	}
	if path == "" {
		return nil
	}
	return a.thumbs.get(path, targetPx)
}

func (a *App) itemCard(c *ui.Context, item core.Item, index int, w, h float32) {
	p := a.palette(c)
	selected := a.selected[item.ID]
	isList := a.prefs.LayoutMode == "list"

	card := ui.ButtonBase(c.Key("item-" + item.ID)).Width(w).Height(h).FocusRing(false)
	if isList {
		card.Row().AlignItems(ui.Center).Gap(10).PaddingX(8).PaddingY(4)
	} else {
		card.Column().Gap(4).AlignItems(ui.Start).Padding(3)
	}
	card.Cursor(ui.CursorPointer)
	card.Radius(8)
	bg := ui.Transparent
	if isList {
		if selected {
			bg = p.surface2
		} else if card.Hovered() {
			bg = p.hover
		}
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	// Native drag out: advertises the file list to Finder and other apps. The
	// preview is the image itself (a stack for a multi-selection), not the
	// card's background; it is only built for the card under the pointer.
	// The typed payload lets folder rows in this process classify the drag
	// without importing the files again.
	if card.Hovered() || card.Pressed() {
		card.Drag(sidebarDrag{ItemIDs: a.itemDragIDs(item)})
	}
	card.DragDataFrom(func() transfer.Data { return a.itemDragData(item) }, a.dragOptions(card, item))

	var dropVariantOver bool
	var fileDropOpts transfer.DropOptions
	if a.nav != NavTrash {
		fileDropOpts = transfer.DropOptions{
			Formats:    []transfer.Format{transfer.FileList, transfer.URIList},
			Operations: transfer.Copy | transfer.Move,
		}
		_, dataOver := ui.DataDragOver(card, fileDropOpts)
		nativeOver := dataOver || card.FileDragOver()
		if nativeOver {
			a.dropItemID = item.ID
			a.dropFolderID = ""
		}
		if dragged, over := ui.DragOver[sidebarDrag](card); over && len(dragged.ItemIDs) > 0 {
			for _, id := range dragged.ItemIDs {
				if id != "" && id != item.ID {
					dropVariantOver = true
					break
				}
			}
		}
		if nativeOver {
			dropVariantOver = true
		}
		if dropVariantOver && isList {
			card.Background(p.press)
		}
	}

	// Resolve thumbnail or system file icon
	path := ""
	if item.HasThumbnail && item.ItemPath != "" {
		path = core.ResolveThumbnailPath(item.ItemPath)
	} else if categoryOf(item) == "image" && !a.canGenerateThumbnail(item) {
		path = item.FilePath
	}

	var bmp *ui.Bitmap
	if path != "" {
		bmp = a.thumbs.get(path, int(w*2))
	}
	var sysIconBmp *ui.Bitmap
	if bmp == nil && path == "" && item.FilePath != "" && categoryOf(item) != "image" {
		sysIconBmp = a.thumbs.getSystemIcon(item.Extension, item.FilePath)
	}

	if isList {
		card.Children(func() {
			thumbSize := h - 8
			if thumbSize < 24 {
				thumbSize = 24
			}
			thumbBox := ui.Box(c).Size(thumbSize, thumbSize).Radius(6).Clip()
			if bmp == nil {
				thumbBox.Background(p.surface2)
			}
			thumbBox.Children(func() {
				if bmp != nil {
					ui.Image(c, bmp).Fill().Fit(ui.Cover)
				} else if sysIconBmp != nil {
					ui.Image(c, sysIconBmp).Fill().Fit(ui.Contain)
				} else {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Icon(c, categoryIcon(item)).Size(16, 16).TextColor(p.faint)
					})
				}
			})

			// Name
			ui.Text(c, item.Name).FontSize(12).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0).SingleLine()

			if len(item.Variants) > 0 {
				ui.Row(c).AlignItems(ui.Center).Gap(3).Padding(2, 6).Radius(4).Background(p.accent).Children(func() {
					ui.Icon(c, iconLayers).Size(10, 10).TextColor(p.onAccent)
					ui.Textf(c, "%d", len(item.Variants)+1).FontSize(9.5).FontWeight(650).TextColor(p.onAccent)
				})
			} else if item.VariantOf != "" {
				ui.Row(c).AlignItems(ui.Center).Gap(3).Padding(2, 6).Radius(4).Background(p.surface2).Children(func() {
					ui.Icon(c, iconLayers).Size(10, 10).TextColor(p.muted)
				})
			}

			// Rating
			if item.Rating > 0 {
				ui.Row(c).AlignItems(ui.Center).Gap(2).Children(func() {
					ui.Icon(c, iconStarFill).Size(11, 11).TextColor(p.star)
					ui.Textf(c, "%d", item.Rating).FontSize(11).FontWeight(650).TextColor(p.star)
				})
			}

			// Dimension or Category
			dimStr := "-"
			if item.Width > 0 && item.Height > 0 {
				dimStr = fmt.Sprintf("%d × %d", item.Width, item.Height)
			}
			ui.Text(c, dimStr).FontSize(11).TextColor(p.faint).Width(90).TextAlign(ui.End).FontFeatures("tnum").SingleLine()

			// Extension badge
			extStr := strings.ToUpper(item.Extension)
			if extStr == "" {
				extStr = "-"
			}
			ui.Box(c).Width(48).Height(20).Radius(4).Background(p.surface2).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, extStr).FontSize(9).FontWeight(650).TextColor(p.muted)
			})

			// Size
			ui.Text(c, formatBytes(item.Size)).FontSize(11).TextColor(p.faint).Width(70).TextAlign(ui.End).FontFeatures("tnum").SingleLine()
		})
	} else {
		card.Children(func() {
			nameH := float32(0)
			if a.prefs.ShowNames {
				nameH = 20
			}
			imgH := h - nameH - 8
			if imgH < 24 {
				imgH = h - 8
				nameH = 0
			}
			buildThumb := func(thumbBox ui.Element) {
				if bmp == nil {
					thumbBox.Background(p.surface2)
				}

				thumbBox.Children(func() {
					fadeTarget := float32(0)
					if bmp != nil || sysIconBmp != nil {
						fadeTarget = 1
					}
					op := card.AnimateTo("fade-"+item.ID, fadeTarget, ui.Animation{Duration: 320 * time.Millisecond, Ease: ui.EaseOut})
					if bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Opacity(maxF(op, 0.01))
					} else if sysIconBmp != nil {
						ui.Column(c).Fill().Center().Children(func() {
							iconSize := minF(imgH*0.65, 64)
							ui.Image(c, sysIconBmp).Size(iconSize, iconSize).Fit(ui.Contain).Opacity(maxF(op, 0.01))
						})
					} else {
						ui.Column(c).Fill().Center().Gap(4).Children(func() {
							ui.Icon(c, categoryIcon(item)).Size(24, 24).TextColor(p.faint)
							if !item.HasThumbnail && categoryOf(item) != "image" {
								ui.Text(c, strings.ToUpper(item.Extension)).FontSize(9).FontWeight(650).TextColor(p.faint)
							}
						})
					}
					// Variant stack / child badge at top-left.
					if len(item.Variants) > 0 {
						ui.Row(c).Absolute().Top(6).Left(6).Padding(2, 6).Radius(4).Background(p.accent).Gap(3).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, iconLayers).Size(9, 9).TextColor(p.onAccent)
							ui.Textf(c, "%d", len(item.Variants)+1).FontSize(9).FontWeight(700).TextColor(p.onAccent)
						})
					} else if item.VariantOf != "" {
						ui.Row(c).Absolute().Top(6).Left(6).Padding(2, 5).Radius(4).Background(p.badge).Gap(2).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, iconLayers).Size(9, 9).TextColor(p.onBadge)
						})
					}
					// Format / duration badge.
					if a.prefs.ShowExt && item.Extension != "" {
						badge := strings.ToUpper(item.Extension)
						ui.Box(c).Absolute().Bottom(6).Right(6).Padding(2, 6).Radius(4).Background(p.badge).Children(func() {
							ui.Text(c, badge).FontSize(9).FontWeight(650).TextColor(p.onBadge)
						})
					}
					if item.Rating > 0 {
						ui.Row(c).Absolute().Top(6).Right(6).Padding(2, 6).Radius(4).Background(p.badge).Gap(2).Children(func() {
							ui.Icon(c, iconStarFill).Size(9, 9).TextColor(p.star)
							ui.Textf(c, "%d", item.Rating).FontSize(9).FontWeight(650).TextColor(p.onBadge)
						})
					}
					if dropVariantOver {
						ui.Box(c).Absolute().Fill().Radius(7).Background(p.accent.Alpha(0.16)).Border(2, p.accent)
					} else if selected {
						ui.Box(c).Absolute().Fill().Radius(7).Border(2, p.accent)
					}
				})
			}

			if len(item.Variants) > 0 {
				backLayers := min(len(item.Variants), 3)
				rightPad := float32(backLayers*3 + 2)
				topPad := float32(backLayers + 1)
				bottomPad := float32(2)
				ui.Box(c).FillWidth().Height(imgH).Children(func() {
					for layer := backLayers; layer >= 1; layer-- {
						shiftX := float32(layer) * 2.5
						shiftY := float32(layer) * 0.5
						ui.Box(c).Absolute().
							Left(shiftX).Top(topPad - shiftY).Right(rightPad - shiftX).Bottom(bottomPad + shiftY).
							Radius(7).Background(p.surface2).Border(1, p.hairline).
							TransformOrigin(0.2, 0.85).Rotate(float32(layer) * 1.8).
							Shadow(0, 1.5, 4, 0, ui.RGBA(0, 0, 0, 0.13))
					}
					buildThumb(ui.Box(c).Absolute().Left(0).Top(topPad).Right(rightPad).Bottom(bottomPad).
						Radius(7).Clip().Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.18)))
				})
			} else {
				buildThumb(ui.Box(c).FillWidth().Height(imgH).Radius(7).Clip())
			}
			if nameH > 0 {
				ui.Row(c).FillWidth().Height(nameH).PaddingX(2).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					nameBox := ui.Box(c).PaddingX(6).PaddingY(2).Radius(4).AlignItems(ui.Center).Justify(ui.Center)
					nameColor := p.text
					if selected {
						nameBox.Background(p.accent)
						nameColor = p.onAccent
					}
					nameWeight := 400
					if selected {
						nameWeight = 600
					}
					nameBox.Children(func() {
						ui.Text(c, item.Name).FontSize(11).FontWeight(nameWeight).TextColor(nameColor).Grow(1).Shrink(1).MinWidth(0).SingleLine().TextAlign(ui.Center)
					})
				})
			}
		})
	}
	if mods, key, ok := a.shortcutKeys(core.ShortcutTogglePreview); ok && card.Shortcut(mods, key) {
		if a.previewOpen {
			a.closePreview()
		} else {
			a.openPreview(item.ID, "full")
		}
	} else if card.DoubleClicked() {
		a.clearItemClick()
		a.openPreview(item.ID, "inline")
	} else if card.Clicked() {
		mods := card.ClickModifiers()
		if mods == 0 {
			mods = c.Modifiers()
		}
		a.handleItemClick(item, index, mods)
	}
	card.ContextMenu(func(m *ui.Menu) {
		if !a.selected[item.ID] {
			a.chooseItem(index, 0)
		}
		a.itemContextMenu(m, item)
	})
	if a.nav != NavTrash {
		if dropped, ok := ui.Drop[sidebarDrag](card); ok && len(dropped.ItemIDs) > 0 {
			a.linkItemsAsVariants(item.ID, dropped.ItemIDs)
		}
		if drop, ok := ui.DropData(card, fileDropOpts); ok {
			if files, err := drop.Data.Files(); err == nil && len(files) > 0 {
				a.addVariantFiles(item.ID, files)
			}
		}
	}
}

// sidebarDrag is the in-app payload shared by gallery cards and folder rows.
// Items use ItemIDs to classify existing assets into a folder; folders use
// FolderID and the ParentID captured at drag start to reorder siblings.
type sidebarDrag struct {
	ItemIDs  []string
	FolderID string
	ParentID string
}

// itemDragIDs returns the items a drag anchored at item carries: the whole
// selection when item is part of a multi-selection, otherwise just item.
func (a *App) itemDragIDs(item core.Item) []string {
	if len(a.selected) <= 1 || !a.selected[item.ID] {
		return []string{item.ID}
	}
	ids := make([]string, 0, len(a.selected))
	for _, it := range a.items {
		if a.selected[it.ID] {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

// moveItemsToFolder moves dropped items into targetFolderID, removing them
// from the currently viewed folder (or replacing all folder memberships when
// dragged from a non-folder view).
func (a *App) moveItemsToFolder(ids []string, targetFolderID string) {
	if a.svc == nil || targetFolderID == "" {
		return
	}
	fromFolderID := ""
	if a.nav == NavFolder {
		fromFolderID = a.folderID
	}
	for _, id := range ids {
		_ = a.svc.MoveItemToFolder(id, fromFolderID, targetFolderID)
	}
	a.refreshAll()
}

// itemDragData builds the native file-list drag for a gallery card or the
// inspector hero. Dragging a member of a multi-selection carries the whole
// selection; dragging any other item carries just that item.
func (a *App) itemDragData(item core.Item) transfer.Data {
	a.itemDragging = true
	ids := a.itemDragIDs(item)
	if a.svc != nil && len(ids) > 0 {
		copied := append([]string(nil), ids...)
		go func() {
			if err := a.svc.RecordRecentlyUsed(copied); err == nil {
				if recentIDs, err := a.svc.GetRecentlyUsedIDs(); err == nil {
					a.post(func() {
						a.recentUsedCount = len(recentIDs)
					})
				}
			}
		}()
	}
	urls := make([]string, 0, len(ids))
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var selected []core.Item
	for _, it := range a.items {
		if wanted[it.ID] {
			selected = append(selected, it)
		}
	}
	for _, it := range a.fullWorkspaceItems(selected) {
		if it.FileURL != "" {
			urls = append(urls, it.FileURL)
		}
	}
	return transfer.New(transfer.NewItem(transfer.Bytes(transfer.FileList, []byte(strings.Join(urls, "\r\n")))))
}

// clearItemClick forgets the click that may start a manual double click.
func (a *App) clearItemClick() {
	a.lastClickID = ""
	a.lastClickTime = time.Time{}
}

// itemClickCompletesDouble records a click and reports whether it completes
// a quick second click on the same item. It is the fallback for platforms
// that do not report click counts; the native DoubleClicked is preferred.
func (a *App) itemClickCompletesDouble(id string) bool {
	now := time.Now()
	if a.lastClickID == id && now.Sub(a.lastClickTime) < 350*time.Millisecond {
		a.clearItemClick()
		return true
	}
	a.lastClickID = id
	a.lastClickTime = now
	return false
}

// handleItemClick selects a card, with a manual double-click fallback for
// platforms that do not report click counts.
func (a *App) handleItemClick(item core.Item, index int, mods ui.Modifiers) {
	if a.itemClickCompletesDouble(item.ID) {
		a.openPreview(item.ID, "inline")
		return
	}
	a.chooseItem(index, mods)
}

// rangeSelectIndices computes the indices of items between anchorIdx and targetIdx
// taking into account the geometry of the current layout (list, grid, justified).
func (a *App) rangeSelectIndices(anchorIdx, targetIdx int) []int {
	if len(a.items) == 0 {
		return nil
	}
	if anchorIdx < 0 {
		anchorIdx = 0
	}
	if anchorIdx >= len(a.items) {
		anchorIdx = len(a.items) - 1
	}
	if targetIdx < 0 {
		targetIdx = 0
	}
	if targetIdx >= len(a.items) {
		targetIdx = len(a.items) - 1
	}
	if anchorIdx == targetIdx {
		return []int{targetIdx}
	}

	mode := a.prefs.LayoutMode
	switch mode {
	case "list":
		// 1D: consecutive linear range
		start := min(anchorIdx, targetIdx)
		end := max(anchorIdx, targetIdx)
		out := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			out = append(out, i)
		}
		return out

	case "grid":
		// 2D grid matrix box selection: [minRow..maxRow] x [minCol..maxCol]
		anchorRow, anchorCol := -1, -1
		targetRow, targetCol := -1, -1
		for rIdx, row := range a.rows {
			for cIdx, cell := range row {
				if cell.index == anchorIdx {
					anchorRow, anchorCol = rIdx, cIdx
				}
				if cell.index == targetIdx {
					targetRow, targetCol = rIdx, cIdx
				}
			}
		}
		if anchorRow >= 0 && targetRow >= 0 {
			minRow, maxRow := min(anchorRow, targetRow), max(anchorRow, targetRow)
			minCol, maxCol := min(anchorCol, targetCol), max(anchorCol, targetCol)
			var out []int
			seen := make(map[int]bool)
			for r := minRow; r <= maxRow; r++ {
				if r < 0 || r >= len(a.rows) {
					continue
				}
				for c := minCol; c <= maxCol; c++ {
					if c >= 0 && c < len(a.rows[r]) {
						idx := a.rows[r][c].index
						if idx >= 0 && idx < len(a.items) && !seen[idx] {
							seen[idx] = true
							out = append(out, idx)
						}
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}

	case "justified":
		// 2D justified stream selection: row span with horizontal projection overlap
		gap := a.galleryGap()
		anchorRow, targetRow := -1, -1
		var anchorX0, anchorX1, targetX0, targetX1 float32

		for rIdx, row := range a.rows {
			curX := float32(gallerySidePadding)
			for _, cell := range row {
				cellW := float32(cell.width)
				x0 := curX
				x1 := curX + cellW
				if cell.index == anchorIdx {
					anchorRow = rIdx
					anchorX0, anchorX1 = x0, x1
				}
				if cell.index == targetIdx {
					targetRow = rIdx
					targetX0, targetX1 = x0, x1
				}
				curX = x1 + gap
			}
		}

		if anchorRow >= 0 && targetRow >= 0 {
			minRow, maxRow := min(anchorRow, targetRow), max(anchorRow, targetRow)
			minX, maxX := minF(anchorX0, targetX0), maxF(anchorX1, targetX1)

			var out []int
			seen := make(map[int]bool)
			for r := minRow; r <= maxRow; r++ {
				if r < 0 || r >= len(a.rows) {
					continue
				}
				curX := float32(gallerySidePadding)
				for _, cell := range a.rows[r] {
					cellW := float32(cell.width)
					x0 := curX
					x1 := curX + cellW
					curX = x1 + gap

					if cell.index < 0 || cell.index >= len(a.items) {
						continue
					}
					// Overlap with [minX, maxX]
					if x0 <= maxX && x1 >= minX {
						if !seen[cell.index] {
							seen[cell.index] = true
							out = append(out, cell.index)
						}
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}

	// Sequential fallback
	start := min(anchorIdx, targetIdx)
	end := max(anchorIdx, targetIdx)
	out := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	return out
}

func (a *App) chooseItem(index int, mods ui.Modifiers) {
	if index < 0 || index >= len(a.items) {
		return
	}
	item := a.items[index]
	isShift := mods&ui.Shift != 0
	isCtrl := mods&ui.Ctrl != 0 || mods&ui.Cmd != 0 || mods&ui.Super != 0

	// 1. Shift range selection
	if isShift && a.selectedAnchor >= 0 && a.selectedAnchor < len(a.items) {
		indices := a.rangeSelectIndices(a.selectedAnchor, index)
		if !isCtrl {
			a.selected = make(map[string]bool, len(indices))
			a.selectedOrder = make([]string, 0, len(indices))
			for _, idx := range indices {
				itID := a.items[idx].ID
				a.selected[itID] = true
				a.selectedOrder = append(a.selectedOrder, itID)
			}
		} else {
			for _, idx := range indices {
				itID := a.items[idx].ID
				if !a.selected[itID] {
					a.selected[itID] = true
					a.selectedOrder = append(a.selectedOrder, itID)
				}
			}
		}
		// Shift preserves selectedAnchor for continuous range adjustments
		a.selectedAt = index
		a.activeItemID = item.ID
		cp := item
		a.activeItem = &cp
		a.loadInspector(item.ID)
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}

	// 2. Ctrl / Cmd toggle selection
	if isCtrl {
		if a.selected[item.ID] {
			delete(a.selected, item.ID)
			newOrder := make([]string, 0, len(a.selectedOrder))
			for _, id := range a.selectedOrder {
				if id != item.ID {
					newOrder = append(newOrder, id)
				}
			}
			a.selectedOrder = newOrder
			if len(a.selectedOrder) > 0 {
				lastID := a.selectedOrder[len(a.selectedOrder)-1]
				a.activeItemID = lastID
				for i, it := range a.items {
					if it.ID == lastID {
						cp := it
						a.activeItem = &cp
						a.selectedAt = i
						break
					}
				}
				a.loadInspector(a.activeItemID)
			} else {
				a.activeItemID = ""
				a.activeItem = nil
				a.selectedAt = -1
				a.loadInspector("")
			}
		} else {
			a.selected[item.ID] = true
			a.selectedOrder = append(a.selectedOrder, item.ID)
			a.activeItemID = item.ID
			cp := item
			a.activeItem = &cp
			a.selectedAt = index
			a.loadInspector(item.ID)
		}
		a.selectedAnchor = index
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}

	// 3. Normal single click
	a.selected = map[string]bool{item.ID: true}
	a.selectedOrder = []string{item.ID}
	a.selectedAt = index
	a.selectedAnchor = index
	a.activeItemID = item.ID
	cp := item
	a.activeItem = &cp
	a.loadInspector(item.ID)
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) moveGallerySelection(dx, dy int) {
	if len(a.items) == 0 {
		return
	}
	currentIdx := a.selectedAt
	if currentIdx < 0 || currentIdx >= len(a.items) {
		if a.activeItemID != "" {
			for i, it := range a.items {
				if it.ID == a.activeItemID {
					currentIdx = i
					break
				}
			}
		}
	}
	if currentIdx < 0 || currentIdx >= len(a.items) {
		if dy < 0 || dx < 0 {
			a.chooseItem(len(a.items)-1, 0)
		} else {
			a.chooseItem(0, 0)
		}
		return
	}

	mode := a.prefs.LayoutMode
	if mode == "list" {
		next := currentIdx + dy
		if dx != 0 && dy == 0 {
			next = currentIdx + dx
		}
		if next < 0 {
			next = 0
		}
		if next >= len(a.items) {
			next = len(a.items) - 1
		}
		a.chooseItem(next, 0)
		a.galleryList.ScrollIntoView(next)
		return
	}

	// Horizontal move in grid / justified
	if dx != 0 && dy == 0 {
		next := currentIdx + dx
		if next < 0 {
			next = 0
		}
		if next >= len(a.items) {
			next = len(a.items) - 1
		}
		a.chooseItem(next, 0)
		for rIdx, r := range a.rows {
			for _, c := range r {
				if c.index == next {
					a.galleryList.ScrollIntoView(rIdx)
					return
				}
			}
		}
		return
	}

	// Vertical move in 2D rows (grid / justified)
	// Find which row and column currently holds currentIdx
	curRow := -1
	curCol := -1
	curCenterX := float32(0)
	for rIdx, r := range a.rows {
		curX := float32(gallerySidePadding)
		for cIdx, c := range r {
			w := float32(c.width)
			if c.index == currentIdx {
				curRow = rIdx
				curCol = cIdx
				curCenterX = curX + w*0.5
				break
			}
			curX += w + a.galleryGap()
		}
		if curRow >= 0 {
			break
		}
	}

	if curRow < 0 {
		next := currentIdx + dy
		if next >= 0 && next < len(a.items) {
			a.chooseItem(next, 0)
		}
		return
	}

	targetRow := curRow + dy
	if targetRow < 0 {
		targetRow = 0
	}
	if targetRow >= len(a.rows) {
		targetRow = len(a.rows) - 1
	}
	if targetRow == curRow {
		return
	}

	// For fixed grid, try same column index first
	targetCells := a.rows[targetRow]
	if len(targetCells) == 0 {
		return
	}
	if mode == "grid" && curCol >= 0 {
		cIdx := curCol
		if cIdx >= len(targetCells) {
			cIdx = len(targetCells) - 1
		}
		bestIdx := targetCells[cIdx].index
		a.chooseItem(bestIdx, 0)
		a.galleryList.ScrollIntoView(targetRow)
		return
	}

	// For justified layout, find cell in target row whose horizontal center is closest to curCenterX
	bestIdx := -1
	bestDist := float32(1e9)
	curX := float32(gallerySidePadding)
	for _, c := range targetCells {
		w := float32(c.width)
		centerX := curX + w*0.5
		dist := centerX - curCenterX
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			bestIdx = c.index
		}
		curX += w + 8
	}

	if bestIdx >= 0 {
		a.chooseItem(bestIdx, 0)
		a.galleryList.ScrollIntoView(targetRow)
	}
}
func (a *App) selectedItems() []core.Item {
	out := make([]core.Item, 0, len(a.selected))
	for _, it := range a.items {
		if a.selected[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

func (a *App) selectedIDs() []string {
	out := make([]string, 0, len(a.selected))
	for _, it := range a.items {
		if a.selected[it.ID] {
			out = append(out, it.ID)
		}
	}
	return out
}

// toggleSidebar shows or hides the sidebar and relayouts the gallery.
func (a *App) toggleSidebar() {
	a.prefs.SidebarOpen = !a.prefs.SidebarOpen
	a.savePrefs()
	a.rebuildRows()
	if a.win != nil {
		a.win.Invalidate()
	}
}

// viewContextMenu appends the workspace view controls shared by the empty
// gallery and item context menus.
func (a *App) viewContextMenu(m *ui.Menu) {
	// Creating a subfolder only makes sense inside a real folder: the fixed
	// smart views (all, untagged, uncategorized, tags, trash) have no folder
	// to nest it under.
	if a.nav == NavFolder && a.folderID != "" {
		if m.Item(a.t("contextMenu.newSubfolder")).Chosen() {
			a.dialog = dialogNewFolder
			a.dialogValue = ""
			a.newFolderParentID = a.folderID
		}
		m.Separator()
	}
	if m.Item(a.t("contextMenu.showSidebar")).Checked(a.prefs.SidebarOpen).Chosen() {
		a.toggleSidebar()
	}
	if m.Item(a.t("workspace.includeSubfolders")).Checked(a.prefs.IncludeSubs).Chosen() {
		a.prefs.IncludeSubs = !a.prefs.IncludeSubs
		a.savePrefs()
		if a.nav == NavFolder && a.folderID != "" {
			a.refreshWorkspace()
		}
	}
	if m.Item(a.t("contextMenu.showInspector")).Checked(a.prefs.InspectorOpen).Chosen() {
		a.prefs.InspectorOpen = !a.prefs.InspectorOpen
		a.savePrefs()
	}
	m.Submenu(a.t("contextMenu.layout"), func(sub *ui.Menu) {
		if sub.Item(a.t("contextMenu.layoutJustified")).Radio(a.prefs.LayoutMode == "" || a.prefs.LayoutMode == "justified").Chosen() {
			a.prefs.LayoutMode = "justified"
			a.savePrefs()
			a.rebuildRows()
		}
		if sub.Item(a.t("contextMenu.layoutGrid")).Radio(a.prefs.LayoutMode == "grid").Chosen() {
			a.prefs.LayoutMode = "grid"
			a.savePrefs()
			a.rebuildRows()
		}
		if sub.Item(a.t("contextMenu.layoutList")).Radio(a.prefs.LayoutMode == "list").Chosen() {
			a.prefs.LayoutMode = "list"
			a.savePrefs()
			a.rebuildRows()
		}
	})
	m.Submenu(a.t("contextMenu.thumbnail"), func(sub *ui.Menu) {
		for _, r := range []int{120, 190, 280, 360} {
			r := r
			label := fmt.Sprintf("%d px", r)
			if sub.Item(label).Radio(a.rowHeight == r).Chosen() {
				a.setRowHeight(r)
			}
		}
	})
}

func (a *App) itemContextMenu(m *ui.Menu, item core.Item) {
	if loaded := a.fullWorkspaceItems([]core.Item{item}); len(loaded) > 0 {
		item = loaded[0]
	}
	if a.nav == NavTrash {
		if m.Item(a.t("contextMenu.restore")).Chosen() {
			a.restoreSelected()
		}
		if m.Item(a.t("contextMenu.deletePermanently")).Chosen() {
			a.deleteSelectedPermanently()
		}
		m.Separator()
		a.viewContextMenu(m)
		return
	}
	if m.Item(a.t("contextMenu.fullPreview")).Chosen() {
		a.openPreview(item.ID, "full")
	}
	if mods, key, ok := a.shortcutKeys(core.ShortcutCopy); ok {
		if m.Item(a.t("common.copy")).Shortcut(mods, key).Chosen() {
			a.copySelected()
		}
	}
	if m.Item(a.t("contextMenu.revealInFinder")).Chosen() {
		_ = a.svc.RevealInFinder(item.ID)
	}
	if m.Item(a.t("contextMenu.openWithDefaultApp")).Chosen() {
		_ = a.svc.OpenWithDefaultApp(item.ID)
	}
	if m.Item(a.t("contextMenu.copyFilePath")).Chosen() {
		_ = a.svc.WriteClipboardText(item.FilePath)
		a.showToast(a.t("common.copied"))
	}
	if m.Item(a.t("contextMenu.generateThumbnails")).Chosen() {
		ids := a.selectedIDs()
		if len(ids) == 0 {
			ids = []string{item.ID}
		}
		a.regenerateThumbnails(ids)
	}
	m.Separator()
	selectedIDs := a.selectedIDs()
	if len(selectedIDs) >= 2 {
		if m.Item(a.t("variants.groupSelected")).Chosen() {
			a.groupSelectedAsVariants(item.ID)
		}
	}
	if len(item.Variants) > 0 {
		label := a.t("variants.expand")
		if a.expandedVariantGroups[item.ID] {
			label = a.t("variants.collapse")
		}
		if m.Item(label).Chosen() {
			a.toggleVariantGroupExpanded(item.ID)
		}
	}
	if m.Item(a.t("variants.addFile")).Chosen() {
		a.pickAndAddVariantFile(item.ID)
	}
	if item.VariantOf != "" {
		if m.Item(a.t("variants.setPrimary")).Chosen() {
			a.setPrimaryVariant(item.VariantOf, item.ID)
		}
		if m.Item(a.t("variants.unlink")).Chosen() {
			a.unlinkVariant(item.VariantOf, item.ID)
		}
	}
	m.Separator()
	if m.Item(a.t("contextMenu.addToFavorites")).Chosen() {
		for _, id := range a.selectedIDs() {
			_, _ = a.svc.ToggleFavorite(id)
		}
		a.refreshAll()
	}
	m.Submenu(a.t("contextMenu.addToFolder"), func(sub *ui.Menu) {
		for _, f := range a.folderIndex().all {
			id := f.ID
			if sub.Item(f.Name).Chosen() {
				for _, itemID := range a.selectedIDs() {
					_ = a.svc.AddItemToFolder(itemID, id)
				}
				a.refreshAll()
			}
		}
	})
	m.Separator()
	a.viewContextMenu(m)
	actions := a.pluginHost.Actions(pluginContext(a.fullWorkspaceItems(a.selectedItems()), a.folderID))
	if len(actions) > 0 {
		m.Separator()
		for _, action := range actions {
			action := action
			if m.Item(action.Title).Chosen() {
				context := pluginContext(a.fullWorkspaceItems(a.selectedItems()), a.folderID)
				go func() {
					err := a.pluginHost.RunAction(action.ID, context)
					a.post(func() {
						if err != nil {
							a.showToast(err.Error())
						}
						a.refreshAll()
					})
				}()
			}
		}
	}
	m.Separator()
	if m.Item(a.deleteActionLabel()).Chosen() {
		a.deleteSelection()
	}
}

func pluginContext(items []core.Item, folder string) plugin.ItemContext {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id": it.ID, "name": it.Name, "filename": it.Filename, "filePath": it.FilePath,
			"itemPath": it.ItemPath, "fileUrl": it.FileURL, "mimeType": it.MimeType,
			"extension": it.Extension, "size": it.Size, "width": it.Width, "height": it.Height,
			"url": it.URL, "originalUrl": it.OriginalURL, "thumbnailUrl": it.ThumbnailURL,
			"downloadUrl": it.DownloadURL, "dominantColor": it.DominantColor,
			"palette": it.Palette, "meta": it.Meta, "hasThumbnail": it.HasThumbnail,
			"note": it.Note, "prompt": it.Prompt, "rating": it.Rating,
			"tags": it.Tags, "folders": it.Folders, "inTrash": it.InTrash,
			"createdAt": it.CreatedAt, "importedAt": it.ImportedAt, "updatedAt": it.UpdatedAt,
		})
	}
	return plugin.ItemContext{Items: out, Folder: folder}
}

// isTextDocument mirrors the extensions the native preview decodes as text.
func isTextDocument(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "md", "markdown", "mdown", "mkd", "mdwn", "mdt", "txt", "json", "js", "ts", "tsx", "jsx", "go", "py", "css", "html", "xml", "yaml", "yml", "toml", "prompt":
		return true
	}
	return false
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
