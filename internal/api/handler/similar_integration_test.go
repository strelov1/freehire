//go:build integration

package handler

import (
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// Exercise real slug/embedding/visibility reads with a fake search engine. Engine
// hybrid/filter serialization is covered by the search package's integration tests.
func TestSimilarJobsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := t.Context()
	for _, slug := range []string{"source", "other", "private", "closed"} {
		_, err := pool.Exec(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, category, countries, cities, is_private, closed_at)
   VALUES ('greenhouse', $1, 'https://example.test', 'Go Developer', $1, 'backend', '{de}', '{Berlin}', $1 = 'private', CASE WHEN $1 = 'closed' THEN now() END)`, slug)
		if err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(pool)
	row, err := q.GetJobBySlug(ctx, "source")
	if err != nil {
		t.Fatal(err)
	}
	var hits []search.JobDocument
	for _, slug := range []string{"private", "closed", "other", "source"} {
		job, err := q.GetJobBySlug(ctx, slug)
		if err != nil {
			t.Fatal(err)
		}
		hits = append(hits, search.JobDocument{ID: job.ID})
	}
	idx := &similarSearch{keyword: search.SearchResult{Hits: hits}, hybrid: search.SearchResult{Hits: hits}}
	h := &searchHandlers{search: idx, queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/:slug/similar", h.SimilarJobs)
	status, body := doGet(t, app, "/jobs/source/similar?limit=1")
	if status != 200 {
		t.Fatalf("status=%d body=%v", status, body)
	}
	data := body["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["public_slug"] != "other" {
		t.Fatalf("data=%v", data)
	}
	// A stored vector enables hybrid ranking without a query-time embedding call.
	_, err = pool.Exec(ctx, `INSERT INTO job_semantic_chunks (job_id, chunk_index, embedding) VALUES ($1,0,array_fill(0.1::real,ARRAY[768])::vector)`, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	idx.calls = nil
	status, _ = doGet(t, app, "/jobs/source/similar")
	if status != 200 || len(idx.calls) != 1 || len(idx.calls[0].SemanticVector) != 768 {
		t.Fatalf("hybrid status=%d calls=%v", status, idx.calls)
	}
	status, _ = doGet(t, app, "/jobs/missing/similar")
	if status != 404 {
		t.Fatalf("unknown slug status=%d", status)
	}
	status, _ = doGet(t, app, "/jobs/private/similar")
	if status != 404 {
		t.Fatalf("private slug status=%d, want 404", status)
	}
}

func TestSimilarJobsResolvesTransitiveDuplicateOwner(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := t.Context()
	var parent *int64
	for _, slug := range []string{"owner", "middle", "source"} {
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO jobs (source,external_id,url,title,public_slug,category,countries,duplicate_of_role)
    VALUES ('greenhouse',$1,'https://example.test','Go Developer',$1,'backend','{de}',$2) RETURNING id`, slug, parent).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		parent = &id
	}
	owner, err := q.GetJobBySlug(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	source, err := q.GetJobBySlug(ctx, "source")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := q.GetJobDuplicateOwner(ctx, source.ID)
	if err != nil || resolved != owner.ID {
		t.Fatalf("owner=%d err=%v want %d", resolved, err, owner.ID)
	}
	idx := &similarSearch{keyword: search.SearchResult{Hits: []search.JobDocument{{ID: owner.ID}}}}
	got, err := similarViews(ctx, idx, q, source, 6, 0, visiblePublic)
	if err != nil || len(got) != 0 {
		t.Fatalf("canonical posting recommended: %v %v", got, err)
	}
}
