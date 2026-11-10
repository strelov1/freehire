package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func doc(id int, source string) string {
	b, _ := json.Marshal(map[string]any{"id": id, "source": source, "skills": []string{"go"}})
	return string(b)
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func loaded(t *testing.T, snap string, docs ...string) (*Server, http.Handler) {
	t.Helper()
	s := NewServer(snap)
	h := s.Handler()
	call(t, h, "POST", "/rebuild/begin", "")
	call(t, h, "POST", "/rebuild/push", `{"docs":[`+strings.Join(docs, ",")+`]}`)
	if c, out := call(t, h, "POST", "/rebuild/commit", ""); c != 200 {
		t.Fatalf("commit: %d %v", c, out)
	}
	return s, h
}

func total(t *testing.T, h http.Handler, filter string) float64 {
	t.Helper()
	c, out := call(t, h, "POST", "/facets", `{"filter":`+filter+`,"facets":["source"]}`)
	if c != 200 {
		t.Fatalf("facets %s: %d %v", filter, c, out)
	}
	return out["total"].(float64)
}

func TestNotReadyThenRebuild(t *testing.T) {
	s := NewServer("")
	h := s.Handler()
	for _, p := range []string{"/facets", "/distinct", "/write"} {
		if c, _ := call(t, h, "POST", p, `{"filter":null,"facets":["source"]}`); c != 503 {
			t.Errorf("%s before load = %d, want 503", p, c)
		}
	}
	if c, _ := call(t, h, "GET", "/ready", ""); c != 503 {
		t.Errorf("/ready = %d", c)
	}
	_, h = loaded(t, "", doc(1, "a"), doc(2, "b"))
	if c, _ := call(t, h, "GET", "/ready", ""); c != 200 {
		t.Errorf("/ready after commit = %d", c)
	}
	if n := total(t, h, "null"); n != 2 {
		t.Errorf("total = %v", n)
	}
}

func TestQueryShapesAndRefusals(t *testing.T) {
	_, h := loaded(t, "", doc(1, "a"), doc(2, "b"))
	for filter, want := range map[string]float64{
		`"source = a"`:                    1,
		`["source = a"]`:                  1,
		`[["source = a","source = b"]]`:   2,
		`["skills = go",["source = a"]]`:  1, // mixed strings and groups
		`[["source = a"],["source = b"]]`: 0,
	} {
		if got := total(t, h, filter); got != want {
			t.Errorf("filter %s = %v, want %v", filter, got, want)
		}
	}
	if c, _ := call(t, h, "POST", "/facets", `{"filter":"bogus ??","facets":["source"]}`); c != 422 {
		t.Errorf("bad filter = %d, want 422", c)
	}
	if c, _ := call(t, h, "POST", "/facets", `{"filter":[1],"facets":["source"]}`); c != 422 {
		t.Errorf("non-string element = %d, want 422", c)
	}
	if c, _ := call(t, h, "POST", "/facets", `{"filter":null,"facets":["nope"]}`); c != 422 {
		t.Errorf("unknown facet = %d, want 422", c)
	}
	if c, _ := call(t, h, "POST", "/distinct", `{"filter":null,"attr":"nope"}`); c != 422 {
		t.Errorf("unknown attr = %d, want 422", c)
	}
	if c, out := call(t, h, "POST", "/distinct", `{"filter":null,"attr":"source"}`); c != 200 || out["count"].(float64) != 2 {
		t.Errorf("distinct = %d %v", c, out)
	}
	if c, _ := call(t, h, "POST", "/facets", `{not json`); c != 400 {
		t.Errorf("bad body = %d, want 400", c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), "GET", "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{`facetd_refused_total{reason="unsupported"} 2`, `facetd_refused_total{reason="unknown_facet"} 2`,
		`facetd_requests_total{endpoint="facets",status="422"} 3`, "facetd_documents 2", "facetd_documents_live 2", "facetd_generation"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
}

func TestWriteApplies(t *testing.T) {
	_, h := loaded(t, "", doc(1, "a"), doc(2, "b"))
	if c, _ := call(t, h, "POST", "/write", `{"close":[1],"upserts":[`+doc(3, "b")+`]}`); c != 200 {
		t.Fatalf("write = %d", c)
	}
	if n := total(t, h, `"source = b"`); n != 2 {
		t.Errorf("b = %v, want 2", n)
	}
	if n := total(t, h, `"source = a"`); n != 0 {
		t.Errorf("closed a = %v, want 0", n)
	}
	if c, _ := call(t, h, "POST", "/write", `{"upserts":[{"id":"x"}]}`); c != 400 {
		t.Errorf("bad doc = %d, want 400", c)
	}
}

// During the first rebuild there is no index to apply to, yet a bad write is still refused
// rather than acknowledged and dropped at commit; a good one is recorded and kept.
func TestFirstRebuildValidatesWrites(t *testing.T) {
	s := NewServer("")
	h := s.Handler()
	call(t, h, "POST", "/rebuild/begin", "")
	if c, _ := call(t, h, "POST", "/write", `{"upserts":[{"id":"bad"}],"close":[1]}`); c != 400 {
		t.Fatalf("bad write = %d, want 400", c)
	}
	// close names a job the empty index has never seen: still a valid batch
	if c, _ := call(t, h, "POST", "/write", `{"close":[1],"upserts":[`+doc(9, "c")+`]}`); c != 200 {
		t.Fatalf("good write = %d", c)
	}
	call(t, h, "POST", "/rebuild/push", `{"docs":[`+doc(1, "a")+`]}`)
	if c, _ := call(t, h, "POST", "/rebuild/commit", ""); c != 200 {
		t.Fatal("commit failed")
	}
	if n := total(t, h, `"source = c"`); n != 1 {
		t.Errorf("write recorded during the first rebuild lost: c = %v", n)
	}
	if n := total(t, h, `"source = a"`); n != 0 {
		t.Errorf("close recorded during the first rebuild lost: a = %v", n)
	}
}

// A write that arrives between begin and commit must survive the swap.
func TestWriteDuringRebuildIsReapplied(t *testing.T) {
	s, h := loaded(t, "", doc(1, "a"), doc(2, "b"))
	call(t, h, "POST", "/rebuild/begin", "")
	call(t, h, "POST", "/rebuild/push", `{"docs":[`+doc(1, "a")+`,`+doc(2, "b")+`]}`)
	call(t, h, "POST", "/write", `{"close":[1],"upserts":[`+doc(9, "c")+`]}`)
	if n := total(t, h, `"source = a"`); n != 0 {
		t.Errorf("live index missed the write: a = %v", n)
	}
	if c, _ := call(t, h, "POST", "/rebuild/commit", ""); c != 200 {
		t.Fatal("commit failed")
	}
	if n := total(t, h, `"source = a"`); n != 0 {
		t.Errorf("close lost across the swap: a = %v", n)
	}
	if n := total(t, h, `"source = c"`); n != 1 {
		t.Errorf("upsert lost across the swap: c = %v", n)
	}
	if s.rebuilding || len(s.pending) != 0 {
		t.Error("rebuild state not cleared")
	}
}

func TestRebuildAbortAndConflicts(t *testing.T) {
	s, h := loaded(t, "", doc(1, "a"))
	if c, _ := call(t, h, "POST", "/rebuild/commit", ""); c != 409 {
		t.Errorf("commit without begin = %d, want 409", c)
	}
	if c, _ := call(t, h, "POST", "/rebuild/push", `{"docs":[]}`); c != 409 {
		t.Errorf("push without begin = %d, want 409", c)
	}
	call(t, h, "POST", "/rebuild/begin", "")
	call(t, h, "POST", "/rebuild/push", `{"docs":[`+doc(5, "z")+`]}`)
	if c, _ := call(t, h, "POST", "/rebuild/begin", ""); c != 409 {
		t.Errorf("begin while a rebuild is open = %d, want 409", c)
	}
	call(t, h, "POST", "/rebuild/abort", "")
	if s.rebuilding {
		t.Error("abort left rebuild open")
	}
	if n := total(t, h, "null"); n != 1 {
		t.Errorf("abort changed the index: %v", n)
	}
}

func TestSnapshotRoundTripAndCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facetd.snap")
	s, _ := loaded(t, path, doc(1, "a"), doc(2, "b"))
	if err := s.Snapshot(true); err != nil { // already written by commit: no-op
		t.Fatal(err)
	}
	s2 := NewServer(path)
	if err := s2.LoadSnapshot(); err != nil {
		t.Fatal(err)
	}
	if n := total(t, s2.Handler(), "null"); n != 2 {
		t.Errorf("restored total = %v", n)
	}
	// corrupt: stays not-ready
	if err := writeFile(path, bytes.Repeat([]byte{7}, 64)); err != nil {
		t.Fatal(err)
	}
	s3 := NewServer(path)
	if err := s3.LoadSnapshot(); err == nil {
		t.Fatal("corrupt snapshot loaded")
	}
	if c, _ := call(t, s3.Handler(), "GET", "/ready", ""); c != 503 {
		t.Errorf("ready after corrupt = %d", c)
	}
	if err := NewServer(filepath.Join(t.TempDir(), "missing")).LoadSnapshot(); err == nil {
		t.Error("missing snapshot loaded")
	}
}

func writeFile(path string, b []byte) error {
	_, err := writeFileAtomic(path, func(w io.Writer) error { _, err := w.Write(b); return err })
	return err
}

// TestIdleRebuildIsAbandoned: a rebuild nobody pushes to for rebuildLease stops recording
// /write batches, so a reindex killed between begin and commit cannot grow them forever.
func TestIdleRebuildIsAbandoned(t *testing.T) {
	s, h := loaded(t, "", doc(1, "a"))
	call(t, h, "POST", "/rebuild/begin", "")
	s.mu.Lock()
	s.rebuildAt = s.rebuildAt.Add(-rebuildLease - time.Second)
	s.mu.Unlock()
	if c, out := call(t, h, "POST", "/write", `{"upserts":[`+doc(2, "b")+`]}`); c != 200 {
		t.Fatalf("write: %d %v", c, out)
	}
	s.mu.Lock()
	rebuilding, pending := s.rebuilding, len(s.pending)
	s.mu.Unlock()
	if rebuilding || pending != 0 {
		t.Fatalf("rebuilding=%t pending=%d, want the idle rebuild abandoned", rebuilding, pending)
	}
	if c, _ := call(t, h, "POST", "/rebuild/push", `{"docs":[`+doc(3, "c")+`]}`); c != 409 {
		t.Errorf("push into an abandoned rebuild = %d, want 409", c)
	}
	if got := total(t, h, "null"); got != 2 {
		t.Errorf("total = %v, want 2: the write is applied to the live index", got)
	}
}

// A commit whose snapshot cannot be written still swaps the index in but answers 500, so the
// reindex knows the rebuild is not durable.
func TestCommitReportsSnapshotFailure(t *testing.T) {
	s := NewServer(filepath.Join(t.TempDir(), "missing-dir", "facetd.snap"))
	h := s.Handler()
	call(t, h, "POST", "/rebuild/begin", "")
	call(t, h, "POST", "/rebuild/push", `{"docs":[`+doc(1, "a")+`]}`)
	if c, body := call(t, h, "POST", "/rebuild/commit", ""); c != 500 {
		t.Errorf("commit with a failing snapshot = %d %s, want 500", c, body)
	}
	if n := total(t, h, "null"); n != 1 {
		t.Errorf("committed index not live: total = %v", n)
	}
}

func TestMetricsCarryMemoryAndBatchSize(t *testing.T) {
	_, h := loaded(t, "", doc(1, "a"))
	if c, _ := call(t, h, "POST", "/write", `{"close":[1],"duplicate":[3],"delete":[4],"upserts":[`+doc(2, "b")+`]}`); c != 200 {
		t.Fatalf("write = %d", c)
	}
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"\nfacetd_write_batch_jobs_count 1\n", "\nfacetd_write_batch_jobs_sum 4\n", "\ngo_memstats_heap_alloc_bytes "} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics lacks %q", want)
		}
	}
}
