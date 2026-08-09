package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SemanticVector is one job's persisted chunk vectors read back from the semantic
// index — the (job id, vectors) pair the Postgres backfill needs so the durable copy
// of the embeddings can live beside the jobs they belong to. A job carries one vector
// per chunk of its description (see chunkText); ALL of them are kept, not just the
// first, so this stays a faithful copy of what Meilisearch actually holds.
type SemanticVector struct {
	ID      int64
	Vectors [][]float32
}

// ListSemanticVectors reads one offset-paged window of (id, vector) pairs from the
// semantic index, fetching only the id and the stored vector. It is the read side of
// the Postgres backfill: these vectors already exist in Meili (computed once, at great
// expense), so persisting them costs no embedding. An empty slice means the window is
// past the last document. It uses the raw documents route because the SDK's typed
// query does not expose retrieveVectors on this engine version.
func (c *Client) ListSemanticVectors(ctx context.Context, offset, limit int) ([]SemanticVector, error) {
	url := fmt.Sprintf("%s/indexes/%s/documents?retrieveVectors=true&fields=id&offset=%d&limit=%d",
		c.url, semanticIndexUID, offset, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("search: list semantic vectors request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, err := rawClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: list semantic vectors: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search: list semantic vectors: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("search: read semantic vectors: %w", err)
	}
	return parseSemanticVectorsPage(body)
}

// parseSemanticVectorsPage decodes one page of the `GET /indexes/<uid>/documents?
// retrieveVectors=true` response into (id, chunk vectors) pairs. Documents that carry
// no stored vector are skipped, so the caller only persists real embeddings. ALL of a
// document's embeddings are kept, not just the first — a userProvided doc carries one
// per chunk (see chunkText), and dropping the rest would silently truncate an
// already-chunked job's vectors if this backfill is ever re-run after the fact.
func parseSemanticVectorsPage(raw []byte) ([]SemanticVector, error) {
	var page struct {
		Results []struct {
			ID      int64 `json:"id"`
			Vectors map[string]struct {
				Embeddings [][]float32 `json:"embeddings"`
			} `json:"_vectors"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("search: decode semantic vectors page: %w", err)
	}
	out := make([]SemanticVector, 0, len(page.Results))
	for _, r := range page.Results {
		emb := r.Vectors[embedderName].Embeddings
		if len(emb) == 0 {
			continue // no stored vector — nothing to persist
		}
		out = append(out, SemanticVector{ID: r.ID, Vectors: emb})
	}
	return out, nil
}
