//go:build darwin && !ios

package main

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type nsPoint struct{ X, Y float64 }
type nsSize struct{ W, H float64 }
type nsRect struct {
	Origin nsPoint
	Size   nsSize
}

var (
	dropPanelHookOnce sync.Once
	dropZoneAboveWins sync.Map // map[uintptr]bool
	origPanelSetLevel uintptr
)

func ensurePanelLevelHook(win objc.ID) {
	dropPanelHookOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		var objectGetClass func(objc.ID) objc.Class
		var classGetInstanceMethod func(objc.Class, objc.SEL) uintptr
		var methodSetImplementation func(uintptr, uintptr) uintptr
		purego.RegisterLibFunc(&objectGetClass, lib, "object_getClass")
		purego.RegisterLibFunc(&classGetInstanceMethod, lib, "class_getInstanceMethod")
		purego.RegisterLibFunc(&methodSetImplementation, lib, "method_setImplementation")

		cls := objectGetClass(win)
		sel := objc.RegisterName("setLevel:")
		m := classGetInstanceMethod(cls, sel)
		if m == 0 {
			return
		}
		cb := purego.NewCallback(func(self objc.ID, _cmd objc.SEL, level int64) {
			if _, ok := dropZoneAboveWins.Load(uintptr(self)); ok {
				level = 24 + 3 // NSMainMenuWindowLevel + 3
			}
			if origPanelSetLevel != 0 {
				purego.SyscallN(origPanelSetLevel, uintptr(self), uintptr(_cmd), uintptr(level))
			}
		})
		origPanelSetLevel = methodSetImplementation(m, cb)
	})
}

func pinDropZoneWindow(w *mygo.Window, bounds mygo.Rectangle, aboveMenuBar bool) {
	if w == nil {
		return
	}
	handle := w.NativeHandle()
	if handle == 0 {
		return
	}
	win := objc.ID(handle)
	ensurePanelLevelHook(win)

	if aboveMenuBar {
		dropZoneAboveWins.Store(handle, true)
		// MyGo creates windows with NSWindowStyleMaskTitled + hidden titlebar,
		// which AppKit clamps below the menu bar. Switching to borderless
		// nonactivatingPanel | fullSizeContentView lets it sit flush at y=0
		// over the menu bar.
		const nonactivatingPanel, fullSizeContentView = 1 << 7, 1 << 15
		win.Send(objc.RegisterName("setStyleMask:"), uint(nonactivatingPanel|fullSizeContentView))
		win.Send(objc.RegisterName("setLevel:"), int64(24+3))
		win.Send(objc.RegisterName("setCollectionBehavior:"), uint(1|(1<<4)|(1<<8)|(1<<6)))
	} else {
		dropZoneAboveWins.Delete(handle)
		win.Send(objc.RegisterName("setLevel:"), int64(3)) // NSFloatingWindowLevel
	}

	win.Send(objc.RegisterName("setOpaque:"), false)
	clearColor := objc.ID(objc.GetClass("NSColor")).Send(objc.RegisterName("clearColor"))
	win.Send(objc.RegisterName("setBackgroundColor:"), clearColor)

	screens := objc.ID(objc.GetClass("NSScreen")).Send(objc.RegisterName("screens"))
	if objc.Send[uint](screens, objc.RegisterName("count")) > 0 {
		primary := screens.Send(objc.RegisterName("objectAtIndex:"), uint(0))
		pf := objc.Send[nsRect](primary, objc.RegisterName("frame"))
		r := nsRect{
			Origin: nsPoint{float64(bounds.X), pf.Size.H - float64(bounds.Y) - float64(bounds.Height)},
			Size:   nsSize{float64(bounds.Width), float64(bounds.Height)},
		}
		win.Send(objc.RegisterName("setFrame:display:"), r, true)
	}
}
