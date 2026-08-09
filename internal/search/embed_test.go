package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/db"
)

// jobPassages must prefix EVERY chunk with e5's "passage:" marker and the
// title/company context (so a query matching just one chunk still stays comparable
// to the "query:"-prefixed CV), one passage per chunk of semanticText — FromJob's
// full, HTML-stripped copy of the description, never the facet-index-capped
// Description (see maxIndexedDescriptionRunes) and never the enrichment summary
// (design.md Decision 5: the summary's only advantage, dodging truncation, evaporates
// once the full text is chunked instead of capped, so it is no longer special-cased).
// A short description (chunkText's common case) yields exactly one passage.
func TestJobPassages_ShortTextYieldsOnePassage(t *testing.T) {
	var d JobDocument
	d.Title = "Backend Engineer"
	d.Company = "Acme"
	d.Description = "truncated facet-index stub, must be ignored"
	d.Enrichment.Summary = "short synopsis, must be ignored"
	d.semanticText = "Go and Postgres. Full requirements list past the facet-index cap."

	want := []string{"passage: Backend Engineer at Acme. Go and Postgres. Full requirements list past the facet-index cap."}
	if got := jobPassages(d); !slices.Equal(got, want) {
		t.Fatalf("jobPassages = %v, want %v", got, want)
	}
}

// A long description splits into multiple chunks (see chunkText), and EACH chunk gets
// its own "passage: {title} at {company}." prefix — every chunk becomes an
// independently-scored vector (nearest-of-N, see tasks.md task 1's Meilisearch spike),
// so each one needs the same job-identifying context a single-vector passage carried.
func TestJobPassages_LongTextPrefixesEveryChunk(t *testing.T) {
	var d JobDocument
	d.Title = "Backend Engineer"
	d.Company = "Acme"
	d.semanticText = strings.Join([]string{
		strings.Repeat("alpha ", 400),
		strings.Repeat("beta ", 400),
	}, "\n")

	chunks := chunkText(d.semanticText)
	if len(chunks) < 2 {
		t.Fatalf("fixture chunkText(...) = %d chunks, want >= 2 (test needs a multi-chunk case)", len(chunks))
	}
	got := jobPassages(d)
	if len(got) != len(chunks) {
		t.Fatalf("jobPassages = %d passages, want %d (one per chunk)", len(got), len(chunks))
	}
	for i, c := range chunks {
		want := "passage: Backend Engineer at Acme. " + c
		if got[i] != want {
			t.Fatalf("jobPassages[%d] = %q, want %q", i, got[i], want)
		}
	}
}

// A job with no description text (chunkText("") yields no chunks) must yield no
// passages at all — nothing to embed — rather than one passage of just the prefix.
func TestJobPassages_EmptyTextYieldsNoPassages(t *testing.T) {
	var d JobDocument
	d.Title = "Backend Engineer"
	d.Company = "Acme"
	if got := jobPassages(d); got != nil {
		t.Fatalf("jobPassages(empty) = %v, want nil", got)
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

// EmbedJobs must compute a chunk vector per job id WITHOUT touching Meilisearch — the
// pg-only backfill path. The Client here has an embedder but no Meili wiring, so any
// Meili call would panic; returning cleanly proves the embed side stands alone.
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
	docs[0].semanticText, docs[1].semanticText = "short body one", "short body two"

	got, err := c.EmbedJobs(context.Background(), docs)
	if err != nil {
		t.Fatalf("EmbedJobs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d jobs' vectors, want 2", len(got))
	}
	if v := got[7]; len(v) != 1 || len(v[0]) != 1 || v[0][0] != 0.5 {
		t.Errorf("vectors for job 7 = %v; want [[0.5]]", v)
	}
	if v := got[42]; len(v) != 1 || len(v[0]) != 1 || v[0][0] != 1.5 {
		t.Errorf("vectors for job 42 = %v; want [[1.5]]", v)
	}
}

// A job with no description text contributes zero chunks and so zero embedding
// inputs — it must be left out of EmbedJobs' result entirely (a design consequence of
// design.md Decision 5 dropping the old always-non-empty title/company-only passage),
// not appear with an empty vector list Meilisearch's userProvided embedder would reject.
func TestEmbedJobsSkipsJobWithNoDescriptionText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	c := &Client{embedURL: srv.URL, embedConcurrency: 1}

	docs := []JobDocument{{ID: 1}, {ID: 2}} // no semanticText on either
	docs[1].semanticText = "has a real body"

	got, err := c.EmbedJobs(context.Background(), docs)
	if err != nil {
		t.Fatalf("EmbedJobs: %v", err)
	}
	if _, ok := got[1]; ok {
		t.Errorf("job 1 (empty description) present in result: %v", got[1])
	}
	if _, ok := got[2]; !ok {
		t.Errorf("job 2 (has description) missing from result")
	}
}

// End-to-end spot check (tasks.md task 6.3): a realistic long, HTML-heavy description —
// the kind a real ATS board posts, with headings/paragraphs/lists — must reach the
// embedding path in full, from the raw jobs.description column all the way to
// jobPassages' output. Nothing past the OLD facet-index cap (maxIndexedDescriptionRunes,
// 1000 runes) and nothing past a single chunk may be silently dropped — the exact
// failure this whole change exists to fix (proposal.md motivation #2). A marker placed
// at the very end of the description must survive both stages and land in the LAST
// passage, proving chunk order is preserved end to end.
func TestEndToEnd_LongHTMLDescriptionReachesLastChunk(t *testing.T) {
	const marker = "REQUIRES-EXPERT-KUBERNETES-CERTIFICATION-XYZ789"
	var body strings.Builder
	body.WriteString("<h2>About the role</h2><p>")
	body.WriteString(strings.Repeat("We build reliable backend systems at scale. ", 60))
	body.WriteString("</p><h2>Requirements</h2><ul>")
	for i := 0; i < 20; i++ {
		body.WriteString("<li>" + strings.Repeat("Solid engineering fundamentals and collaboration skills. ", 3) + "</li>")
	}
	body.WriteString("</ul><h2>Nice to have</h2><p>")
	body.WriteString(marker) // deliberately placed at the very end
	body.WriteString("</p>")
	html := body.String()

	if n := utf8.RuneCountInString(html); n <= maxIndexedDescriptionRunes {
		t.Fatalf("fixture is only %d runes, want > %d (must exceed the OLD facet-index cap to be a meaningful test)", n, maxIndexedDescriptionRunes)
	}

	doc, err := FromJob(db.Job{ID: 1, Title: "Staff Platform Engineer", Company: "Acme", Description: html})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if strings.Contains(doc.Description, marker) {
		t.Fatalf("marker leaked into the facet-index Description, which should stay capped at %d runes", maxIndexedDescriptionRunes)
	}

	passages := jobPassages(doc)
	if len(passages) < 2 {
		t.Fatalf("got %d passages, want > 1 (fixture is long enough to force multiple chunks)", len(passages))
	}
	if !strings.Contains(passages[len(passages)-1], marker) {
		t.Errorf("marker (placed at the very end of the description) not in the LAST passage — "+
			"content past the old facet cap was lost, or chunk order is wrong: %v", passages)
	}
}
