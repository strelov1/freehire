package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// embedChunk embeds one TEI-sized batch and returns the vectors in input order.
func (c *Client) embedChunk(ctx context.Context, inputs []string) ([][]float64, error) {
	body, err := json.Marshal(map[string]any{"input": inputs, "model": embedderModel})
	if err != nil {
		return nil, fmt.Errorf("search: embed marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.embedURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("search: embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: embed call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search: embed: unexpected status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("search: embed decode: %w", err)
	}
	if len(out.Data) != len(inputs) {
		return nil, fmt.Errorf("search: embed: got %d vectors for %d inputs", len(out.Data), len(inputs))
	}
	vecs := make([][]float64, len(out.Data))
	for i, d := range out.Data {
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("search: embed: empty vector at %d", i)
		}
		vecs[i] = d.Embedding
	}
	return vecs, nil
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
