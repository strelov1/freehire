package search

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// teiMaxBatch caps inputs per TEI /embed call; TEI rejects a batch above its
// --max-client-batch-size (default 32).
const teiMaxBatch = 32

// embedMaxAttempts bounds retries of a transient embedding-backend failure (transport
// error, 429, 5xx), so one blip does not abort a bulk reindex but an outage still surfaces.
const embedMaxAttempts = 5

// embedAttemptTimeout caps one embedding HTTP call; http.DefaultClient has no timeout.
const embedAttemptTimeout = 60 * time.Second

// embedRetryBase is the backoff before the first retry and doubles each attempt. A var so
// tests can shrink it.
var embedRetryBase = time.Second

// JobChunkEmbedding is one embedding vector for one 0-indexed chunk of a job's
// description. ChunkIndex matches job_semantic_chunks.chunk_index.
type JobChunkEmbedding struct {
	ChunkIndex int16
	Vector     []float32
}

// maxJobChunks caps chunks per job: ChunkIndex is an int16, so more than math.MaxInt16
// would wrap negative and collide with the primary key.
const maxJobChunks = 32768

// jobChunkPassages renders a job's HTML-stripped description into one embedding text per
// chunk. Every chunk carries the "passage: {title} at {company}. " prefix (e5 is
// asymmetric: corpus side "passage:", query side "query:") because each is scored as an
// independent vector. A job with no description text yields no passages.
func jobChunkPassages(job db.Job) []string {
	chunks := chunkText(stripToPlainText(job.Description))
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) > maxJobChunks {
		chunks = chunks[:maxJobChunks]
	}
	prefix := "passage: " + job.Title + " at " + job.Company + ". "
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = prefix + c
	}
	return out
}

// EmbedInputHash fingerprints exactly what jobChunkPassages reads (title, company, raw
// description), so cmd/embed can skip a job whose content_hash moved on a field the
// embedder never sees. Stored as jobs.semantic_embedded_input_hash.
//
// Fields are joined by U+001F (pinned by a test) and the description is raw, so a
// markup-only edit still re-embeds. A field jobChunkPassages starts reading must be added
// here, or its changes stop reaching the vectors.
func EmbedInputHash(job db.Job) string {
	h := sha256.New()
	h.Write([]byte(job.Title))
	h.Write([]byte{0x1f})
	h.Write([]byte(job.Company))
	h.Write([]byte{0x1f})
	h.Write([]byte(job.Description))
	return hex.EncodeToString(h.Sum(nil))
}

// EmbedJobChunks computes one vector per description chunk of each job, keyed by job id,
// for job_semantic_chunks; it never touches Meilisearch. All passages go through ONE
// embedBatch call and are regrouped by job. A job with no chunks is absent from the map.
func (c *Client) EmbedJobChunks(ctx context.Context, jobs []db.Job) (map[int64][]JobChunkEmbedding, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	type span struct {
		jobID      int64
		start, end int
	}
	var inputs []string
	spans := make([]span, 0, len(jobs))
	for _, job := range jobs {
		passages := jobChunkPassages(job)
		if len(passages) == 0 {
			continue
		}
		start := len(inputs)
		inputs = append(inputs, passages...)
		spans = append(spans, span{jobID: job.ID, start: start, end: len(inputs)})
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	vecs, err := c.embedBatch(ctx, inputs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]JobChunkEmbedding, len(spans))
	for _, sp := range spans {
		chunks := make([]JobChunkEmbedding, 0, sp.end-sp.start)
		for i, v := range vecs[sp.start:sp.end] {
			chunks = append(chunks, JobChunkEmbedding{ChunkIndex: int16(i), Vector: toFloat32(v)})
		}
		out[sp.jobID] = chunks
	}
	return out, nil
}

// embedBatch turns texts into vectors, in input order, by calling the embedding backend
// directly rather than through Meilisearch's embedder integration (the loopback TEI URI
// is unreachable from the engine, and this keeps every caller in one vector space).
// Inputs are split to the backend's batch limit with up to embedConcurrency calls in
// flight, which a remote GPU endpoint needs to hide latency.
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

	// One slot per chunk keeps the result in input order whatever the completion order.
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

// embedChunk embeds one TEI-sized batch in input order, retrying transient failures with
// exponential backoff. A deterministic error (4xx, malformed response) or a cancelled
// parent context fails fast.
func (c *Client) embedChunk(ctx context.Context, inputs []string) ([][]float64, error) {
	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("search: embed marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < embedMaxAttempts; attempt++ {
		if attempt > 0 {
			// Abort the backoff if the parent context is done (a sibling chunk failed).
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

// embedOnce makes one embedding call and reports whether a failure is retriable. It
// speaks TEI's `{"inputs": [...]}` shape, which both the TEI host and an HF Inference
// Endpoint accept. Over-long inputs are truncated server-side, so none is handled here.
//
// Transport errors, 429 and 5xx are retriable; a 4xx or a wrong vector count is not; a
// cancelled parent context is terminal.
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
		// A cancelled parent is terminal; any other failure, including this attempt's own
		// timeout, is transient.
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
		// A body cut short mid-read is a broken connection.
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

// parseEmbeddings decodes a bare array of vectors (TEI /embed) or the
// `{"embeddings": [...]}` form (HF Inference Endpoints).
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

// toFloat32 narrows a vector to the float32 the pgvector columns store.
func toFloat32(v []float64) []float32 {
	f := make([]float32, len(v))
	for i, x := range v {
		f[i] = float32(x)
	}
	return f
}
