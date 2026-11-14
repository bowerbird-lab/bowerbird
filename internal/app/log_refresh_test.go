package app

import (
	"strings"
	"sync"
	"testing"

	"bowerbird/core"
)

func TestLogRefreshCoalescesConcurrentEvents(t *testing.T) {
	var state logRefreshState
	var tasks []func()
	var mu sync.Mutex
	state.setPost(func(fn func()) {
		mu.Lock()
		tasks = append(tasks, fn)
		mu.Unlock()
	})
	refreshes := 0
	refresh := func() { refreshes++ }
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Go(func() {
			for j := 0; j < 100; j++ {
				state.request(refresh)
			}
		})
	}
	workers.Wait()
	if len(tasks) != 1 || refreshes != 0 {
		t.Fatalf("10000 events queued %d tasks and ran %d refreshes", len(tasks), refreshes)
	}
	tasks[0]()
	state.request(refresh)
	if len(tasks) != 2 || refreshes != 1 {
		t.Fatal("next batch was lost after the first refresh")
	}
	tasks[1]()
	if refreshes != 2 {
		t.Fatal("second batch did not run")
	}
}

func TestLogRefreshClosingAndReopeningRejectsStaleTasks(t *testing.T) {
	var state logRefreshState
	var tasks []func()
	post := func(fn func()) { tasks = append(tasks, fn) }
	refreshes := 0
	refresh := func() { refreshes++ }
	state.request(refresh)
	state.setPost(post)
	state.request(refresh)
	state.setPost(nil)
	state.request(refresh)
	state.setPost(post)
	state.request(refresh)
	tasks[0]()
	state.request(refresh)
	if len(tasks) != 2 || refreshes != 0 {
		t.Fatal("stale task ran or disturbed the reopened window's pending refresh")
	}
	tasks[1]()
	if refreshes != 1 {
		t.Fatal("reopened window did not refresh")
	}
}

func TestLogRefreshPreservesEventsDuringRefresh(t *testing.T) {
	var state logRefreshState
	var tasks []func()
	state.setPost(func(fn func()) { tasks = append(tasks, fn) })
	refreshed := false
	state.request(func() {
		state.request(func() { refreshed = true })
	})
	tasks[0]()
	if len(tasks) != 2 {
		t.Fatal("event arriving during refresh was lost")
	}
	tasks[1]()
	if !refreshed {
		t.Fatal("follow-up refresh did not run")
	}
}

func TestLogEventsRefreshLatestSnapshotAfterClear(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	core.ClearLogsBuffer()
	t.Cleanup(core.ClearLogsBuffer)
	a := &App{svc: core.NewService(nil)}
	var tasks []func()
	a.logRefresh.setPost(func(fn func()) { tasks = append(tasks, fn) })
	old := core.RecordLog("info", "test", "before clear", "", 0)
	a.HandleEvent(core.LogEntryEvent, old)
	core.ClearLogsBuffer()
	a.HandleEvent(core.LogsClearedEvent, "")
	latest := core.RecordLog("info", "test", "after clear", "", 0)
	a.HandleEvent(core.LogEntryEvent, latest)
	// A late event must never resurrect a cleared payload.
	a.HandleEvent(core.LogEntryEvent, old)
	if len(tasks) != 1 {
		t.Fatalf("log events queued %d tasks", len(tasks))
	}
	tasks[0]()
	if len(a.logs) != 1 || a.logs[0] != latest {
		t.Fatalf("refresh did not read the latest snapshot: %+v", a.logs)
	}
}

func TestLogFilteringReusesResultsAndReleasesOldEntries(t *testing.T) {
	a := &App{
		logs: []core.LogEntry{
			{Message: "Hello", Detail: "world", Level: "info"},
			{Message: "Other", Level: "error"},
		},
		logsLevel: "all",
	}
	if allocations := testing.AllocsPerRun(10, func() { a.filteredLogs() }); allocations != 0 {
		t.Fatalf("unfiltered logs allocate %g objects", allocations)
	}
	a.logsFilter = "HELLO WORLD"
	if got := a.filteredLogs(); len(got) != 1 || got[0].Message != "Hello" {
		t.Fatalf("filter did not match across message and detail: %+v", got)
	}
	a.logsFilter = strings.ToLower(a.logsFilter)
	if allocations := testing.AllocsPerRun(10, func() { a.filteredLogs() }); allocations != 0 {
		t.Fatalf("cached filter allocates %g objects", allocations)
	}
	backing := a.logsFiltered[:cap(a.logsFiltered)]
	a.invalidateLogFilter()
	for _, entry := range backing {
		if entry != (core.LogEntry{}) {
			t.Fatal("invalidating filter retained old entries")
		}
	}
	a.logs = []core.LogEntry{{Message: "new", Level: "error"}}
	a.logsFilter = ""
	a.logsLevel = "error"
	if got := a.filteredLogs(); len(got) != 1 || got[0].Message != "new" {
		t.Fatalf("filter reused stale entries: %+v", got)
	}
}
