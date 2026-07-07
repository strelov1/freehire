package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// jobPassage must prefix the corpus side with e5's "passage:" marker and weave in the
// title/company/description, so it stays comparable to the "query:"-prefixed CV.
func TestJobPassage(t *testing.T) {
	var d JobDocument
	d.Title = "Backend Engineer"
	d.Company = "Acme"
	d.Description = "Go and Postgres"
	got := jobPassage(d)
	want := "passage: Backend Engineer at Acme. Go and Postgres"
	if got != want {
		t.Fatalf("jobPassage = %q, want %q", got, want)
	}
}

// teiEcho is a stub TEI /v1/embeddings that returns, for each input, a one-element
// vector holding the integer the input text parses to — so a test can assert both that
// every input got its own vector and that order is preserved across chunk boundaries.
func teiEcho(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type item struct {
			Embedding []float64 `json:"embedding"`
		}
		out := struct {
			Data []item `json:"data"`
		}{}
		for _, s := range in.Input {
			n, _ := strconv.Atoi(strings.TrimSpace(s))
			out.Data = append(out.Data, item{Embedding: []float64{float64(n)}})
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
	c := &Client{embedURL: srv.URL}

	n := teiMaxBatch*2 + 3 // spans three chunks
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

// A TEI reply with a different vector count than inputs is corruption we must reject,
// not silently misalign vectors to jobs.
func TestEmbedBatchRejectsCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float64{1}}}, // one vector regardless of input count
		})
	}))
	defer srv.Close()
	c := &Client{embedURL: srv.URL}

	if _, err := c.embedBatch(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("expected an error on vector/input count mismatch, got nil")
	}
}
