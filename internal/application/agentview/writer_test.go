package agentview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnUnsetDirectoryIsANoOpWriter(t *testing.T) {
	w := NewWriter("")
	if w != nil {
		t.Fatal("NewWriter(\"\") must be nil")
	}
	w.Write(Event{}) // must not panic
	w.Close()
}

// clockAt is a writer clock stopped at t.
func clockAt(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestEachEventIsOneLineInItsDaysFile(t *testing.T) {
	dir := t.TempDir()
	day1 := time.Date(2026, 10, 27, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 10, 28, 0, 1, 0, 0, time.UTC)
	w := newWriter(dir, clockAt(day2))
	w.Write(Event{TS: day1, Kind: KindSearch, Surface: SurfaceMCP, Viewer: "v", JobIDs: []int64{1, 2}})
	w.Write(Event{TS: day2, Kind: KindDetail, Surface: SurfaceMCP, Viewer: "v", JobIDs: []int64{1}})
	w.Close()

	for name, want := range map[string]int{"agent-views-2026-10-27.jsonl": 1, "agent-views-2026-10-28.jsonl": 1} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(b), "\n"); n != want {
			t.Errorf("%s: %d lines, want %d", name, n, want)
		}
	}
}

func TestAFullBufferDropsInsteadOfBlocking(t *testing.T) {
	// No drain goroutine, so nothing empties the one-slot buffer: the first event fills
	// it and every later one must be dropped rather than wait.
	w := &Writer{ch: make(chan Event, 1)}
	done := make(chan struct{})
	go func() {
		for range 10 {
			w.Write(Event{TS: time.Now().UTC(), JobIDs: []int64{1}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked on a full buffer")
	}
	if got := w.Dropped(); got != 9 {
		t.Errorf("Dropped() = %d, want 9", got)
	}
}

func TestAWriteAfterCloseIsDroppedNotAPanic(t *testing.T) {
	w := NewWriter(t.TempDir())
	w.Close()

	w.Write(Event{TS: time.Now().UTC(), Kind: KindDetail, JobIDs: []int64{1}})
	w.Close() // idempotent

	if w.Dropped() != 1 {
		t.Errorf("Dropped() = %d, want the late write counted", w.Dropped())
	}
}

func TestAWriterCreatesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")
	ts := time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)
	w := newWriter(dir, clockAt(ts))
	w.Write(Event{TS: ts, Kind: KindSearch, Surface: SurfaceMCP, Viewer: "v", JobIDs: []int64{1}})
	w.Close()

	b, err := os.ReadFile(filepath.Join(dir, "agent-views-2026-10-28.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 1 {
		t.Errorf("%d lines, want 1", n)
	}
}

func TestAnEventTooLateForItsDayIsDropped(t *testing.T) {
	// Once its day may have been rolled up, an append would change the file the rollup
	// already claimed and have the whole day counted again.
	dir := t.TempDir()
	ts := time.Date(2026, 10, 27, 23, 59, 0, 0, time.UTC)
	w := newWriter(dir, clockAt(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC).Add(lateWriteCutoff+time.Second)))
	w.Write(Event{TS: ts, Kind: KindSearch, Surface: SurfaceMCP, Viewer: "v", JobIDs: []int64{1}})
	w.Close()

	if _, err := os.Stat(filepath.Join(dir, FileName(ts))); !os.IsNotExist(err) {
		t.Errorf("late event reached its day's file (stat err %v)", err)
	}
	if w.Dropped() != 1 {
		t.Errorf("Dropped() = %d, want the late event counted", w.Dropped())
	}
}

func TestTheWriterStopsBeforeTheRollupReads(t *testing.T) {
	if lateWriteCutoff >= SettleAfter {
		t.Fatalf("lateWriteCutoff %v must be below SettleAfter %v", lateWriteCutoff, SettleAfter)
	}
}
