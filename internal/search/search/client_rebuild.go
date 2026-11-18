package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/meilisearch/meilisearch-go"
)

// Rebuild is a fresh-index build session for a full reindex. Documents stream into a
// throwaway index (Push does not wait, so Meilisearch auto-batches the tasks); Promote
// then waits, swaps it over the live index and drops the old one. Search serves the live
// index untouched until that single swap.
type Rebuild struct {
	c          *Client
	liveUID    string
	rebuildUID string
	settings   *meilisearch.Settings
	rebuild    meilisearch.IndexManager
	tasks      []int64
}

// NewFacetRebuild starts a full rebuild of the facet/keyword production index.
func (c *Client) NewFacetRebuild() *Rebuild {
	return &Rebuild{c: c, liveUID: facetIndexUID, rebuildUID: facetRebuildUID, settings: facetSettings()}
}

// Prepare creates an empty rebuild index with this pass's settings. It also ensures the
// live index exists, because the swap needs both (on a first run it is created empty).
func (r *Rebuild) Prepare(ctx context.Context) error {
	if err := r.c.createIndex(ctx, r.c.manager.Index(r.liveUID), r.liveUID, primaryKey); err != nil {
		return err
	}
	// Drop a leftover from an aborted run so the build starts empty.
	if err := r.c.dropIndex(ctx, r.rebuildUID); err != nil {
		return err
	}
	r.rebuild = r.c.manager.Index(r.rebuildUID)
	// ensure() clears inherited embedders first, so the index carries exactly the
	// embedders these settings declare.
	return r.c.ensure(ctx, r.rebuild, r.rebuildUID, primaryKey, r.settings)
}

// Push enqueues a batch without waiting; Promote awaits the kept task uids.
func (r *Rebuild) Push(ctx context.Context, docs []JobDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := primaryKey
	task, err := r.rebuild.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: rebuild push: %w", err)
	}
	r.tasks = append(r.tasks, task.TaskUID)
	return nil
}

// Promote waits for every enqueued batch, swaps the rebuild index over the live one and
// drops the old data.
func (r *Rebuild) Promote(ctx context.Context) error {
	for _, uid := range r.tasks {
		if err := r.c.awaitTask(ctx, r.rebuild, uid); err != nil {
			return err
		}
	}
	if err := r.c.swapIndexes(ctx, r.liveUID, r.rebuildUID); err != nil {
		return err
	}
	// The old data now lives under the rebuild uid.
	return r.c.dropIndex(ctx, r.rebuildUID)
}

// Cleanup drops the rebuild index, tolerating its absence. Deferred so a run that aborts
// before Promote leaves no orphan index eating disk. Idempotent.
func (r *Rebuild) Cleanup(ctx context.Context) error {
	return r.c.dropIndex(ctx, r.rebuildUID)
}

// swapIndexes atomically swaps two indexes and waits for the task. It calls
// POST /swap-indexes directly: the pinned meilisearch-go always sends a `rename` field
// that engine v1.13 rejects.
func (c *Client) swapIndexes(ctx context.Context, a, b string) error {
	payload := []map[string][]string{{"indexes": {a, b}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("search: marshal swap: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/swap-indexes", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("search: swap request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := rawClient.Do(req)
	if err != nil {
		return fmt.Errorf("search: swap indexes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("search: swap indexes: unexpected status %d", resp.StatusCode)
	}
	var task struct {
		TaskUID int64 `json:"taskUid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return fmt.Errorf("search: decode swap task: %w", err)
	}
	return c.awaitManagerTask(ctx, task.TaskUID)
}
