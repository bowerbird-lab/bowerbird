package core

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"
)

func prepareLogMemoryTest(t *testing.T) {
	t.Helper()
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	ClearLogsBuffer()
	t.Cleanup(ClearLogsBuffer)
}

func TestLogsClearReleasesBackingReferences(t *testing.T) {
	prepareLogMemoryTest(t)
	RecordLog("info", "core", "message", "detail", 0)
	logMu.RLock()
	backing := logEntries[:cap(logEntries)]
	logMu.RUnlock()
	ClearLogsBuffer()
	for i, entry := range backing {
		if entry != (LogEntry{}) {
			t.Fatalf("cleared slot %d still retains a log: %+v", i, entry)
		}
	}
}

func TestLogsEvictionReleasesUnusedSlots(t *testing.T) {
	prepareLogMemoryTest(t)
	for i := 0; i <= maxBufferedLogs; i++ {
		RecordLog("info", "core", "message", "detail", 0)
	}
	logMu.RLock()
	defer logMu.RUnlock()
	if len(logEntries) > maxBufferedLogs || logEntries[0].ID >= logEntries[len(logEntries)-1].ID {
		t.Fatal("eviction broke the log limit or chronological order")
	}
	for i, entry := range logEntries[len(logEntries):cap(logEntries)] {
		if entry != (LogEntry{}) {
			t.Fatalf("unused slot %d still retains a log", i)
		}
	}
}

func TestLogFieldsAreBoundedBeforeStorageAndEmission(t *testing.T) {
	prepareLogMemoryTest(t)
	host := &logMemoryHost{recordingHost: &recordingHost{}}
	SetLogHost(host)
	t.Cleanup(func() { SetLogHost(nil) })
	input := strings.Repeat("中文🙂", maxLogTextBytes)
	entry := RecordLog("INFO", strings.Repeat("c", 1000), input, input, 5)
	if entry.Level != "info" || len(entry.Category) > maxLogLabelBytes {
		t.Fatal("log labels were not normalized and bounded")
	}
	for _, field := range []string{entry.Message, entry.Detail} {
		if len(field) > maxLogTextBytes || !utf8.ValidString(field) || !strings.HasSuffix(field, "…") {
			t.Fatalf("log text is not bounded valid UTF-8: length=%d", len(field))
		}
	}
	if logs := GetLogsSnapshot(); len(logs) != 1 || logs[0] != entry {
		t.Fatal("stored log differs from bounded return value")
	}
	if host.entry != entry {
		t.Fatal("log event contains unbounded or different fields")
	}
	data, err := os.ReadFile(AppLogFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 2*maxLogTextBytes+maxLogLabelBytes+256 || !utf8.Valid(data) {
		t.Fatal("disk output was not bounded before formatting")
	}
}

type logMemoryHost struct {
	*recordingHost
	entry LogEntry
}

func (h *logMemoryHost) Emit(name string, payload any) {
	if name == LogEntryEvent {
		h.entry = payload.(LogEntry)
	}
}

func TestBoundedLogTextOwnsTrimmedAndSlicedInputs(t *testing.T) {
	for _, input := range []string{
		strings.Repeat(" ", 1<<20) + "small" + strings.Repeat(" ", 1<<20),
		strings.Repeat("x", 1<<20)[:5],
		strings.Repeat("界", 5000),
	} {
		trimmed := strings.TrimSpace(input)
		output := boundedLogText(input, maxLogTextBytes)
		if unsafe.StringData(trimmed) == unsafe.StringData(output) {
			t.Fatal("bounded text retains the original input allocation")
		}
		if len(output) > maxLogTextBytes || !utf8.ValidString(output) {
			t.Fatal("bounded text exceeds its size limit or splits UTF-8")
		}
	}
}
