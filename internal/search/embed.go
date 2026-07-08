package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// teiMaxBatch caps how many inputs go in one TEI /v1/embeddings call. TEI rejects a
// batch above its --max-client-batch-size (default 32); embedBatch chunks larger
// inputs into sequential calls so callers can hand it a whole reindex batch.
const teiMaxBatch = 32

// embedMaxAttempts bounds how many times embedChunk retries a transient embedding-backend
// failure (a transport error, 429, or 5xx) before giving up. A bulk semantic reindex makes
// hundreds of thousands of cross-network calls to a remote embedding endpoint over many
// hours, so a single blip (a dropped connection, a brief endpoint restart) must not abort
// the whole run — but a persistent outage, or a deterministic 4xx from a malformed batch,
// still surfaces after the bounded retries rather than looping forever.
const embedMaxAttempts = 5

// embedAttemptTimeout caps a single embedding HTTP call so a hung connection is retried
// rather than stalling the run indefinitely — http.DefaultClient has no timeout of its own.
const embedAttemptTimeout = 60 * time.Second

// embedRetryBase is the backoff before the first retry; it doubles each attempt (1s, 2s,
// 4s, …). A package var, not a const, so tests can shrink it to keep them fast.
var embedRetryBase = time.Second

// jobPassage renders a job document into the text embedded for semantic retrieval.
// e5 is asymmetric: the corpus side carries the "passage:" prefix and the query side
// carries "query:" (see EmbedText), so they must be embedded the same way to be
// comparable.
//
// It prefers the enrichment summary over the raw description: the summary is a short,
// model-written synopsis (capped well under e5's 512-token window) that captures the
// whole role — including requirements a long description buries past the truncation
// point — so it embeds the job more faithfully than the head-truncated description, and
// its distilled form is closer to a CV query. Unenriched jobs fall back to the
// description (already capped at maxIndexedDescriptionRunes).
func jobPassage(d JobDocument) string {
	body := d.Description
	if s := d.Enrichment.Summary; s != "" {
		body = s
	}
	return "passage: " + d.Title + " at " + d.Company + ". " + body
}

// embedBatch turns texts into vectors, in input order, by calling the embedding backend
// (see embedChunk). We embed here and store the result as a userProvided Meilisearch
// embedder (see jobEmbedder) rather than letting Meili's rest embedder reach the server
// itself: the engine rejects the loopback TEI URI, and embedding in one place keeps the
// job corpus and the CV query on an identical path (one model, one server → one vector
// space). Inputs are chunked to the backend's per-call batch limit, and up to
// embedConcurrency chunks run in flight — a remote GPU endpoint needs the concurrency to
// hide per-call latency (the CPU-bound host2 TEI runs it at 1).
func (c *Client) embedBatch(ctx context.Context, inputs []string) ([][]float64, error) {
	type span struct{ start, end int }
	var chunks []span
	for start := 0; start < len(inputs); start += teiMaxBatch {
		end := start + teiMaxBatch
		if end > len(inputs) {
			end = len(inputs)
		}
		chunks = append(chunks, span{start, end})
	}

	conc := c.embedConcurrency
	if conc < 1 {
		conc = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Each chunk writes its vectors into its own slot, so the flattened result stays in
	// input order regardless of completion order.
	results := make([][][]float64, len(chunks))
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i, ch := range chunks {
		wg.Add(1)
		go func(i int, ch span) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			vecs, err := c.embedChunk(ctx, inputs[ch.start:ch.end])
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel() // stop the remaining chunks on the first failure
				}
				mu.Unlock()
				return
			}
			results[i] = vecs
		}(i, ch)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	out := make([][]float64, 0, len(inputs))
	for _, r := range results {
		out = append(out, r...)
	}
	return out, nil
}

// embedChunk embeds one TEI-sized batch and returns the vectors in input order,
// retrying a transient backend failure with exponential backoff (see embedMaxAttempts).
// A long bulk reindex crosses the network hundreds of thousands of times, so one blip
// must not abort it — but a deterministic error (a 4xx, a malformed response) or a
// cancelled parent context fails fast without burning the retry budget.
func (c *Client) embedChunk(ctx context.Context, inputs []string) ([][]float64, error) {
	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("search: embed marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < embedMaxAttempts; attempt++ {
		if attempt > 0 {
			// Back off before retrying, but abort promptly if the parent context is done
			// (e.g. a sibling chunk failed and cancelled the batch).
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(embedRetryBase << (attempt - 1)):
			}
		}
		vecs, retriable, err := c.embedOnce(ctx, body, len(inputs))
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		if !retriable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("search: embed: giving up after %d attempts: %w", embedMaxAttempts, lastErr)
}

// embedOnce makes a single embedding call and reports whether a failure is worth
// retrying. It speaks TEI's native `/embed` shape — `{"inputs": [...]}` in, an array of
// vectors out — which every backend we target accepts: the host2 TEI (/embed, bare array)
// and an HF Inference Endpoint (root, `{"embeddings": [...]}`). Over-long inputs (e5 caps
// at 512 tokens) are truncated server-side (host2 TEI's --auto-truncate; HF truncates by
// default), so no per-input length handling is needed here.
//
// Transport errors, 429, and 5xx are transient (retriable); a 4xx or a mismatched vector
// count is deterministic (not retriable); a cancelled parent context is terminal.
func (c *Client) embedOnce(ctx context.Context, body []byte, n int) ([][]float64, bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, embedAttemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.embedURL, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("search: embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.embedKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.embedKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// A cancelled PARENT context is terminal (the whole batch is being torn down);
		// any other transport failure — including this attempt's own timeout — is transient.
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, true, fmt.Errorf("search: embed call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		retriable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retriable, fmt.Errorf("search: embed: unexpected status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		// A body cut short mid-read is a broken connection — transient.
		return nil, true, fmt.Errorf("search: embed read: %w", err)
	}
	vecs, err := parseEmbeddings(raw)
	if err != nil {
		return nil, false, err
	}
	if len(vecs) != n {
		return nil, false, fmt.Errorf("search: embed: got %d vectors for %d inputs", len(vecs), n)
	}
	return vecs, false, nil
}

// parseEmbeddings decodes a TEI-style embeddings response, tolerating both the bare
// array of vectors (`[[...], ...]`, TEI /embed) and the object form
// (`{"embeddings": [[...], ...]}`, HF Inference Endpoints).
func parseEmbeddings(raw []byte) ([][]float64, error) {
	var bare [][]float64
	if err := json.Unmarshal(raw, &bare); err == nil && len(bare) > 0 {
		return bare, nil
	}
	var wrapped struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Embeddings) > 0 {
		return wrapped.Embeddings, nil
	}
	return nil, fmt.Errorf("search: embed: unrecognized response shape")
}

// semanticDocument is a JobDocument carrying its precomputed embedding for the
// userProvided embedder. The embedded JobDocument flattens its own fields into the
// document; _vectors adds the vector Meilisearch stores and searches by.
type semanticDocument struct {
	JobDocument
	Vectors map[string][]float32 `json:"_vectors"`
}

// embedDocs embeds each job's passage text and wraps it with its vector, ready to push
// into the semantic index.
func (c *Client) embedDocs(ctx context.Context, docs []JobDocument) ([]semanticDocument, error) {
	inputs := make([]string, len(docs))
	for i, d := range docs {
		inputs[i] = jobPassage(d)
	}
	vecs, err := c.embedBatch(ctx, inputs)
	if err != nil {
		return nil, err
	}
	out := make([]semanticDocument, len(docs))
	for i, d := range docs {
		out[i] = semanticDocument{JobDocument: d, Vectors: map[string][]float32{embedderName: toFloat32(vecs[i])}}
	}
	return out, nil
}

// toFloat32 narrows a float64 vector to the float32 Meilisearch stores.
func toFloat32(v []float64) []float32 {
	f := make([]float32, len(v))
	for i, x := range v {
		f[i] = float32(x)
	}
	return f
}
