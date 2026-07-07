package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// teiMaxBatch caps how many inputs go in one TEI /v1/embeddings call. TEI rejects a
// batch above its --max-client-batch-size (default 32); embedBatch chunks larger
// inputs into sequential calls so callers can hand it a whole reindex batch.
const teiMaxBatch = 32

// jobPassage renders a job document into the text embedded for semantic retrieval.
// e5 is asymmetric: the corpus side carries the "passage:" prefix and the query side
// carries "query:" (see EmbedText), so they must be embedded the same way to be
// comparable. This mirrors the document template Meilisearch used to render, now that
// embedding runs in Go (see embedBatch). doc.Description is already capped at index
// time (maxIndexedDescriptionRunes), so this stays within e5's token window.
func jobPassage(d JobDocument) string {
	return "passage: " + d.Title + " at " + d.Company + ". " + d.Description
}

// embedBatch turns texts into vectors by calling TEI's OpenAI-compatible
// /v1/embeddings directly, in input order. We embed here and store the result as a
// userProvided Meilisearch embedder (see jobEmbedder) rather than letting Meili's rest
// embedder reach TEI itself: the engine rejects the loopback TEI URI, and embedding in
// one place keeps the job corpus and the CV query on an identical path (one model, one
// server → one vector space). Inputs are chunked to TEI's per-call batch limit.
func (c *Client) embedBatch(ctx context.Context, inputs []string) ([][]float64, error) {
	out := make([][]float64, 0, len(inputs))
	for start := 0; start < len(inputs); start += teiMaxBatch {
		end := start + teiMaxBatch
		if end > len(inputs) {
			end = len(inputs)
		}
		vecs, err := c.embedChunk(ctx, inputs[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// embedChunk embeds one TEI-sized batch and returns the vectors in input order. It
// speaks TEI's native `/embed` shape — `{"inputs": [...]}` in, an array of vectors out
// — which every backend we target accepts: the host2 TEI (/embed, bare array) and an
// HF Inference Endpoint (root, `{"embeddings": [...]}`). Over-long inputs (e5 caps at
// 512 tokens) are truncated server-side (host2 TEI's --auto-truncate; HF truncates by
// default), so no per-input length handling is needed here.
func (c *Client) embedChunk(ctx context.Context, inputs []string) ([][]float64, error) {
	body, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("search: embed marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.embedURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("search: embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.embedKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.embedKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: embed call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search: embed: unexpected status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("search: embed read: %w", err)
	}
	vecs, err := parseEmbeddings(raw)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(inputs) {
		return nil, fmt.Errorf("search: embed: got %d vectors for %d inputs", len(vecs), len(inputs))
	}
	return vecs, nil
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
