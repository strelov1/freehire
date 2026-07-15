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
)

// jobPassage must prefix the corpus side with e5's "passage:" marker and weave in the
// title/company/body, so it stays comparable to the "query:"-prefixed CV. It embeds the
// description by default but prefers the enrichment summary when present.
func TestJobPassage(t *testing.T) {
	var d JobDocument
	d.Title = "Backend Engineer"
	d.Company = "Acme"
	d.Description = "Go and Postgres"

	if got, want := jobPassage(d), "passage: Backend Engineer at Acme. Go and Postgres"; got != want {
		t.Fatalf("jobPassage (description) = %q, want %q", got, want)
	}

	d.Enrichment.Summary = "Senior Go role building payment APIs"
	if got, want := jobPassage(d), "passage: Backend Engineer at Acme. Senior Go role building payment APIs"; got != want {
		t.Fatalf("jobPassage (summary preferred) = %q, want %q", got, want)
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

// EmbedJobs must compute a vector per job id WITHOUT touching Meilisearch — the pg-only
// backfill path. The Client here has an embedder but no Meili wiring, so any Meili call
// would panic; returning cleanly proves the embed side stands alone.
func TestEmbedJobsReturnsVectorPerJobWithoutMeili(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Inputs []string `json:"inputs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := make([][]float64, len(in.Inputs))
		for i := range in.Inputs {
			out[i] = []float64{float64(i) + 0.5} // positional so id->vector mapping is checkable
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL, embedConcurrency: 1}

	docs := []JobDocument{{ID: 7}, {ID: 42}}
	docs[0].Title, docs[1].Title = "A", "B"

	got, err := c.EmbedJobs(context.Background(), docs)
	if err != nil {
		t.Fatalf("EmbedJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d vectors, want 2", len(got))
	}
	if v := got[7]; len(v) != 1 || v[0] != 0.5 {
		t.Errorf("vector for job 7 = %v; want [0.5]", v)
	}
	if v := got[42]; len(v) != 1 || v[0] != 1.5 {
		t.Errorf("vector for job 42 = %v; want [1.5]", v)
	}
}
