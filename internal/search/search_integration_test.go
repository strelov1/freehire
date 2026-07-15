//go:build integration

// Integration tests for the Meilisearch-backed search package: EnsureIndex
// (settings + embedder), IndexJobs, and Search (keyword, faceted, hybrid). These
// exercise behavior that only a real engine exhibits. Run with:
//
//	go test -tags=integration ./internal/search/
//
// Requires Docker (testcontainers spins up a throwaway Meilisearch). The semantic
// index uses a userProvided embedder, so no model download happens in-engine; the
// vectors come from a stub TEI (fakeTEI) the client is pointed at.
package search

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

// fakeTEI stands in for the embedding server, replying in the wrapped
// {"embeddings": [...]} shape an HF Inference Endpoint uses. It returns a deterministic
// bag-of-words vector per input, so texts sharing tokens land near each other under
// cosine similarity — enough for the semantic assertions (a query hits jobs whose text
// it overlaps) without a real model, and it keeps the vector width at
// embedderDimensions so Meili accepts the userProvided vectors.
func fakeTEI(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Inputs []string `json:"inputs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := struct {
			Embeddings [][]float64 `json:"embeddings"`
		}{}
		for _, s := range in.Inputs {
			out.Embeddings = append(out.Embeddings, bagOfWords(s))
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// bagOfWords hashes each token into a fixed-width count vector — a crude but
// deterministic embedding whose cosine similarity tracks token overlap.
func bagOfWords(s string) []float64 {
	v := make([]float64, embedderDimensions)
	for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		v[h.Sum32()%embedderDimensions]++
	}
	return v
}

func startMeili(t *testing.T) *Client {
	t.Helper()
	ctx := context.Background()
	const key = "test-master-key"

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "getmeili/meilisearch:v1.13",
			ExposedPorts: []string{"7700/tcp"},
			Env:          map[string]string{"MEILI_MASTER_KEY": key, "MEILI_ENV": "development"},
			WaitingFor:   wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start meilisearch: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := container.MappedPort(ctx, "7700")
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	c := NewClient("http://"+host+":"+port.Port(), key)
	c.embedURL = fakeTEI(t)
	return c
}

func enrichedJSON(t *testing.T, e enrich.Enrichment) []byte {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal enrichment: %v", err)
	}
	return raw
}

func TestIntegration_EnsureIndexIndexAndSearch(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 1, Title: "Senior Golang Engineer", Company: "Acme", Location: "Berlin",
			Remote: true, Description: "Build backend services in Go.",
			PublicSlug: "senior-golang-engineer-acme-aaa",
			// seniority/category are dictionary columns (served dict-only).
			Seniority:  "senior",
			Category:   "backend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 2, Title: "Junior Frontend Developer", Company: "Beta", Location: "Remote",
			Remote: true, Description: "React and TypeScript UI work.",
			PublicSlug: "junior-frontend-developer-beta-bbb",
			Seniority:  "junior",
			Category:   "frontend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	t.Run("keyword matches and strips nothing from the document", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{Query: "golang", Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "senior-golang-engineer-acme-aaa" {
			t.Fatalf("keyword search hits = %+v", res.Hits)
		}
		if res.Hits[0].ID != 1 {
			t.Errorf("hit ID = %d, want 1 (kept internally as PK)", res.Hits[0].ID)
		}
	})

	t.Run("facet filter narrows by nested seniority", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{
			Filter: Filter([]string{Eq("enrichment.seniority", "senior")}),
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Hits) != 1 || res.Hits[0].Enrichment.Seniority != "senior" {
			t.Fatalf("filtered hits = %+v", res.Hits)
		}
	})

	t.Run("sort by posted_at string orders chronologically", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{Sort: []string{"posted_at:desc"}, Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Hits) != 2 || res.Hits[0].PublicSlug != "junior-frontend-developer-beta-bbb" {
			t.Fatalf("posted_at:desc order = %+v", res.Hits)
		}
	})

	t.Run("reindex is idempotent", func(t *testing.T) {
		if err := c.IndexJobs(ctx, docs); err != nil {
			t.Fatalf("re-IndexJobs: %v", err)
		}
		res, err := c.Search(ctx, SearchParams{Limit: 100})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 2 {
			t.Errorf("Total after re-index = %d, want 2", res.Total)
		}
	})

	t.Run("deleting a closed job removes it from the index", func(t *testing.T) {
		if err := c.DeleteJobs(ctx, []int64{2}); err != nil {
			t.Fatalf("DeleteJobs: %v", err)
		}
		res, err := c.Search(ctx, SearchParams{Limit: 100})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 1 || res.Hits[0].ID != 1 {
			t.Fatalf("after delete: total=%d hits=%+v, want only job 1", res.Total, res.Hits)
		}
		// Idempotent: deleting an id that is no longer indexed is a no-op.
		if err := c.DeleteJobs(ctx, []int64{2}); err != nil {
			t.Fatalf("re-DeleteJobs: %v", err)
		}
		// Reopened job: indexing it again restores the document.
		if err := c.IndexJobs(ctx, docs[1:2]); err != nil {
			t.Fatalf("re-IndexJobs reopened: %v", err)
		}
		res, err = c.Search(ctx, SearchParams{Limit: 100})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 2 {
			t.Errorf("after reopen: total=%d, want 2", res.Total)
		}
	})

	t.Run("hybrid search hits the separate semantic index", func(t *testing.T) {
		// Semantic search rides a second index built by the --semantic pass (the
		// facet index above carries no embedder), so build and populate it here.
		if err := c.EnsureSemanticIndex(ctx); err != nil {
			t.Fatalf("EnsureSemanticIndex: %v", err)
		}
		if _, err := c.IndexSemanticJobs(ctx, docs); err != nil {
			t.Fatalf("IndexSemanticJobs: %v", err)
		}
		res, err := c.Search(ctx, SearchParams{Query: "backend engineering role", SemanticRatio: 0.5, Limit: 10})
		if err != nil {
			t.Fatalf("hybrid Search: %v", err)
		}
		if len(res.Hits) == 0 {
			t.Error("hybrid search returned no hits")
		}
	})
}

// TestIntegration_EnsureIndexResetsExistingEmbedder guards the merge-semantics
// trap: facetSettings omits the embedder, but a Meilisearch settings update only
// MERGES, so an embedder a prior version put on the `jobs` index would survive and
// keep embedding on every facet reindex. EnsureIndex must reset it explicitly.
func TestIntegration_EnsureIndexResetsExistingEmbedder(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// Put an embedder on the facet index. userProvided needs no model download.
	task, err := c.facet.UpdateEmbeddersWithContext(ctx, map[string]meilisearch.Embedder{
		"manual": {Source: "userProvided", Dimensions: 3},
	})
	if err != nil {
		t.Fatalf("UpdateEmbedders: %v", err)
	}
	if _, err := c.facet.WaitForTaskWithContext(ctx, task.TaskUID, 50*time.Millisecond); err != nil {
		t.Fatalf("await embedder set: %v", err)
	}
	if emb, err := c.facet.GetEmbeddersWithContext(ctx); err != nil || len(emb) == 0 {
		t.Fatalf("precondition: embedder should be set (emb=%v err=%v)", emb, err)
	}

	// EnsureIndex must strip it, leaving the facet index embedder-free.
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex (reset): %v", err)
	}
	emb, err := c.facet.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}
	if len(emb) != 0 {
		t.Errorf("EnsureIndex left embedders on the facet index: %v", emb)
	}
}

func TestSearchFiltersBySkillsFacet(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 10, Title: "Go Engineer", Company: "Acme", Location: "Berlin",
			PublicSlug: "go-engineer-acme-aaa",
			Skills:     []string{"go", "kubernetes"},
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 11, Title: "Python Developer", Company: "Beta", Location: "Remote",
			PublicSlug: "python-developer-beta-bbb",
			Skills:     []string{"python"},
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{
		Filter: Filter([]string{Eq("skills", "go")}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with skills filter: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "go-engineer-acme-aaa" {
		t.Fatalf("skills facet filter hits = %+v, want only go-engineer-acme-aaa", res.Hits)
	}
}

// The "posted within N days" freshness filter works end-to-end: posted_ts is
// indexed as a numeric, filterable attribute, so a Meilisearch range filter built
// from posted_within_days returns only the recent posting and drops the stale one.
func TestSearchFiltersByPostedWithinDays(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	now := time.Now()
	jobs := []db.Job{
		{
			ID: 20, Title: "Fresh Role", Company: "Acme", PublicSlug: "fresh-role-acme",
			PostedAt:   pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 21, Title: "Stale Role", Company: "Beta", PublicSlug: "stale-role-beta",
			PostedAt:   pgtype.Timestamptz{Time: now.Add(-60 * 24 * time.Hour), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{
		Filter: FilterFromValues(vals("posted_within_days=7")),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with posted_within_days filter: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "fresh-role-acme" {
		t.Fatalf("freshness filter hits = %+v, want only fresh-role-acme", res.Hits)
	}
}

// A job carries its company's curated collections as a top-level facet: filtering
// on collections returns only the tagged jobs, and a facet distribution over
// collections reports their counts.
func TestSearchFiltersByCollectionsFacet(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 20, Title: "Founding Engineer", Company: "Stripe", Location: "Remote",
			PublicSlug:  "founding-engineer-stripe-aaa",
			Collections: []string{"yc", "bigtech"},
			PostedAt:    pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment:  enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 21, Title: "Backend Engineer", Company: "Acme", Location: "Remote",
			PublicSlug: "backend-engineer-acme-bbb",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{
		Filter: Filter([]string{Eq("collections", "yc")}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with collections filter: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "founding-engineer-stripe-aaa" {
		t.Fatalf("collections facet filter hits = %+v, want only founding-engineer-stripe-aaa", res.Hits)
	}

	fres, err := c.FacetCounts(ctx, FacetParams{Facets: []string{"collections"}})
	if err != nil {
		t.Fatalf("FacetCounts: %v", err)
	}
	if fres.Facets["collections"]["yc"] != 1 || fres.Facets["collections"]["bigtech"] != 1 {
		t.Errorf("collections dist = %v, want yc:1 bigtech:1", fres.Facets["collections"])
	}
}

// SimilarJobs returns the nearest neighbours of a job from the semantic index:
// other jobs are returned (ranked by embedding proximity), the source job is never
// in its own list, and the caller's limit is honoured.
func TestIntegration_SimilarJobs(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	jobs := []db.Job{
		{ID: 1, Title: "Senior Golang Backend Engineer", Company: "Acme", Location: "Berlin",
			Description: "Build distributed backend services and APIs in Go.",
			PublicSlug:  "senior-golang-backend-engineer-acme-aaa",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 2, Title: "Backend Software Engineer (Go)", Company: "Beta", Location: "Remote",
			Description: "Design server-side microservices and REST APIs in Golang.",
			PublicSlug:  "backend-software-engineer-go-beta-bbb",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 3, Title: "Frontend React Developer", Company: "Gamma", Location: "Remote",
			Description: "Build user interfaces with React and TypeScript.",
			PublicSlug:  "frontend-react-developer-gamma-ccc",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 4, Title: "Data Scientist", Company: "Delta", Location: "London",
			Description: "Train machine learning models and analyse datasets in Python.",
			PublicSlug:  "data-scientist-delta-ddd",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
	}
	if _, err := c.IndexSemanticJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexSemanticJobs: %v", err)
	}

	t.Run("returns neighbours and excludes the source job", func(t *testing.T) {
		hits, err := c.SimilarJobs(ctx, 1, 10)
		if err != nil {
			t.Fatalf("SimilarJobs: %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("SimilarJobs returned no neighbours")
		}
		for _, h := range hits {
			if h.ID == 1 {
				t.Errorf("source job id 1 must not appear in its own similar list: %+v", hits)
			}
		}
	})

	t.Run("honours the limit", func(t *testing.T) {
		hits, err := c.SimilarJobs(ctx, 1, 2)
		if err != nil {
			t.Fatalf("SimilarJobs: %v", err)
		}
		if len(hits) > 2 {
			t.Errorf("limit 2 returned %d hits", len(hits))
		}
	})

	// The semantic index is built incrementally, so a job can exist in Postgres
	// (and thus reach this call) without a vector in the index yet. Meilisearch
	// answers that with a 400 not_found_similar_id, which must degrade to an empty
	// list — not a hard error — so the detail page just hides the section.
	t.Run("source absent from the index yields empty, not an error", func(t *testing.T) {
		hits, err := c.SimilarJobs(ctx, 9999, 10) // never indexed
		if err != nil {
			t.Fatalf("SimilarJobs for an unindexed id must not error: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no neighbours for an unindexed id, got %d", len(hits))
		}
	})

	// The semantic index may be dropped entirely to reclaim disk while its rebuild
	// is paused. Meilisearch then answers with index_not_found, which must degrade
	// to an empty list — exactly like a missing source — so the detail page hides
	// the section rather than 500ing. Runs last: it removes the shared index.
	t.Run("absent semantic index yields empty, not an error", func(t *testing.T) {
		if err := c.dropIndex(ctx, semanticIndexUID); err != nil {
			t.Fatalf("dropIndex: %v", err)
		}
		hits, err := c.SimilarJobs(ctx, 1, 10)
		if err != nil {
			t.Fatalf("SimilarJobs against an absent index must not error: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no neighbours when the index is absent, got %d", len(hits))
		}
	})
}

// TestIntegration_EmbedTextAndRecommend covers the whole /my/recommendations path
// end-to-end against a real engine: a CV is embedded (EmbedText, the query side) into
// the same space as the indexed jobs (the passage side), and RecommendByVector ranks
// the corpus by that vector. This is the path a unit test cannot reach — the PR that
// shipped EmbedText broke on exactly this gap — so it must hit real Meili + the stub
// embedder. A backend-flavoured CV must surface the backend jobs, not the frontend one.
func TestIntegration_EmbedTextAndRecommend(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}
	jobs := []db.Job{
		{ID: 1, Title: "Senior Golang Backend Engineer", Company: "Acme", Location: "Berlin",
			Description: "Build distributed backend services and REST APIs in Go.",
			PublicSlug:  "senior-golang-backend-engineer-acme-aaa",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 2, Title: "Backend Software Engineer", Company: "Beta", Location: "Remote",
			Description: "Design server-side backend microservices and REST APIs.",
			PublicSlug:  "backend-software-engineer-beta-bbb",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 3, Title: "Frontend React Developer", Company: "Gamma", Location: "Remote",
			Description: "Build user interfaces with React and TypeScript.",
			PublicSlug:  "frontend-react-developer-gamma-ccc",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
	}
	if _, err := c.IndexSemanticJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexSemanticJobs: %v", err)
	}

	vec, model, err := c.EmbedText(ctx, "Backend engineer building Go services and REST APIs.")
	if err != nil {
		t.Fatalf("EmbedText: %v", err)
	}
	if len(vec) != embedderDimensions {
		t.Fatalf("EmbedText vector width = %d, want %d", len(vec), embedderDimensions)
	}
	if model != embedderModel {
		t.Errorf("EmbedText model = %q, want %q", model, embedderModel)
	}

	res, err := c.RecommendByVector(ctx, vec, nil, 10, 0)
	if err != nil {
		t.Fatalf("RecommendByVector: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("RecommendByVector returned no hits for a CV that overlaps indexed jobs")
	}
	// The backend CV must rank a backend job first, ahead of the React job.
	if top := res.Hits[0].ID; top != 1 && top != 2 {
		t.Errorf("top recommendation id = %d, want a backend job (1 or 2); hits=%+v", top, ids(res.Hits))
	}

	t.Run("filter narrows the ranked set to matching jobs", func(t *testing.T) {
		// A filter passed to the recommend call must constrain the candidates before
		// the vector ranks them: with `id = 1` only the first job survives, even though
		// the CV also overlaps job 2. Proves the filter reaches the semantic search.
		res, err := c.RecommendByVector(ctx, vec, "id = 1", 10, 0)
		if err != nil {
			t.Fatalf("RecommendByVector(filter): %v", err)
		}
		if got := ids(res.Hits); len(got) != 1 || got[0] != 1 {
			t.Errorf("filtered recommend ids = %v, want exactly [1]", got)
		}
	})

	t.Run("empty vector yields empty feed, not an error", func(t *testing.T) {
		empty, err := c.RecommendByVector(ctx, nil, nil, 10, 0)
		if err != nil {
			t.Fatalf("RecommendByVector(nil): %v", err)
		}
		if len(empty.Hits) != 0 {
			t.Errorf("expected empty feed for an empty vector, got %d hits", len(empty.Hits))
		}
	})
}

func ids(docs []JobDocument) []int64 {
	out := make([]int64, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}
	return out
}

func toDocs(t *testing.T, jobs []db.Job) []JobDocument {
	t.Helper()
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	return docs
}

// A full rebuild builds a brand-new index and swaps it over the live one in one
// atomic step: documents only in the OLD set must vanish (a wholesale replace, not
// a merge), the fresh set must be searchable, and the throwaway rebuild index must
// be dropped afterwards.
func TestIntegration_RebuildSwapsFreshIndexAndDropsOld(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	// Seed the live index with an OLD set: ids 1 and 2.
	old := toDocs(t, []db.Job{
		{ID: 1, Title: "Old One", PublicSlug: "old-one-aaa", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
		{ID: 2, Title: "Old Two", PublicSlug: "old-two-bbb", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
	})
	if err := c.IndexJobs(ctx, old); err != nil {
		t.Fatalf("IndexJobs (seed): %v", err)
	}

	// Rebuild with a DIFFERENT set: ids 2 and 3 (id 1 dropped, id 3 new).
	r := c.NewFacetRebuild()
	if err := r.Prepare(ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	fresh := toDocs(t, []db.Job{
		{ID: 2, Title: "Two", PublicSlug: "two-bbb", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
		{ID: 3, Title: "Three", PublicSlug: "three-ccc", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
	})
	if err := r.Push(ctx, fresh); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := r.Promote(ctx); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{Limit: 100})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := map[int64]bool{}
	for _, h := range res.Hits {
		got[h.ID] = true
	}
	if got[1] {
		t.Error("id 1 still present; the swap should have dropped the old-only doc")
	}
	if !got[2] || !got[3] {
		t.Errorf("fresh set missing: got ids %v, want 2 and 3", got)
	}
	if len(res.Hits) != 2 {
		t.Errorf("hit count = %d, want 2 (the fresh set only)", len(res.Hits))
	}

	if _, err := c.manager.GetIndexWithContext(ctx, "jobs_rebuild"); err == nil {
		t.Error("jobs_rebuild still exists; Promote should drop it")
	}
}

// A --from-pg semantic rebuild rehydrates the index from the vectors persisted in
// Postgres (jobs.semantic_embedding) instead of re-embedding via TEI: a job carrying a
// stored vector lands in jobs_semantic with that exact vector, and a job without one is
// left out (for the embed worker to fill). This is the disaster-recovery path.
func TestIntegration_SemanticRebuildFromPG(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	vec := func(seed float32) []float32 {
		v := make([]float32, embedderDimensions)
		for i := range v {
			v[i] = seed
		}
		return v
	}
	jobs := []db.Job{
		{ID: 1, Title: "Senior Golang Engineer", Company: "Acme", Location: "Berlin",
			Description: "Build backend services in Go.",
			PublicSlug:  "senior-golang-engineer-acme-aaa",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{}),
			// 0.5 is exactly representable in float32, so the round-trip is exact.
			SemanticEmbedding: vec(0.5)},
		{ID: 2, Title: "Frontend Developer", Company: "Beta", Location: "Remote",
			Description: "Build UIs in React.",
			PublicSlug:  "frontend-developer-beta-bbb",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})}, // no persisted vector
	}

	b := c.NewSemanticRebuildFromPG()
	if err := b.Prepare(ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := b.Push(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := b.Promote(ctx); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	got, err := c.ListSemanticVectors(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ListSemanticVectors: %v", err)
	}
	byID := make(map[int64][]float32, len(got))
	for _, v := range got {
		byID[v.ID] = v.Vector
	}
	if v, ok := byID[1]; !ok {
		t.Fatal("job 1 missing from jobs_semantic after from-pg rebuild")
	} else if len(v) != embedderDimensions || v[0] != 0.5 {
		t.Errorf("job 1 vector[0] = %v (len %d); want 0.5 (len %d)", v[0], len(v), embedderDimensions)
	}
	if _, ok := byID[2]; ok {
		t.Error("job 2 has no persisted vector and must be absent from the rehydrated index")
	}
}
