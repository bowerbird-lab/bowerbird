package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// The library identity, edited from the inspector: a reusable tile that shows
// the library's picture or built-in icon, an icon picker, and the generic crop
// dialog (ui_crop.go) for a picture dropped or chosen from disk.

// libraryIconChoice is one built-in picture the icon picker offers; Name is
// what a library manifest stores.
type libraryIconChoice struct {
	Name string
	Icon *ui.SVG
}

// libraryIconSet builds the picker from the bundled Lucide slice once.
var libraryIconSet = func() []libraryIconChoice {
	choices := make([]libraryIconChoice, 0, len(lucideLibraryIcons))
	for _, icon := range lucideLibraryIcons {
		choices = append(choices, libraryIconChoice{Name: icon.Name, Icon: strokeIcon(icon.Body)})
	}
	return choices
}()

// libraryIconAliases maps icon names older builds persisted to the Lucide icon
// that replaced them, so an existing library keeps a recognizable symbol.
var libraryIconAliases = map[string]string{
	"audio":    "music",
	"document": "file-text",
	"prompt":   "terminal",
}

// resolvedLibraryIconName follows an alias to the icon actually drawn.
func resolvedLibraryIconName(name string) string {
	if alias, ok := libraryIconAliases[name]; ok {
		return alias
	}
	return name
}

// libraryIconName is the built-in icon a library shows: its picked one, or the
// default when it picked none or uses a custom picture.
func libraryIconName(lib *core.LibraryInfo) string {
	if lib == nil || lib.Icon == "" || lib.Icon == core.LibraryIconCustom {
		return "library"
	}
	return lib.Icon
}

func libraryIconByName(name string) *ui.SVG {
	name = resolvedLibraryIconName(name)
	for _, choice := range libraryIconSet {
		if choice.Name == name {
			return choice.Icon
		}
	}
	return iconLibrary
}

// invalidateLibraryIcon drops the decoded custom icon of a library folder so
// the next frame reads the picture just saved.
func (a *App) invalidateLibraryIcon(libPath string) {
	if a.libIcons == nil || libPath == "" {
		return
	}
	delete(a.libIcons, filepath.Join(libPath, core.LibraryIconFile))
}

// libraryIconBitmap returns the library's custom icon, decoded once per folder.
// A nil entry caches a missing or unreadable picture.
func (a *App) libraryIconBitmap(lib *core.LibraryInfo) *ui.Bitmap {
	if lib == nil || lib.Icon != core.LibraryIconCustom || lib.Path == "" {
		return nil
	}
	path := filepath.Join(lib.Path, core.LibraryIconFile)
	if a.libIcons == nil {
		a.libIcons = map[string]*ui.Bitmap{}
	}
	if bmp, ok := a.libIcons[path]; ok {
		return bmp
	}
	data, err := os.ReadFile(path)
	if err != nil {
		a.libIcons[path] = nil
		return nil
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		a.libIcons[path] = nil
		return nil
	}
	a.libIcons[path] = bmp
	return bmp
}

// libraryAvatar draws the reusable library tile: a rounded surface holding the
// library's custom picture, or its built-in icon. Rows, the sidebar switcher,
// the picker and the inspector header all use it, so a custom icon follows the
// library everywhere.
func (a *App) libraryAvatar(c *ui.Context, lib *core.LibraryInfo, size float32) ui.Element {
	p := a.palette(c)
	box := ui.Box(c).Size(size, size).Radius(size * 0.24).Background(p.surface2).Clip()
	if bmp := a.libraryIconBitmap(lib); bmp != nil {
		box.Children(func() {
			ui.Image(c, bmp).Fill().Fit(ui.Cover)
		})
		return box
	}
	// The icon keeps the same size it had without the tile, so the background
	// only pads it; the picture itself fills the tile.
	glyph := size * 0.55
	box.Center()
	box.Children(func() {
		ui.Icon(c, libraryIconByName(libraryIconName(lib))).Size(glyph, glyph).TextColor(p.accent)
	})
	return box
}

// libraryIconTrigger draws the inspector's editable tile: pressing it opens the
// icon picker, and a picture dropped on the tile itself starts the cropper. The
// tile is the whole drop target, so hovering a drag only outlines it instead of
// inserting a hint row that moves the panel.
func (a *App) libraryIconTrigger(c *ui.Context, lib *core.LibraryInfo) {
	p := a.palette(c)
	trigger := ui.ButtonBase(c.Key("library-icon")).Size(42, 42).Radius(10).Padding(0).FocusRing(false).
		Cursor(ui.CursorPointer).Tooltip(a.t("inspector.changeIcon"))
	over := a.libraryIconDropTarget(trigger)
	trigger.Children(func() {
		avatar := a.libraryAvatar(c, lib, 42)
		switch {
		case over:
			avatar.Border(2, p.accent)
		case trigger.Hovered() || a.libIconPickerOpen:
			avatar.Border(1.5, p.accent)
		}
	})
	if trigger.Clicked() {
		a.libIconPickerOpen = !a.libIconPickerOpen
		a.libNameEdit = false
	}
	ui.PopoverBase(c, trigger, &a.libIconPickerOpen, func(panel ui.Element) {
		panel.Padding(10).Radius(12).Background(p.surface).Border(1, p.hairline).
			Shadow(0, 18, 44, -14, p.cardShade)
		panel.Children(func() {
			a.viewLibraryIconPicker(c)
		})
	})
}

// The picker shows its icons as a grid of short columns and scrolls sideways:
// the many icons of the pack stay in one row's height instead of a tall list.
const (
	libIconCell = float32(44)
	libIconGap  = float32(6)
	libIconRows = 3
)

// viewLibraryIconPicker is the picker's content: the built-in set, a file
// chooser, and a reset.
func (a *App) viewLibraryIconPicker(c *ui.Context) {
	p := a.palette(c)
	lib := a.library
	if lib == nil {
		return
	}
	custom := lib.Icon == core.LibraryIconCustom
	current := resolvedLibraryIconName(libraryIconName(lib))
	cols := (len(libraryIconSet) + libIconRows - 1) / libIconRows
	gridHeight := float32(libIconRows)*libIconCell + float32(libIconRows-1)*libIconGap
	ui.Column(c).Width(222).Gap(8).Children(func() {
		grid := ui.ScrollHorizontal(c).FillWidth().Height(gridHeight).TrackScroll(&a.libIconScroll)
		// A mouse wheel has no sideways axis: let it walk the strip, so the
		// whole pack stays reachable without a trackpad.
		grid.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputScroll || ev.DX != 0 || ev.DY == 0 {
				return false
			}
			a.libIconScroll.X = minF(maxF(a.libIconScroll.X+ev.DY, 0), a.libIconScroll.MaxX)
			return true
		})
		grid.Children(func() {
			ui.Column(c).Gap(libIconGap).Children(func() {
				for row := 0; row < libIconRows; row++ {
					row := row
					ui.Row(c).Gap(libIconGap).Children(func() {
						for col := 0; col < cols; col++ {
							i := row*cols + col
							if i >= len(libraryIconSet) {
								break
							}
							a.libraryIconChoice(c, p, libraryIconSet[i], !custom && libraryIconSet[i].Name == current)
						}
					})
				}
			})
		})
		ui.Box(c).FillWidth().Height(1).Background(p.hairline)
		a.libraryIconPickerAction(c, "lib-icon-file", iconUpload, a.t("inspector.iconChooseFile"), a.chooseLibraryIconFile)
		if lib.Icon != "" {
			a.libraryIconPickerAction(c, "lib-icon-reset", iconRestore, a.t("inspector.iconReset"), func() {
				a.chooseLibraryIcon("")
			})
		}
	})
}

// libraryIconChoice is one square of the picker's grid.
func (a *App) libraryIconChoice(c *ui.Context, p palette, choice libraryIconChoice, selected bool) {
	b := ui.ButtonBase(c.Key("lib-icon-"+choice.Name)).Size(libIconCell, libIconCell).Radius(9).
		FocusRing(false).AlignItems(ui.Center).Justify(ui.Center).Label(humanIconName(choice.Name))
	bg := ui.Transparent
	if selected {
		bg = p.press
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	if selected {
		b.Border(1, p.accent)
	} else {
		b.Border(1, p.hairline)
	}
	b.Children(func() {
		color := p.muted
		if selected {
			color = p.accent
		}
		ui.Icon(c, choice.Icon).Size(20, 20).TextColor(color)
	})
	if b.Clicked() {
		a.chooseLibraryIcon(choice.Name)
	}
}

// humanIconName turns "folder-open" into "Folder open" for assistive
// technology, which reads the icons' names but shows no tooltip.
func humanIconName(name string) string {
	name = strings.ReplaceAll(name, "-", " ")
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func (a *App) libraryIconPickerAction(c *ui.Context, key string, icon *ui.SVG, label string, onClick func()) {
	p := a.palette(c)
	row := ui.ButtonBase(c.Key(key)).FillWidth().Height(30).PaddingX(6).Radius(7).FocusRing(false).
		AlignItems(ui.Center).Gap(8)
	bg := ui.Transparent
	if row.Hovered() {
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	row.Children(func() {
		ui.Icon(c, icon).Size(14, 14).TextColor(p.muted)
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1).Shrink(1).SingleLine()
	})
	if row.Clicked() {
		a.libIconPickerOpen = false
		onClick()
	}
}

// chooseLibraryIcon stores a built-in icon, or the default when name is empty.
func (a *App) chooseLibraryIcon(name string) {
	a.libIconPickerOpen = false
	if a.svc == nil || a.library == nil {
		return
	}
	if a.library.Icon == name {
		return
	}
	libName := a.library.Name
	go func() {
		info, err := a.svc.UpdateLibraryDetails(libName, name)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			if info != nil {
				a.library = info
			}
			a.refreshAll()
		})
	}()
}

// beginLibraryRename opens the inline editor over the library name.
func (a *App) beginLibraryRename() {
	if a.library == nil {
		return
	}
	a.libNameValue = a.library.Name
	a.libNameEdit = true
	a.libIconPickerOpen = false
}

// commitLibraryRename saves the inline name editor when it holds a new name.
func (a *App) commitLibraryRename() {
	if !a.libNameEdit {
		return
	}
	a.libNameEdit = false
	name := strings.TrimSpace(a.libNameValue)
	if a.svc == nil || a.library == nil || name == "" || name == a.library.Name {
		return
	}
	icon := a.library.Icon
	go func() {
		info, err := a.svc.UpdateLibraryDetails(name, icon)
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			if info != nil {
				a.library = info
			}
			a.refreshAll()
		})
	}()
}

// libraryIconDropTarget makes el take a picture dragged from another app or a
// gallery card, and reports whether one is over it, for showing it would.
func (a *App) libraryIconDropTarget(zone ui.Element) bool {
	opts := transfer.DropOptions{
		Formats:    []transfer.Format{transfer.FileList},
		Operations: transfer.Copy,
	}
	_, over := ui.DataDragOver(zone, opts)
	if drop, ok := ui.DropData(zone, opts); ok {
		if files, err := drop.Data.Files(); err == nil && len(files) > 0 {
			a.beginLibraryIconCropFromDrop(files)
		}
	}
	// Pictures dragged from Finder/Explorer come as native file drops.
	if files := zone.DroppedFiles(); len(files) > 0 {
		a.beginLibraryIconCropFromDrop(files)
	}
	return over || zone.FileDragOver()
}

func (a *App) beginLibraryIconCropFromDrop(paths []string) {
	path := firstImagePath(paths)
	if path == "" {
		a.showToast(a.t("inspector.iconUnsupported"))
		return
	}
	a.beginLibraryIconCrop(path)
}

// firstImagePath picks the first path with an image extension, and still tries
// a lone path without one so an oddly named file is not rejected silently.
func firstImagePath(paths []string) string {
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
			return path
		}
	}
	if len(paths) == 1 {
		return paths[0]
	}
	return ""
}

// chooseLibraryIconFile asks for a picture on disk, then crops it.
func (a *App) chooseLibraryIconFile() {
	a.libIconPickerOpen = false
	title := a.t("inspector.iconChooseFile")
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: title,
			Filters: []mygo.FileFilter{{
				Name:       "Images",
				Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.beginLibraryIconCrop(paths[0])
	}()
}

// beginLibraryIconCrop decodes a picture off the main thread and opens the
// reusable cropper on it with the library-icon copy and save action.
func (a *App) beginLibraryIconCrop(path string) {
	path = strings.TrimSpace(path)
	if path == "" || a.svc == nil || a.libIconBusy {
		return
	}
	a.libIconPickerOpen = false
	a.libNameEdit = false
	a.libIconBusy = true
	go func() {
		img, err := decodeCropSource(path, cropMaxSource)
		if err != nil {
			a.post(func() {
				a.libIconBusy = false
				a.showToast(a.t("inspector.iconUnsupported"))
			})
			return
		}
		a.post(func() {
			a.libIconBusy = false
			dialog := NewCropDialog(img)
			dialog.Title = a.t("inspector.cropTitle")
			dialog.Hint = a.t("inspector.cropHint")
			dialog.ZoomLabel = a.t("inspector.cropZoom")
			dialog.ApplyLabel = a.t("inspector.cropApply")
			dialog.CancelLabel = a.t("common.cancel")
			dialog.OnApply = a.saveLibraryIcon
			a.crop = dialog
		})
	}()
}

// saveLibraryIcon stores a cropped picture as the active library's custom icon
// and refreshes every view that shows the library.
func (a *App) saveLibraryIcon(icon image.Image) {
	if a.svc == nil || icon == nil {
		return
	}
	go func() {
		var buf bytes.Buffer
		if err := png.Encode(&buf, icon); err != nil {
			a.post(func() { a.showToast(err.Error()) })
			return
		}
		info, err := a.svc.SaveLibraryIcon(buf.Bytes())
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				return
			}
			if info != nil {
				a.library = info
				a.invalidateLibraryIcon(info.Path)
			}
			a.refreshAll()
			a.showToast(a.t("inspector.iconUpdated"))
		})
	}()
}
