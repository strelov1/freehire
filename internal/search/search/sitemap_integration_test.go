//go:build integration

// Integration test for the sitemap readers' scope against a real Meilisearch: the job
// sitemap names TECH postings only, and the company sitemap is untouched by that
// filter. The filter expression is evaluated by the engine, so only a real one can
// prove it selects what it claims. Run with:
//
//	go test -tags=integration ./internal/search/search/
//
// Requires Docker (reuses startMeili from search_integration_test.go).
package search

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobreality"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// indexJobsForSitemap maps rows through FromJob and indexes them, attaching each
// document's reality class the way cmd/reindex does — the classifier needs a clock and
// role-cluster counts FromJob has no access to, so the caller sets it. A row whose
// class is empty gets no reality object at all, which is how a document indexed before
// the signal existed looks.
func indexJobsForSitemap(t *testing.T, c *Client, jobs []db.Job, classes []string) {
	t.Helper()
	ctx := context.Background()
	docs := make([]JobDocument, 0, len(jobs))
	for i, j := range jobs {
		d, err := FromJob(j)
		if err != nil {
			t.Fatalf("FromJob(%d): %v", j.ID, err)
		}
		if classes[i] != "" {
			d.Reality = &jobview.Reality{Class: classes[i]}
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}
}

// TestIntegration_JobSitemapScope locks the sitemap's scope spec scenarios against a
// real engine: of four indexed, findable postings only the plain tech one is named.
// The non_tech and the unclassified postings stay INDEXED and searchable — this is a
// sitemap-scope filter, not an indexing one — which is why the assertion is about what
// ListSitemapPage returns and not about what the index holds.
func TestIntegration_JobSitemapScope(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	yes := pgtype.Bool{Bool: true, Valid: true}
	no := pgtype.Bool{Bool: false, Valid: true}
	unknown := pgtype.Bool{} // NULL — jobview omits the facet, so it filters as absent

	jobs := []db.Job{
		{ID: 1, Title: "Senior Go Engineer", Company: "Acme", PublicSlug: "tech-fresh", Category: "backend", IsTech: yes},
		{ID: 2, Title: "Staff Platform Engineer", Company: "Acme", PublicSlug: "tech-stale", Category: "backend", IsTech: yes},
		{ID: 3, Title: "Warehouse Cleaner", Company: "Beta", PublicSlug: "non-tech", IsTech: no},
		{ID: 4, Title: "Yard Coordinator", Company: "Gamma", PublicSlug: "unclassified", IsTech: unknown},
		{ID: 5, Title: "Backend Engineer", Company: "Delta", PublicSlug: "tech-evergreen", Category: "backend", IsTech: yes},
	}
	classes := []string{
		jobreality.ClassFresh,
		// stale is "everything else", not a liveness verdict — an ordinary open posting
		// more than a fortnight old. It must still be named, which is the whole reason
		// the filter excludes likely-evergreen and not this.
		jobreality.ClassStale,
		jobreality.ClassFresh,
		jobreality.ClassFresh,
		jobreality.ClassLikelyEvergreen,
	}
	indexJobsForSitemap(t, c, jobs, classes)

	t.Run("names tech postings, fresh and stale alike", func(t *testing.T) {
		docs, total, err := c.ListSitemapPage(ctx, 0, 100)
		if err != nil {
			t.Fatalf("ListSitemapPage: %v", err)
		}
		got := make(map[string]bool, len(docs))
		for _, d := range docs {
			got[d.Slug] = true
		}
		for _, want := range []string{"tech-fresh", "tech-stale"} {
			if !got[want] {
				t.Errorf("sitemap should name %q, got %v", want, got)
			}
		}
		for _, unwanted := range []string{"non-tech", "unclassified", "tech-evergreen"} {
			if got[unwanted] {
				t.Errorf("sitemap should not name %q, got %v", unwanted, got)
			}
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
	})

	// The sitemap index tiles by this count and its chunks are served by the reader
	// above. If the two ever disagree, /sitemap.xml names a chunk that comes back
	// empty — so the agreement is the property under test, not the number.
	t.Run("the count matches what the pages serve", func(t *testing.T) {
		total, err := c.CountSitemapDocuments(ctx)
		if err != nil {
			t.Fatalf("CountSitemapDocuments: %v", err)
		}
		docs, pageTotal, err := c.ListSitemapPage(ctx, 0, 100)
		if err != nil {
			t.Fatalf("ListSitemapPage: %v", err)
		}
		if total != pageTotal || total != int64(len(docs)) {
			t.Errorf("count = %d, page total = %d, page len = %d — all three must agree",
				total, pageTotal, len(docs))
		}
	})

	// An offset past the end is what a crawler holding an older sitemap index asks
	// for, and it must be an empty page rather than an error.
	t.Run("an offset past the end is an empty page", func(t *testing.T) {
		docs, total, err := c.ListSitemapPage(ctx, 1000, 100)
		if err != nil {
			t.Fatalf("ListSitemapPage past the end: %v", err)
		}
		if len(docs) != 0 {
			t.Errorf("docs past the end = %v, want none", docs)
		}
		if total != 2 {
			t.Errorf("total past the end = %d, want the unchanged 2", total)
		}
	})
}

// TestIntegration_CompanySitemapUnfiltered proves the job filter does not reach the
// company reader. The two share sitemapPage, so a filter applied there rather than at
// the job caller would silently empty the company sitemap — `is_tech` is not an
// attribute of a company document, and Meilisearch rejects a filter naming an
// undeclared attribute, so the failure would be a 400 surfacing as a 500 on
// /sitemap.xml rather than anything a reader would guess from this change.
func TestIntegration_CompanySitemapUnfiltered(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	buildCompanyIndex(t, c, []CompanyDocument{
		{Slug: "acme", Name: "Acme", JobCount: 3},
		{Slug: "beta", Name: "Beta", JobCount: 1},
	})

	docs, total, err := c.ListCompanySitemapPage(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ListCompanySitemapPage: %v", err)
	}
	if total != 2 || len(docs) != 2 {
		t.Errorf("company sitemap = %d docs (total %d), want both companies", len(docs), total)
	}

	count, err := c.CountCompanySitemapDocuments(ctx)
	if err != nil {
		t.Fatalf("CountCompanySitemapDocuments: %v", err)
	}
	if count != 2 {
		t.Errorf("company count = %d, want 2", count)
	}
}

// TestIntegration_FreshJobSitemapOrder is the one test that can prove the ordering
// guarantee web-ssr-seo states, and the reason it needs a real engine is the defect it
// guards: the paged reader's endpoint accepts no sort at all, so "is this index
// actually sortable by created_at, and does the sort yield chronological order" is a
// question only Meilisearch can answer. The unit tests beside this one assert that the
// REQUEST carries the sort — a stub can return anything in any order, so they cannot
// observe a sorted RESULT.
//
// The dates are deliberately not in insertion order: indexing oldest-first would let a
// reader with no sort at all pass by accident, which is exactly the accident that
// shipped.
func TestIntegration_FreshJobSitemapOrder(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	yes := pgtype.Bool{Bool: true, Valid: true}
	at := func(day int) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC), Valid: true}
	}

	jobs := []db.Job{
		{ID: 1, Title: "Backend Engineer", Company: "Acme", PublicSlug: "middle", Category: "backend", IsTech: yes, CreatedAt: at(10)},
		{ID: 2, Title: "Platform Engineer", Company: "Acme", PublicSlug: "oldest", Category: "backend", IsTech: yes, CreatedAt: at(1)},
		{ID: 3, Title: "Site Reliability Engineer", Company: "Beta", PublicSlug: "newest", Category: "backend", IsTech: yes, CreatedAt: at(20)},
	}
	indexJobsForSitemap(t, c, jobs, []string{
		jobreality.ClassFresh, jobreality.ClassFresh, jobreality.ClassFresh,
	})

	t.Run("names the newest first", func(t *testing.T) {
		docs, err := c.ListFreshSitemapPage(ctx, 0, 100)
		if err != nil {
			t.Fatalf("ListFreshSitemapPage: %v", err)
		}
		got := make([]string, len(docs))
		for i, d := range docs {
			got[i] = d.Slug
		}
		want := []string{"newest", "middle", "oldest"}
		if len(got) != len(want) {
			t.Fatalf("fresh sitemap = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("fresh sitemap = %v, want %v", got, want)
			}
		}
	})

	// The fresh reader must narrow to the same population the paged one does. Proving
	// it against the engine rather than against the filter string is what catches a
	// filter that parses but selects differently under a sort.
	t.Run("applies the same scope as the paged reader", func(t *testing.T) {
		nonTech := db.Job{ID: 4, Title: "Warehouse Cleaner", Company: "Gamma", PublicSlug: "non-tech-fresh", IsTech: pgtype.Bool{Bool: false, Valid: true}, CreatedAt: at(21)}
		indexJobsForSitemap(t, c, []db.Job{nonTech}, []string{jobreality.ClassFresh})

		docs, err := c.ListFreshSitemapPage(ctx, 0, 100)
		if err != nil {
			t.Fatalf("ListFreshSitemapPage: %v", err)
		}
		for _, d := range docs {
			if d.Slug == "non-tech-fresh" {
				t.Fatalf("fresh sitemap named a non-tech posting, even though it is the newest: %v", docs)
			}
		}
	})

	// A crawler holding a stale sitemap index asks for an offset the window no longer
	// reaches; that must be an empty page, not an error.
	t.Run("an offset past the end is an empty page", func(t *testing.T) {
		docs, err := c.ListFreshSitemapPage(ctx, 1000, 100)
		if err != nil {
			t.Fatalf("ListFreshSitemapPage past the end: %v", err)
		}
		if len(docs) != 0 {
			t.Errorf("past the end = %d docs, want 0", len(docs))
		}
	})
}
