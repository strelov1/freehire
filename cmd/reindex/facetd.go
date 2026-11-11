package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/search/search"
)

// facetdRebuild is the rebuilder over facetd's /rebuild/* endpoints: facetd builds the new
// generation off to the side and swaps it in on commit (Promote). It sends the same
// search.JobDocument values the Meilisearch rebuild gets; facetd ignores _vectors.
type facetdRebuild struct {
	base string
	hc   *http.Client
}

func newFacetdRebuild(baseURL string) *facetdRebuild {
	// commit builds and snapshots a catalogue-sized index in one request
	return &facetdRebuild{base: strings.TrimRight(baseURL, "/"), hc: &http.Client{Timeout: 20 * time.Minute}}
}

func (f *facetdRebuild) Prepare(ctx context.Context) error { return f.post(ctx, "/rebuild/begin", nil) }
func (f *facetdRebuild) Promote(ctx context.Context) error {
	return f.post(ctx, "/rebuild/commit", nil)
}
func (f *facetdRebuild) Cleanup(ctx context.Context) error { return f.post(ctx, "/rebuild/abort", nil) }

// Push sends the batch without its vectors: facetd indexes no `_vectors`.
func (f *facetdRebuild) Push(ctx context.Context, docs []search.JobDocument) error {
	slim := make([]search.JobDocument, len(docs))
	for i, d := range docs {
		d.Vectors = search.DocumentVectors{}
		slim[i] = d
	}
	return f.post(ctx, "/rebuild/push", map[string]any{"docs": slim})
}

func (f *facetdRebuild) post(ctx context.Context, path string, body any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.hc.Do(req)
	if err != nil {
		return fmt.Errorf("facetd %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("facetd %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// teeRebuild runs a rebuild on the primary (Meilisearch) and mirrors it to the secondary
// (facetd). The secondary can never fail the primary: its first error is logged, it is
// aborted, and from then on it is skipped for the rest of the run.
type teeRebuild struct {
	primary, secondary rebuilder
	secondaryDown      bool
}

func (t *teeRebuild) secondaryFailed(ctx context.Context, step string, err error) {
	log.Printf("reindex: facetd %s failed, its rebuild is aborted and the Meilisearch reindex continues: %v", step, err)
	t.secondaryDown = true
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	_ = t.secondary.Cleanup(cctx)
}

func (t *teeRebuild) Prepare(ctx context.Context) error {
	if err := t.primary.Prepare(ctx); err != nil {
		return err
	}
	if err := t.secondary.Prepare(ctx); err != nil {
		t.secondaryFailed(ctx, "prepare", err)
	}
	return nil
}

func (t *teeRebuild) Push(ctx context.Context, docs []search.JobDocument) error {
	if err := t.primary.Push(ctx, docs); err != nil {
		return err
	}
	if !t.secondaryDown {
		if err := t.secondary.Push(ctx, docs); err != nil {
			t.secondaryFailed(ctx, "push", err)
		}
	}
	return nil
}

func (t *teeRebuild) Promote(ctx context.Context) error {
	if err := t.primary.Promote(ctx); err != nil {
		return err
	}
	if !t.secondaryDown {
		if err := t.secondary.Promote(ctx); err != nil {
			t.secondaryFailed(ctx, "promote", err)
		}
	}
	return nil
}

func (t *teeRebuild) Cleanup(ctx context.Context) error {
	err := t.primary.Cleanup(ctx)
	if !t.secondaryDown {
		if serr := t.secondary.Cleanup(ctx); serr != nil {
			log.Printf("reindex: facetd cleanup (best-effort): %v", serr)
		}
	}
	return err
}

// retryFacetd rebuilds facetd alone once after it fell out of the tee, and reports whether
// it now holds this run's catalogue. Without it facetd keeps its old generation: the drain
// replays only search_outbox rows, never the derived fields a full rebuild recomputes
// (reality.class, closure geography), so facetd's answers stay stale until the next rebuild.
func retryFacetd(ctx context.Context, t *teeRebuild, retry func(context.Context, rebuilder) error) bool {
	log.Printf("reindex: facetd fell out of the rebuild, rebuilding it alone")
	if err := retry(ctx, t.secondary); err != nil {
		// facetd kept its old generation and never saw these rows' changes; the drain still owes them to it
		log.Printf("reindex: facetd retry failed, search_outbox entries kept for the drain: %v", err)
		return false
	}
	return true
}
