package handler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeSimilarReader struct {
	chunks   []db.JobSemanticChunk
	chunkErr error
	owner    int64
	rows     []db.Job
}

func (f fakeSimilarReader) GetJobDuplicateOwner(context.Context, int64) (int64, error) {
	return f.owner, nil
}

func (f fakeSimilarReader) ListSemanticChunksForJobs(context.Context, []int64) ([]db.JobSemanticChunk, error) {
	return f.chunks, f.chunkErr
}
func (f fakeSimilarReader) GetJobsByIDs(context.Context, []int64) ([]db.Job, error) {
	out := slices.Clone(f.rows)
	slices.Reverse(out)
	return out, nil
}

type similarSearch struct {
	calls      []search.SearchParams
	hybrid     search.SearchResult
	keyword    search.SearchResult
	hybridErr  error
	keywordErr error
}

func (f *similarSearch) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.calls = append(f.calls, p)
	if len(p.SemanticVector) > 0 {
		return f.hybrid, f.hybridErr
	}
	return f.keyword, f.keywordErr
}

func similarSource() db.Job {
	return db.Job{ID: 99, Title: "Go Developer", Category: "backend", Countries: []string{"de"}, Cities: []string{"Berlin"}}
}
func visiblePublic(row db.Job) bool { return !row.ClosedAt.Valid && !row.IsPrivate }

func TestSimilarParamsRequiresSpecializationAndGeography(t *testing.T) {
	for _, missing := range []string{"category", "geography", "other", "blank geography"} {
		row := similarSource()
		switch missing {
		case "category":
			row.Category = ""
		case "other":
			row.Category = "other"
		case "geography":
			row.Countries = nil
			row.Cities = nil
		case "blank geography":
			row.Countries = []string{""}
			row.Cities = nil
		}
		view, err := jobview.FromRow(row)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := similarParams(view, row.ID, 6); ok {
			t.Errorf("searched without %s", missing)
		}
	}
}

func TestSimilarParamsKeepsCountryCityAndSpecializationInSeparateGroups(t *testing.T) {
	view, err := jobview.FromRow(similarSource())
	if err != nil {
		t.Fatal(err)
	}
	params, ok := similarParams(view, 99, 6)
	if !ok {
		t.Fatal("no params")
	}
	groups := params.Filter.([][]string)
	for _, field := range []string{"enrichment.category", "countries", "cities", "id != 99"} {
		found := false
		for _, group := range groups {
			text := strings.Join(group, " OR ")
			if strings.Contains(text, field) {
				found = true
				if field == "countries" && strings.Contains(text, "cities") {
					t.Fatal("country and city were ORed")
				}
			}
		}
		if !found {
			t.Errorf("missing %s in %v", field, groups)
		}
	}
	if params.Query != "Go Developer" {
		t.Errorf("query = %q", params.Query)
	}
}

func TestSimilarViewsHybridAndKeywordFallback(t *testing.T) {
	for _, mode := range []string{"hybrid", "no vectors", "chunk read fails", "hybrid refused", "hybrid empty", "hybrid hidden", "keyword fails"} {
		t.Run(mode, func(t *testing.T) {
			reader := fakeSimilarReader{
				chunks: []db.JobSemanticChunk{{JobID: 99, Embedding: pgvector.NewVector([]float32{1, 0})}},
				rows:   []db.Job{{ID: 1, PublicSlug: "hybrid"}, {ID: 2, PublicSlug: "keyword"}, {ID: 3, IsPrivate: true}},
			}
			idx := &similarSearch{
				hybrid:  search.SearchResult{Hits: []search.JobDocument{{ID: 1}}},
				keyword: search.SearchResult{Hits: []search.JobDocument{{ID: 2}}},
			}
			switch mode {
			case "no vectors":
				reader.chunks = nil
			case "chunk read fails":
				reader.chunkErr = errors.New("db read failed")
			case "hybrid refused":
				idx.hybridErr = search.ErrBadQuery
			case "hybrid empty":
				idx.hybrid.Hits = nil
			case "hybrid hidden":
				idx.hybrid.Hits = []search.JobDocument{{ID: 3}}
			case "keyword fails":
				reader.chunks = nil
				idx.keywordErr = errors.New("Meili unavailable")
			}
			views, err := similarViews(t.Context(), idx, reader, similarSource(), 6, .925, visiblePublic)
			if mode == "keyword fails" {
				if err == nil {
					t.Fatal("hidden keyword error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "keyword"
			if mode == "hybrid" {
				want = "hybrid"
			}
			if len(views) != 1 || views[0].PublicSlug != want {
				t.Fatalf("views = %v, want %s", views, want)
			}
			if len(idx.calls) == 2 {
				a, b := idx.calls[0], idx.calls[1]
				if !reflect.DeepEqual(a.Filter, b.Filter) || a.Query != b.Query {
					t.Fatal("fallback changed scope")
				}
				if b.SemanticVector != nil || b.ScoreThreshold != 0 {
					t.Fatal("keyword fallback retained hybrid parameters")
				}
			}
			if mode == "hybrid" && idx.calls[0].ScoreThreshold != .925 {
				t.Fatal("hybrid lost threshold")
			}
		})
	}
}

func TestSimilarViewsPreservesSearchOrderAndChecksCurrentVisibility(t *testing.T) {
	rows := []db.Job{
		{ID: 1, PublicSlug: "first"}, {ID: 2, PublicSlug: "private", IsPrivate: true},
		{ID: 3, PublicSlug: "closed", ClosedAt: pgtype.Timestamptz{Valid: true}},
		{ID: 4, PublicSlug: "duplicate", DuplicateOf: pgtype.Int8{Valid: true, Int64: 1}},
		{ID: 5, PublicSlug: "unseen", Source: "greenhouse", LastSeenAt: pgtype.Timestamptz{Valid: true, Time: time.Now().Add(-30 * 24 * time.Hour)}},
		{ID: 6, PublicSlug: "second"}, similarSource(),
	}
	hits := []search.JobDocument{{ID: 99}, {ID: 2}, {ID: 6}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 1000}, {ID: 6}, {ID: 1}}
	idx := &similarSearch{keyword: search.SearchResult{Hits: hits}}
	views, err := similarViews(t.Context(), idx, fakeSimilarReader{rows: rows}, similarSource(), 6, 0, visiblePublic)
	if err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, v := range views {
		slugs = append(slugs, v.PublicSlug)
	}
	if fmt.Sprint(slugs) != "[second first]" {
		t.Fatalf("views = %v", slugs)
	}
}

func TestSimilarParamsUsesOnlyKnownLocationWithoutRelaxingIt(t *testing.T) {
	for _, scope := range []string{"country", "city", "region"} {
		t.Run(scope, func(t *testing.T) {
			row := similarSource()
			row.Countries = nil
			row.Cities = nil
			switch scope {
			case "country":
				row.Countries = []string{"de"}
			case "city":
				row.Cities = []string{"Berlin"}
			case "region":
				row.Regions = []string{"eu"}
			}
			view, err := jobview.FromRow(row)
			if err != nil {
				t.Fatal(err)
			}
			params, ok := similarParams(view, row.ID, 6)
			if !ok {
				t.Fatal("known location refused")
			}
			filter := fmt.Sprint(params.Filter)
			want := map[string]string{"country": "countries", "city": "cities", "region": "regions"}[scope]
			if !strings.Contains(filter, want) {
				t.Fatalf("scope %s missing in %s", scope, filter)
			}
		})
	}
}

func TestSimilarViewsExcludesCanonicalOwnerOfDuplicateSource(t *testing.T) {
	source := similarSource()
	source.DuplicateOf = pgtype.Int8{Int64: 10, Valid: true}
	reader := fakeSimilarReader{owner: 1, rows: []db.Job{{ID: 1, PublicSlug: "same-posting"}, {ID: 2, PublicSlug: "another-job"}}}
	idx := &similarSearch{keyword: search.SearchResult{Hits: []search.JobDocument{{ID: 1}, {ID: 2}}}}
	got, err := similarViews(t.Context(), idx, reader, source, 6, 0, visiblePublic)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PublicSlug != "another-job" {
		t.Fatalf("self recommended: %v", got)
	}
	if !strings.Contains(fmt.Sprint(idx.calls[0].Filter), "id != 1") {
		t.Fatal("canonical owner not excluded from search")
	}
	reader.owner = 0 // a cycle or broken chain has no safe canonical owner.
	idx.calls = nil
	got, err = similarViews(t.Context(), idx, reader, source, 6, 0, visiblePublic)
	if err != nil || len(got) != 0 || len(idx.calls) != 0 {
		t.Fatalf("unresolved ownership searched: %v %v", got, err)
	}
}
