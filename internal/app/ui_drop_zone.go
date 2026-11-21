package app

import (
	"fmt"
	"log"
	"strings"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	dropZoneNotchIdleW   = 186
	dropZoneNotchIdleH   = 30
	dropZoneNotchExpW    = 260
	dropZoneNotchExpH    = 54
	dropZoneNotchWidth   = 304
	dropZoneNotchHeight  = 68

	dropZoneHWNotchIdleW  = 218
	dropZoneHWNotchIdleH  = 50
	dropZoneHWNotchExpW   = 276
	dropZoneHWNotchExpH   = 72
	dropZoneHWNotchWidth  = 320
	dropZoneHWNotchHeight = 84

	dropZoneCapsuleIdleW  = 182
	dropZoneCapsuleIdleH  = 32
	dropZoneCapsuleExpW   = 254
	dropZoneCapsuleExpH   = 52
	dropZoneCapsuleWidth  = 292
	dropZoneCapsuleHeight = 64

	dropZoneCapsuleTopGap  = 0
	dropZoneFeedbackWindow = 750 * time.Millisecond
)

// notchShape returns a single continuous path for the top-of-screen notch:
// top-left concave flare -> straight left edge -> rounded bottom -> straight
// right edge -> top-right concave flare.
func notchShape(cx, top, w, h, flare, foot float32) *ui.Path {
	const k = 0.5523
	l, r := cx-w/2, cx+w/2
	p := &ui.Path{}
	p.MoveTo(l-flare, top)
	p.CubeTo(l-flare+flare*k, top, l, top+flare-flare*k, l, top+flare)
	p.LineTo(l, top+h-foot)
	p.CubeTo(l, top+h-foot+foot*k, l+foot-foot*k, top+h, l+foot, top+h)
	p.LineTo(r-foot, top+h)
	p.CubeTo(r-foot+foot*k, top+h, r, top+h-foot+foot*k, r, top+h-foot)
	p.LineTo(r, top+flare)
	p.CubeTo(r, top+flare-flare*k, r+flare-flare*k, top, r+flare, top)
	p.Close()
	return p
}

type dropZoneFeedbackState struct {
	text  string
	isErr bool
	until time.Time
	seq   uint64
}

// HasHardwareNotch reports whether d appears to have a macOS hardware notch
// (internal display with a tall menu-bar inset).
func HasHardwareNotch(d mygo.Display) bool {
	return d.Internal && (d.WorkArea.Y-d.Bounds.Y) >= 32
}

// ResolveDropZoneDisplays filters connected displays according to settings.
func ResolveDropZoneDisplays(displays []mygo.Display, settings core.DropZoneSettings) []mygo.Display {
	if !settings.Enabled || len(displays) == 0 {
		return nil
	}
	switch settings.ScreenMode {
	case core.DropZoneScreenAll:
		out := make([]mygo.Display, len(displays))
		copy(out, displays)
		return out
	case core.DropZoneScreenCustom:
		if len(settings.ScreenIDs) == 0 {
			return nil
		}
		allowed := make(map[int64]struct{}, len(settings.ScreenIDs))
		for _, id := range settings.ScreenIDs {
			allowed[id] = struct{}{}
		}
		var out []mygo.Display
		for _, d := range displays {
			if _, ok := allowed[d.ID]; ok {
				out = append(out, d)
			}
		}
		return out
	default:
		for _, d := range displays {
			if d.Primary {
				return []mygo.Display{d}
			}
		}
		return []mygo.Display{displays[0]}
	}
}

// DropZoneWindowBounds computes the top-center window bounds on display d for
// the given style ("notch" or "capsule").
func DropZoneWindowBounds(d mygo.Display, style string) mygo.Rectangle {
	if style == core.DropZoneStyleCapsule {
		w := dropZoneCapsuleWidth
		h := dropZoneCapsuleHeight
		x := d.Bounds.X + (d.Bounds.Width-w)/2
		top := d.WorkArea.Y
		if top < d.Bounds.Y {
			top = d.Bounds.Y
		}
		y := top + dropZoneCapsuleTopGap
		return mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
	}

	w := dropZoneNotchWidth
	h := dropZoneNotchHeight
	if HasHardwareNotch(d) {
		w = dropZoneHWNotchWidth
		h = dropZoneHWNotchHeight
	}
	x := d.Bounds.X + (d.Bounds.Width-w)/2
	y := d.Bounds.Y
	return mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
}

func (a *App) displays() []mygo.Display {
	if a.displaysProvider != nil {
		return a.displaysProvider()
	}
	return mygo.Screen.Displays()
}

// SetDisplaysProvider overrides display discovery for tests.
func (a *App) SetDisplaysProvider(fn func() []mygo.Display) {
	a.displaysProvider = fn
}

// SetDropZoneCallbacks wires host actions triggered from the floating drop zone.
func (a *App) SetDropZoneCallbacks(openApp, openSettings func()) {
	a.dropZoneOpenApp = openApp
	a.dropZoneOpenSettings = openSettings
}

// RegisterDropZoneWindow tracks an active floating drop-zone window so state
// updates can trigger repaints on it.
func (a *App) RegisterDropZoneWindow(displayID int64, w *mygo.Window) {
	if a.dropZoneWins == nil {
		a.dropZoneWins = map[int64]*mygo.Window{}
	}
	if w == nil {
		delete(a.dropZoneWins, displayID)
		return
	}
	a.dropZoneWins[displayID] = w
}

// UnregisterDropZoneWindow removes a closed drop-zone window from tracking.
func (a *App) UnregisterDropZoneWindow(displayID int64) {
	delete(a.dropZoneWins, displayID)
}

func (a *App) invalidateDropZones() {
	for _, w := range a.dropZoneWins {
		if w != nil {
			w.Update(func() {})
		}
	}
}

func (a *App) setDropZoneFeedback(displayID int64, text string, isErr bool, autoClear bool) {
	if a.dropZoneFeedback == nil {
		a.dropZoneFeedback = map[int64]*dropZoneFeedbackState{}
	}
	st := a.dropZoneFeedback[displayID]
	if st == nil {
		st = &dropZoneFeedbackState{}
		a.dropZoneFeedback[displayID] = st
	}
	st.seq++
	seq := st.seq
	st.text = text
	st.isErr = isErr
	if autoClear {
		st.until = time.Now().Add(dropZoneFeedbackWindow)
	} else {
		st.until = time.Now().Add(30 * time.Second)
	}
	a.invalidateDropZone(displayID)

	if autoClear {
		time.AfterFunc(dropZoneFeedbackWindow, func() {
			a.postDropZone(displayID, func() {
				cur := a.dropZoneFeedback[displayID]
				if cur != nil && cur.seq == seq {
					cur.text = ""
					cur.isErr = false
				}
			})
		})
	}
}

func (a *App) clearDropZoneFeedback(displayID int64) {
	if a.dropZoneFeedback == nil {
		return
	}
	if st := a.dropZoneFeedback[displayID]; st != nil {
		st.seq++
		st.text = ""
		st.isErr = false
		st.until = time.Time{}
	}
	a.invalidateDropZone(displayID)
}

func (a *App) invalidateDropZone(displayID int64) {
	if w := a.dropZoneWins[displayID]; w != nil {
		w.Update(func() {})
	}
}

func (a *App) postDropZone(displayID int64, fn func()) {
	if fn == nil {
		return
	}
	if w := a.dropZoneWins[displayID]; w != nil {
		w.Update(fn)
		return
	}
	a.post(fn)
}

// HandleDropZoneDrop imports files dropped onto the floating top drop zone
// directly into the active library and shows brief feedback on that display.
func (a *App) HandleDropZoneDrop(displayID int64, paths []string) {
	log.Printf("[DropZone] HandleDropZoneDrop display=%d paths=%d %v", displayID, len(paths), paths)
	if len(paths) == 0 || a.svc == nil {
		log.Printf("[DropZone] HandleDropZoneDrop ignored: len=%d svcNil=%v", len(paths), a.svc == nil)
		return
	}
	if lib, err := a.svc.GetActiveLibrary(); err != nil || lib == nil {
		log.Printf("[DropZone] HandleDropZoneDrop no active library: err=%v lib=%v", err, lib)
		a.setDropZoneFeedback(displayID, a.t("dropZone.noLibrary"), true, true)
		return
	}

	copied := append([]string(nil), paths...)
	a.setDropZoneFeedback(displayID, a.tf("dropZone.importing", map[string]any{"count": len(copied)}), false, false)

	go func() {
		res, err := a.svc.HandleNativeDropWithResult(copied, "", "")
		log.Printf("[DropZone] HandleNativeDropWithResult display=%d res=%+v err=%v", displayID, res, err)
		a.postDropZone(displayID, func() {
			if err != nil {
				a.setDropZoneFeedback(displayID, err.Error(), true, true)
				return
			}
			if res.External > 0 {
				a.setDropZoneFeedback(displayID, a.tf("dropZone.saved", map[string]any{"count": res.External}), false, true)
				return
			}
			a.clearDropZoneFeedback(displayID)
		})
	}()
}

func (a *App) currentDropZoneSettings() core.DropZoneSettings {
	if a.svc != nil {
		return a.svc.GetDropZoneSettings()
	}
	return a.dropZone
}

func (a *App) setDropZoneSettings(next core.DropZoneSettings) {
	norm := core.NormalizeDropZoneSettings(next)
	a.dropZone = norm
	if a.svc != nil {
		if err := a.svc.SetDropZoneSettings(norm); err != nil {
			a.showToast(err.Error())
		}
	}
}

// DropZoneContent builds the native view for one floating top drop-zone window.
func (a *App) DropZoneContent(displayID int64, hasNotch bool) *ui.Content {
	return a.content(fmt.Sprintf("drop-zone-%d", displayID), func(c *ui.Context) {
		a.dropZoneView(c, displayID, hasNotch)
	})
}

func (a *App) dropZoneView(c *ui.Context, displayID int64, hasNotch bool) {
		c.Root().Background(ui.Transparent)
		p := a.palette(c)
		settings := a.currentDropZoneSettings()
		isCapsule := settings.Style == core.DropZoneStyleCapsule

		var feedbackText string
		var feedbackErr bool
		if st := a.dropZoneFeedback[displayID]; st != nil && st.text != "" && time.Now().Before(st.until) {
			feedbackText = st.text
			feedbackErr = st.isErr
		}

		label := a.t("dropZone.idle")
		if lib := a.activeLibraryName(); lib != "" {
			label = lib
		}

		stage := ui.Box(c).Fill().Background(ui.Transparent)

		bg := ui.RGBA(12, 13, 17, 245)
		fg := ui.RGBA(244, 245, 248, 242)
		if a.dropZoneHovered[displayID] {
			bg = ui.RGBA(22, 24, 31, 248)
		}
		if a.dropZoneDragOver[displayID] {
			bg = p.accent
			fg = p.onAccent
		}
		if feedbackText != "" {
			if feedbackErr {
				bg = p.danger
				fg = p.onAccent
			} else {
				bg = ui.RGBA(16, 42, 34, 248)
				fg = ui.RGBA(110, 231, 183, 255)
			}
		}
		animBg := stage.AnimateColor("dz-bg", bg, ui.Animation{Duration: 100 * time.Millisecond, Ease: ui.EaseOut})

		expanded := a.dropZoneDragOver[displayID] || feedbackText != ""
		hovered := a.dropZoneHovered[displayID]

		var targetW, targetH, targetR float32
		if isCapsule {
			targetW, targetH, targetR = dropZoneCapsuleIdleW, dropZoneCapsuleIdleH, dropZoneCapsuleIdleH/2
			if expanded {
				targetW, targetH, targetR = dropZoneCapsuleExpW, dropZoneCapsuleExpH, 22
			} else if hovered {
				targetW, targetH, targetR = dropZoneCapsuleIdleW+10, dropZoneCapsuleIdleH+2, (dropZoneCapsuleIdleH+2)/2
			}
		} else if hasNotch {
			targetW, targetH, targetR = dropZoneHWNotchIdleW, dropZoneHWNotchIdleH, 14
			if expanded {
				targetW, targetH, targetR = dropZoneHWNotchExpW, dropZoneHWNotchExpH, 18
			} else if hovered {
				targetW, targetH, targetR = dropZoneHWNotchIdleW+10, dropZoneHWNotchIdleH+2, 15
			}
		} else {
			targetW, targetH, targetR = dropZoneNotchIdleW, dropZoneNotchIdleH, 14
			if expanded {
				targetW, targetH, targetR = dropZoneNotchExpW, dropZoneNotchExpH, 18
			} else if hovered {
				targetW, targetH, targetR = dropZoneNotchIdleW+10, dropZoneNotchIdleH+2, 15
			}
		}

		spring := ui.Animation{Spring: ui.SpringOptions{Duration: 360 * time.Millisecond, Bounce: 0.36}}
		if !expanded && !hovered {
			spring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.24}}
		}
		curW := max(0, stage.AnimateTo("dz-w", targetW, spring))
		curH := max(0, stage.AnimateTo("dz-h", targetH, spring))
		curR := max(0, stage.AnimateTo("dz-r", targetR, spring))
		flare := float32(8)
		if expanded {
			flare = 12
		}
		curFlare := max(0, stage.AnimateTo("dz-flare", flare, spring))
		targetIconSize := float32(14)
		if expanded {
			targetIconSize = 16
		}
		curIconSize := max(10, stage.AnimateTo("dz-icon", targetIconSize, spring))

		winW := float32(dropZoneNotchWidth)
		if isCapsule {
			winW = float32(dropZoneCapsuleWidth)
		} else if hasNotch {
			winW = float32(dropZoneHWNotchWidth)
		}
		cx := winW / 2

		if !isCapsule {
			stage.Draw(func(pt *ui.Painter, r ui.Rect) {
				if curW < 1 || curH < 1 {
					return
				}
				path := notchShape(r.X+r.W/2, r.Y, curW, curH, curFlare, curR)
				pt.FillPath(path, animBg)
			})
		}

		stage.Children(func() {
			zone := ui.Row(c.Key(fmt.Sprintf("drop-zone-%d", displayID))).Absolute().Top(0).Left(cx-curW/2).Width(curW).Height(curH).AlignItems(ui.Center).Gap(7).Cursor(ui.CursorPointer)
			if dropped := zone.DroppedFiles(); len(dropped) > 0 {
				log.Printf("[DropZone] ui.DroppedFiles display=%d files=%v", displayID, dropped)
				a.HandleDropZoneDrop(displayID, dropped)
			}
			if a.dropZoneDragOver == nil {
				a.dropZoneDragOver = map[int64]bool{}
			}
			if a.dropZoneHovered == nil {
				a.dropZoneHovered = map[int64]bool{}
			}
			a.dropZoneDragOver[displayID] = zone.FileDragOver()
			a.dropZoneHovered[displayID] = zone.Hovered()

			if isCapsule {
				zone.Radius(curR).Padding(0, 14, 0, 14).Background(animBg)
			} else {
				zone.Radius(0, 0, curR, curR).Background(ui.Transparent)
				if hasNotch {
					zone.Padding(28, 16, 6, 16)
				} else {
					zone.Padding(0, 16, 0, 16)
				}
			}

			icon := iconInbox
			displayText := label
			if feedbackText != "" {
				displayText = feedbackText
				if !feedbackErr {
					icon = iconCheck
				}
			} else if a.dropZoneDragOver[displayID] {
				displayText = a.t("dropZone.dragOver")
				icon = iconArrowDown
			}

			iconSize := curIconSize
			fontSize := float32(12)
			if expanded {
				fontSize = 13
				zone.Justify(ui.Center)
			} else {
				zone.Justify(ui.SpaceBetween)
			}

			hasLib := a.activeLibraryName() != ""
			zone.Children(func() {
				if expanded || !hasLib {
					ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(fg)
					ui.Text(c, displayText).FontSize(fontSize).FontWeight(550).SingleLine().TextColor(fg)
					return
				}
				left := ui.Row(c).AlignItems(ui.Center).Gap(6).Grow(1).Shrink(1).MinWidth(0)
				left.Children(func() {
					ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(fg)
					ui.Text(c, displayText).FontSize(fontSize).FontWeight(550).SingleLine().TextColor(fg)
				})
				ui.Textf(c, "%d", a.stats.Total).FontSize(11).FontWeight(550).FontFeatures("tnum").TextColor(fg.Alpha(0.72))
			})

			if zone.Clicked() && a.dropZoneOpenApp != nil {
				a.dropZoneOpenApp()
			}

			zone.ContextMenu(func(m *ui.Menu) {
				switchLabel := a.t("dropZone.switchToCapsule")
				nextStyle := core.DropZoneStyleCapsule
				if isCapsule {
					switchLabel = a.t("dropZone.switchToNotch")
					nextStyle = core.DropZoneStyleNotch
				}
				if m.Item(switchLabel).Chosen() {
					cur := a.currentDropZoneSettings()
					cur.Style = nextStyle
					a.setDropZoneSettings(cur)
				}
				m.Separator()
				if a.dropZoneOpenApp != nil && m.Item(a.t("dropZone.openApp")).Chosen() {
					a.dropZoneOpenApp()
				}
				if a.dropZoneOpenSettings != nil && m.Item(a.t("dropZone.openSettings")).Chosen() {
					a.dropZoneOpenSettings()
				}
				m.Separator()
				if m.Item(a.t("dropZone.hide")).Chosen() {
					cur := a.currentDropZoneSettings()
					cur.Enabled = false
					a.setDropZoneSettings(cur)
				}
			})
		})
}

func (a *App) activeLibraryName() string {
	if a.svc == nil {
		return ""
	}
	lib, err := a.svc.GetActiveLibrary()
	if err != nil || lib == nil {
		return ""
	}
	return strings.TrimSpace(lib.Name)
}

func (a *App) settingsDropZoneCard(c *ui.Context) {
	p := a.palette(c)
	dz := a.dropZone
	if dz.Style == "" {
		dz = a.currentDropZoneSettings()
	}

	a.settingsCard(c, a.t("settings.dropZone.card"), func() {
		a.settingsRowInfo(c, a.t("settings.dropZone.enabled"), a.t("settings.dropZone.enabledHelp"), func() {
			on := dz.Enabled
			if ui.Switch(c.Key("drop-zone-enabled"), &on).Changed() {
				dz.Enabled = on
				a.setDropZoneSettings(dz)
			}
		})

		a.settingsRowInfo(c, a.t("settings.dropZone.style"), a.t("settings.dropZone.styleHelp"), func() {
			options := []SelectOption{
				{Value: core.DropZoneStyleNotch, Label: a.t("settings.dropZone.styleNotch")},
				{Value: core.DropZoneStyleCapsule, Label: a.t("settings.dropZone.styleCapsule")},
			}
			if val, changed := a.dropdown(c, "drop-zone-style", dz.Style, options); changed {
				dz.Style = val
				a.setDropZoneSettings(dz)
			}
		})

		a.settingsRowInfo(c, a.t("settings.dropZone.screens"), a.t("settings.dropZone.screensHelp"), func() {
			options := []SelectOption{
				{Value: core.DropZoneScreenPrimary, Label: a.t("settings.dropZone.screensPrimary")},
				{Value: core.DropZoneScreenAll, Label: a.t("settings.dropZone.screensAll")},
				{Value: core.DropZoneScreenCustom, Label: a.t("settings.dropZone.screensCustom")},
			}
			if val, changed := a.dropdown(c, "drop-zone-screens", dz.ScreenMode, options); changed {
				dz.ScreenMode = val
				if val == core.DropZoneScreenCustom && len(dz.ScreenIDs) == 0 {
					for _, d := range a.displays() {
						if d.Primary {
							dz.ScreenIDs = []int64{d.ID}
							break
						}
					}
					if len(dz.ScreenIDs) == 0 {
						if ds := a.displays(); len(ds) > 0 {
							dz.ScreenIDs = []int64{ds[0].ID}
						}
					}
				}
				a.setDropZoneSettings(dz)
			}
		})

		if dz.ScreenMode != core.DropZoneScreenCustom {
			return
		}

		displays := a.displays()
		selected := make(map[int64]bool, len(dz.ScreenIDs))
		for _, id := range dz.ScreenIDs {
			selected[id] = true
		}

		ui.Box(c).FillWidth().Height(1).Background(p.hairline)
		if len(dz.ScreenIDs) == 0 {
			ui.Text(c, a.t("settings.dropZone.screensEmpty")).FontSize(11.5).TextColor(p.warning)
		}

		for i, d := range displays {
			d := d
			name := strings.TrimSpace(d.Label)
			if name == "" {
				name = fmt.Sprintf("Display %d", i+1)
			}
			var badges []string
			if d.Primary {
				badges = append(badges, a.t("settings.dropZone.primaryBadge"))
			}
			if d.Internal {
				badges = append(badges, a.t("settings.dropZone.builtinBadge"))
			}
			meta := fmt.Sprintf("%d × %d", d.Bounds.Width, d.Bounds.Height)
			if len(badges) > 0 {
				meta = strings.Join(badges, " · ") + " · " + meta
			}

			row := ui.Row(c.Key(fmt.Sprintf("dz-display-%d", d.ID))).FillWidth().MinHeight(36).AlignItems(ui.Center).Gap(10)
			row.Children(func() {
				col := ui.Column(c).Grow(1).Shrink(1).Gap(2)
				col.Children(func() {
					ui.Text(c, name).FontSize(12.5).TextColor(p.text)
					ui.Text(c, meta).FontSize(11).TextColor(p.faint)
				})
				on := selected[d.ID]
				if ui.Switch(c.Key(fmt.Sprintf("dz-display-sw-%d", d.ID)), &on).Changed() {
					var nextIDs []int64
					for _, cand := range displays {
						if cand.ID == d.ID {
							if on {
								nextIDs = append(nextIDs, cand.ID)
							}
						} else if selected[cand.ID] {
							nextIDs = append(nextIDs, cand.ID)
						}
					}
					// Preserve any currently-disconnected IDs that were saved.
					for _, savedID := range dz.ScreenIDs {
						if savedID == d.ID {
							continue
						}
						known := false
						for _, cand := range displays {
							if cand.ID == savedID {
								known = true
								break
							}
						}
						if !known {
							nextIDs = append(nextIDs, savedID)
						}
					}
					dz.ScreenIDs = nextIDs
					a.setDropZoneSettings(dz)
				}
			})
		}
	})
}
