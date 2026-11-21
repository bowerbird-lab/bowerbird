//go:build !darwin || ios

package main

import "github.com/egoist/mygo"

func pinDropZoneWindow(w *mygo.Window, bounds mygo.Rectangle, aboveMenuBar bool) {
	if w == nil {
		return
	}
	w.SetBounds(bounds)
}
