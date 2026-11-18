package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func upsert(ids ...int) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += doc(id, "w")
	}
	return `{"upserts":[` + out + `]}`
}

func recover_(t *testing.T, snap string) (*Server, http.Handler) {
	t.Helper()
	s := NewServer(snap)
	if err := s.LoadSnapshot(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	return s, s.Handler()
}

func TestWALReplayAfterCrash(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	_, h := loaded(t, snap, doc(1, "a"), doc(2, "a"))
	for _, body := range []string{upsert(3, 4), `{"close":[1]}`, `{"delete":[2]}`} {
		if c, out := call(t, h, "POST", "/write", body); c != 200 {
			t.Fatalf("write %s: %d %v", body, c, out)
		}
	}
	want := total(t, h, "null")
	_, h2 := recover_(t, snap) // crash: no final snapshot
	if got := total(t, h2, "null"); got != want {
		t.Fatalf("after replay total = %v, want %v", got, want)
	}
	if got := total(t, h2, `"source = w"`); got != 2 {
		t.Fatalf("replayed upserts = %v, want 2", got)
	}
}

func TestWALTornTailIgnored(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	_, h := loaded(t, snap, doc(1, "a"))
	call(t, h, "POST", "/write", upsert(2))
	call(t, h, "POST", "/write", upsert(3))
	data, _ := os.ReadFile(snap + ".wal")
	if err := os.WriteFile(snap+".wal", data[:len(data)-5], 0o600); err != nil { // crash mid-append of the last record
		t.Fatal(err)
	}
	s, h2 := recover_(t, snap)
	if got := total(t, h2, "null"); got != 2 {
		t.Fatalf("total = %v, want 2 (snapshot doc + first replayed)", got)
	}
	// the torn bytes are cut, so a new append is not stranded behind garbage
	call(t, h2, "POST", "/write", upsert(9))
	_ = s
	_, h3 := recover_(t, snap)
	if got := total(t, h3, "null"); got != 3 {
		t.Fatalf("total after append past torn tail = %v, want 3", got)
	}
}

func TestWALCorruptionMidFileFailsStartup(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	_, h := loaded(t, snap, doc(1, "a"))
	call(t, h, "POST", "/write", upsert(2))
	call(t, h, "POST", "/write", upsert(3))
	data, _ := os.ReadFile(snap + ".wal")
	data[walHeader+3] ^= 0xff // first record's payload
	_ = os.WriteFile(snap+".wal", data, 0o600)
	s := NewServer(snap)
	if err := s.LoadSnapshot(); err == nil {
		t.Fatal("corrupt log must fail startup")
	}
	if c, _ := call(t, s.Handler(), "GET", "/ready", ""); c != 503 {
		t.Fatalf("/ready = %d, want 503", c)
	}
	if m, _ := filepath.Glob(snap + ".wal.corrupt-*"); len(m) != 1 {
		t.Fatalf("corrupt log not moved aside: %v", m)
	}
}

// A damaged length in a record that is not the last must fail recovery, not be read as a
// torn tail that silently drops every acknowledged record after it.
func TestWALOversizedLengthMidFileFailsStartup(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	_, h := loaded(t, snap, doc(1, "a"))
	call(t, h, "POST", "/write", upsert(2))
	call(t, h, "POST", "/write", upsert(3))
	data, _ := os.ReadFile(snap + ".wal")
	data[0] = 0x7f // first record's length: far past maxBulkBody and past the file
	_ = os.WriteFile(snap+".wal", data, 0o600)
	if err := NewServer(snap).LoadSnapshot(); err == nil {
		t.Fatal("an oversized length must fail startup")
	}
	if m, _ := filepath.Glob(snap + ".wal.corrupt-*"); len(m) != 1 {
		t.Fatalf("corrupt log not moved aside: %v", m)
	}
}

func TestWALSnapshotTruncatesAndRestartKeepsEverything(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	s, h := loaded(t, snap, doc(1, "a"))
	call(t, h, "POST", "/write", upsert(2))
	if fi, _ := os.Stat(snap + ".wal"); fi.Size() == 0 {
		t.Fatal("log empty after a write")
	}
	if err := s.Snapshot(false); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(snap + ".wal"); fi.Size() != 0 {
		t.Fatalf("log not truncated: %d bytes", fi.Size())
	}
	call(t, h, "POST", "/write", upsert(3)) // appended to the rotated log
	_, h2 := recover_(t, snap)
	if got := total(t, h2, "null"); got != 3 {
		t.Fatalf("total = %v, want 3", got)
	}
}

func TestWALFailureReturns500AndIsNotApplied(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	s, h := loaded(t, snap, doc(1, "a"))
	call(t, h, "POST", "/write", upsert(2))
	s.mu.Lock()
	_ = s.wal.Close() // the next append fails
	s.mu.Unlock()
	if c, _ := call(t, h, "POST", "/write", upsert(3)); c != 500 {
		t.Fatalf("write with a dead log = %d, want 500", c)
	}
	if got := total(t, h, "null"); got != 2 {
		t.Fatalf("total = %v, want 2 (failed write not applied)", got)
	}
}

func TestWALRefusedBatchLeavesNoRecord(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	_, h := loaded(t, snap, doc(1, "a"))
	call(t, h, "POST", "/write", upsert(2))
	before, _ := os.Stat(snap + ".wal")
	if c, _ := call(t, h, "POST", "/write", `{"upserts":[{"no_id":true}]}`); c != 400 {
		t.Fatalf("bad doc = %d, want 400", c)
	}
	after, _ := os.Stat(snap + ".wal")
	if before.Size() != after.Size() {
		t.Fatalf("log grew by %d for a refused batch", after.Size()-before.Size())
	}
}

func TestWALRebuildCommitInterplay(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	s := NewServer(snap)
	h := s.Handler()
	call(t, h, "POST", "/rebuild/begin", "")
	call(t, h, "POST", "/rebuild/push", `{"docs":[`+doc(1, "a")+`]}`)
	if c, _ := call(t, h, "POST", "/write", upsert(2)); c != 200 { // recorded while rebuilding, no store yet
		t.Fatalf("write during rebuild = %d", c)
	}
	if c, out := call(t, h, "POST", "/rebuild/commit", ""); c != 200 {
		t.Fatalf("commit: %d %v", c, out)
	}
	call(t, h, "POST", "/write", upsert(3))
	_, h2 := recover_(t, snap)
	if got := total(t, h2, "null"); got != 3 {
		t.Fatalf("total = %v, want 3 (rebuilt doc + recorded + post-commit)", got)
	}
}

func TestWALWithoutSnapshotStaysNotReady(t *testing.T) {
	snap := filepath.Join(t.TempDir(), "idx.snap")
	rec, _ := encodeWALRecord(writeReq{Delete: []int64{1}})
	_ = os.WriteFile(snap+".wal", rec, 0o600)
	s := NewServer(snap)
	if err := s.LoadSnapshot(); err == nil {
		t.Fatal("no snapshot must stay not-ready")
	}
	if c, _ := call(t, s.Handler(), "GET", "/ready", ""); c != 503 {
		t.Fatalf("/ready = %d", c)
	}
}
