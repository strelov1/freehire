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
	call(t, h, "POST", "/rebuild/begin", "") // resets
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
