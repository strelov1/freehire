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

func TestEachEventIsOneLineInItsDaysFile(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)
	day1 := time.Date(2026, 10, 27, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 10, 28, 0, 1, 0, 0, time.UTC)
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
	w := newWriter(t.TempDir(), 1)
	w.pause() // the drain goroutine holds off, so the buffer fills
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
	if w.Dropped() == 0 {
		t.Error("Dropped() = 0, want the overflow counted")
	}
	w.resume()
	w.Close()
}
