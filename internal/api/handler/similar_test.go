package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeSimilarReader struct {
	owner int64
	rows  []db.Job
}

func (f fakeSimilarReader) GetJobDuplicateOwner(context.Context, int64) (int64, error) {
	return f.owner, nil
}

func (f fakeSimilarReader) GetJobsByIDs(context.Context, []int64) ([]db.Job, error) {
	out := slices.Clone(f.rows)
	slices.Reverse(out)
	return out, nil
}

type similarSearch struct {
	calls      []search.SearchParams
	keyword    search.SearchResult
	keywordErr error
}

func (f *similarSearch) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.calls = append(f.calls, p)
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

// Similar jobs is one keyword search under similarParams' filters; its failure is the
// caller's.
func TestSimilarViewsSearchesByKeyword(t *testing.T) {
	reader := fakeSimilarReader{rows: []db.Job{{ID: 2, PublicSlug: "keyword"}}}
	idx := &similarSearch{keyword: search.SearchResult{Hits: []search.JobDocument{{ID: 2}}}}
	views, err := similarViews(t.Context(), idx, reader, similarSource(), 6, visiblePublic)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].PublicSlug != "keyword" || len(idx.calls) != 1 {
		t.Fatalf("views = %v after %d searches, want the keyword hit from one search", views, len(idx.calls))
	}
	if idx.calls[0].Query != "Go Developer" || idx.calls[0].Vector != nil {
		t.Errorf("request = %+v, want the source title as a plain keyword query", idx.calls[0])
	}

	idx = &similarSearch{keywordErr: errors.New("Meili unavailable")}
	if _, err := similarViews(t.Context(), idx, reader, similarSource(), 6, visiblePublic); err == nil {
		t.Fatal("hidden keyword error")
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
	views, err := similarViews(t.Context(), idx, fakeSimilarReader{rows: rows}, similarSource(), 6, visiblePublic)
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
	got, err := similarViews(t.Context(), idx, reader, source, 6, visiblePublic)
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
	got, err = similarViews(t.Context(), idx, reader, source, 6, visiblePublic)
	if err != nil || len(got) != 0 || len(idx.calls) != 0 {
		t.Fatalf("unresolved ownership searched: %v %v", got, err)
	}
}
