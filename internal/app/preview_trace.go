package app

import (
	"fmt"
	"time"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

type previewLoadTrace struct {
	sequence  uint64
	itemID    string
	path      string
	index     int
	mode      string
	started   time.Time
	filmstrip bool
	compact   bool
}

func (trace *previewLoadTrace) record(level, stage string, start time.Time, detail string) {
	if trace == nil {
		return
	}
	elapsed := time.Since(start)
	prefix := "Image preview "
	if trace.filmstrip {
		prefix = "Filmstrip preview "
	}
	core.RecordLog(level, "preview", prefix+stage,
		fmt.Sprintf("request=%d item=%s index=%d mode=%s path=%q elapsed_us=%d %s",
			trace.sequence, trace.itemID, trace.index, trace.mode, trace.path, elapsed.Microseconds(), detail),
		elapsed.Milliseconds())
}

func (trace *previewLoadTrace) recordDecode(stage string, start time.Time, bitmap *ui.Bitmap, err error) {
	if trace == nil {
		return
	}
	if err != nil {
		trace.record("error", stage, start, fmt.Sprintf("error=%q", err))
		return
	}
	if bitmap == nil {
		trace.record("error", stage, start, "decoder returned no bitmap")
		return
	}
	width, height := bitmap.Size()
	trace.record("perf", stage, start, fmt.Sprintf("width=%d height=%d", width, height))
}
