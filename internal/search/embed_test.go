package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/db"
)

// jobChunkPassages must chunk the FULL, HTML-stripped description and carry the same
// "passage: {title} at {company}. " prefix on EVERY chunk, since each chunk becomes an
// independently-scored vector.
func TestJobChunkPassages(t *testing.T) {
	job := db.Job{Title: "Backend Engineer", Company: "Acme", Description: "<p>Go and Postgres.</p>"}
	got := jobChunkPassages(job)
	want := []string{"passage: Backend Engineer at Acme. Go and Postgres."}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("jobChunkPassages = %#v, want %#v", got, want)
	}
}

// A job with no description text (or one that strips to nothing, e.g. pure markup)
// must yield zero passages, not one passage of just the prefix — chunkText's own
// "empty in, empty out" contract propagates through.
func TestJobChunkPassagesEmptyDescriptionYieldsNoPassages(t *testing.T) {
	job := db.Job{Title: "Backend Engineer", Company: "Acme", Description: ""}
	if got := jobChunkPassages(job); got != nil {
		t.Fatalf("jobChunkPassages(empty) = %#v, want nil", got)
	}
}

// A long description must split into multiple chunks, each independently prefixed.
func TestJobChunkPassagesLongDescriptionSplitsIntoMultiplePassages(t *testing.T) {
	var paras []string
	for i := 0; i < 40; i++ {
		paras = append(paras, strings.Repeat("word ", 20)+"end of paragraph.")
	}
	job := db.Job{Title: "Backend Engineer", Company: "Acme", Description: strings.Join(paras, "\n")}
	got := jobChunkPassages(job)
	if len(got) < 2 {
		t.Fatalf("jobChunkPassages(long) = %d passages, want > 1", len(got))
	}
	const prefix = "passage: Backend Engineer at Acme. "
	for i, p := range got {
		if !strings.HasPrefix(p, prefix) {
			t.Errorf("passage %d = %q, want prefix %q", i, p, prefix)
		}
	}
}

// EmbedJobChunks must return one vector per chunk per job, correctly regrouped after a
// single flattened embedBatch call — including a job with multiple chunks interleaved
// with jobs that have only one, so misalignment (a chunk's vector landing under the
// wrong job, or at the wrong index) would be caught.
func TestEmbedJobChunksRegroupsPerJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Inputs []string `json:"inputs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := make([][]float64, len(in.Inputs))
		for i, s := range in.Inputs {
			// Echo back the input's length so each vector is traceable to its passage.
			out[i] = []float64{float64(len(s))}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL, embedConcurrency: 1}

	var longParas []string
	for i := 0; i < 40; i++ {
		longParas = append(longParas, strings.Repeat("word ", 20)+"end of paragraph.")
	}
	jobs := []db.Job{
		{ID: 1, Title: "A", Company: "Acme", Description: "Short role."},
		{ID: 2, Title: "B", Company: "Acme", Description: strings.Join(longParas, "\n")},
		{ID: 3, Title: "C", Company: "Acme", Description: ""}, // no chunks at all
	}

	got, err := c.EmbedJobChunks(context.Background(), jobs)
	if err != nil {
		t.Fatalf("EmbedJobChunks: %v", err)
	}
	if len(got[1]) != 1 {
		t.Fatalf("job 1 chunks = %d, want 1", len(got[1]))
	}
	if got[1][0].ChunkIndex != 0 {
		t.Errorf("job 1 chunk 0 index = %d, want 0", got[1][0].ChunkIndex)
	}
	wantJob1Len := float64(len(jobChunkPassages(jobs[0])[0]))
	if got[1][0].Vector[0] != float32(wantJob1Len) {
		t.Errorf("job 1 vector = %v, want [%v]", got[1][0].Vector, wantJob1Len)
	}
	if n := len(got[2]); n < 2 {
		t.Fatalf("job 2 chunks = %d, want > 1", n)
	}
	for i, ce := range got[2] {
		if int(ce.ChunkIndex) != i {
			t.Errorf("job 2 chunk %d has ChunkIndex %d, want %d", i, ce.ChunkIndex, i)
		}
	}
	if _, ok := got[3]; ok {
		t.Errorf("job 3 (empty description) should have no entry, got %v", got[3])
	}
}

// teiEcho is a stub TEI /embed that returns, for each input, a one-element vector
// holding the integer the input text parses to — so a test can assert both that every
// input got its own vector and that order is preserved across chunk boundaries. It
// replies with the bare-array shape (as the host2 TEI /embed does).
func teiEcho(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Inputs []string `json:"inputs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := make([][]float64, 0, len(in.Inputs))
		for _, s := range in.Inputs {
			n, _ := strconv.Atoi(strings.TrimSpace(s))
			out = append(out, []float64{float64(n)})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
}

// embedBatch must chunk inputs past TEI's per-call limit and stitch the vectors back in
// input order — otherwise a reindex batch (2000) would either be rejected by TEI or
// scramble which vector belongs to which job.
func TestEmbedBatchChunksAndPreservesOrder(t *testing.T) {
	srv := teiEcho(t)
	defer srv.Close()
	// Concurrency > 1 so chunks complete out of order — the result must still be ordered.
	c := &Client{embedURL: srv.URL, embedConcurrency: 8}

	n := teiMaxBatch*5 + 3 // spans several chunks across the worker pool
	inputs := make([]string, n)
	for i := range inputs {
		inputs[i] = strconv.Itoa(i)
	}
	vecs, err := c.embedBatch(context.Background(), inputs)
	if err != nil {
		t.Fatalf("embedBatch: %v", err)
	}
	if len(vecs) != n {
		t.Fatalf("got %d vectors, want %d", len(vecs), n)
	}
	for i, v := range vecs {
		if len(v) != 1 || v[0] != float64(i) {
			t.Fatalf("vecs[%d] = %v, want [%d]", i, v, i)
		}
	}
}

// shrinkEmbedRetryBase makes retry backoff negligible for the duration of a test, so a
// test exercising retries does not sleep whole seconds.
func shrinkEmbedRetryBase(t *testing.T) {
	t.Helper()
	prev := embedRetryBase
	embedRetryBase = time.Microsecond
	t.Cleanup(func() { embedRetryBase = prev })
}

// A transient backend failure (a dropped connection, a brief 5xx while the endpoint
// restarts) must NOT abort a bulk reindex — embedChunk retries it. Without this, a single
// blip over a multi-hour run kills the whole embed pass (the ghost-JD incident).
func TestEmbedChunkRetriesTransientThenSucceeds(t *testing.T) {
	shrinkEmbedRetryBase(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 { // fail the first two attempts, then serve
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var in struct {
			Inputs []string `json:"inputs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := make([][]float64, len(in.Inputs))
		for i := range in.Inputs {
			out[i] = []float64{float64(i)}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL}

	vecs, err := c.embedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("embedBatch after transient failures: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vecs))
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 calls (2 failed + 1 ok), got %d", got)
	}
}

// A persistent backend outage must surface as an error after the bounded retries — not
// loop forever and not give up on the first try.
func TestEmbedChunkGivesUpAfterMaxAttempts(t *testing.T) {
	shrinkEmbedRetryBase(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL}

	if _, err := c.embedBatch(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected an error after exhausting retries, got nil")
	}
	if got := calls.Load(); got != embedMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", embedMaxAttempts, got)
	}
}

// A 4xx is a deterministic client error (a malformed batch) — retrying it only wastes the
// budget, so embedChunk must fail fast on the first call.
func TestEmbedChunkDoesNotRetryClientError(t *testing.T) {
	shrinkEmbedRetryBase(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL}

	if _, err := c.embedBatch(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected an error on a 4xx, got nil")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 call (no retry on 4xx), got %d", got)
	}
}

// A TEI reply with a different vector count than inputs is corruption we must reject,
// not silently misalign vectors to jobs.
func TestEmbedBatchRejectsCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One vector regardless of input count (wrapped/HF shape), to exercise the
		// count-mismatch guard.
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float64{{1}}})
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL}

	if _, err := c.embedBatch(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("expected an error on vector/input count mismatch, got nil")
	}
}
