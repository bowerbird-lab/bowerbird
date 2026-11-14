package app

import (
	"fmt"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

type filmstripLoadState struct {
	sequence uint64
	start    int
	end      int
	started  time.Time
	logged   bool
	warned   bool
	traces   map[string]*previewLoadTrace
}

func (a *App) beginFilmstripLoad(start, end int) {
	state := a.filmstripTrace
	if state != nil && state.sequence == a.previewSeq && state.start == start && state.end == end {
		return
	}
	state = &filmstripLoadState{
		sequence: a.previewSeq, start: start, end: end, started: time.Now(),
		traces: make(map[string]*previewLoadTrace, end-start),
	}
	a.filmstripTrace = state
	if trace := a.previewTrace; trace != nil {
		trace.record("info", "filmstrip_requested", state.started, fmt.Sprintf("range_start=%d range_end=%d", start, end))
	}
}

func (a *App) filmstripBitmap(item core.Item, index int, path string) (*ui.Bitmap, error) {
	var trace *previewLoadTrace
	if state := a.filmstripTrace; state != nil && a.previewTrace != nil {
		trace = state.traces[path]
		if trace == nil {
			trace = &previewLoadTrace{
				sequence: a.previewSeq, itemID: item.ID, index: index, path: path,
				mode: a.previewMode, started: state.started, filmstrip: true, compact: true,
			}
			state.traces[path] = trace
		}
	}
	return a.filmstripImages.getResult(path, 120, trace)
}

func (a *App) recordFilmstripLoad(c *ui.Context, ready, waiting, failed, missing int) {
	state, trace := a.filmstripTrace, a.previewTrace
	if state == nil || trace == nil || state.logged {
		return
	}
	if waiting == 0 && missing == 0 {
		state.logged = true
		level := "info"
		if failed > 0 {
			level = "warn"
		}
		trace.record(level, "filmstrip_ready", state.started, fmt.Sprintf("range_start=%d range_end=%d ready=%d waiting=%d failed=%d missing_path=%d",
			state.start, state.end, ready, waiting, failed, missing))
		return
	}
	if waiting+missing > 0 && !state.warned {
		elapsed := time.Since(state.started)
		if elapsed < 2*time.Second {
			c.After(2*time.Second - elapsed)
			return
		}
		state.warned = true
		trace.record("warn", "filmstrip_waiting", state.started, fmt.Sprintf("range_start=%d range_end=%d ready=%d waiting=%d failed=%d missing_path=%d",
			state.start, state.end, ready, waiting, failed, missing))
	}
}
