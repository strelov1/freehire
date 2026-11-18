package savedsearch_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/search/savedsearch"
)

func TestSameSelection_IgnoresOrderListFormAndSort(t *testing.T) {
	if !savedsearch.SameSelection("skills=go,rust&company_slug=stripe", "company_slug=stripe&skills=rust&skills=go&sort=posted_at:desc") {
		t.Error("the same selection written two ways must compare equal")
	}
	if savedsearch.SameSelection("skills=go", "skills=go,rust") {
		t.Error("different value sets must not compare equal")
	}
}

func TestFilterSummaryAndDefaultName(t *testing.T) {
	if got := savedsearch.FilterSummary("skills=go,rust&q=backend&company_slug=stripe"); got != `"backend" · company_slug: stripe · skills: go, rust` {
		t.Errorf("summary = %q", got)
	}
	if n := len([]rune(savedsearch.DefaultName("q=" + strings.Repeat("x", 200)))); n != 100 {
		t.Errorf("long name has %d runes, want 100", n)
	}
}

// ensureRepo rejects the first Create as a duplicate name, then accepts.
type ensureRepo struct {
	fakeRepo
	names []string
}

func (r *ensureRepo) Create(_ context.Context, _ int64, name, query string, _ bool) (savedsearch.SavedSearch, error) {
	r.names = append(r.names, name)
	if len(r.names) == 1 {
		return savedsearch.SavedSearch{}, savedsearch.ErrDuplicateName
	}
	return savedsearch.SavedSearch{Name: name, Query: query}, nil
}

func TestEnsureForQuery(t *testing.T) {
	ctx := context.Background()

	t.Run("reuses a saved search selecting the same postings", func(t *testing.T) {
		repo := &ensureRepo{fakeRepo: fakeRepo{listRet: []savedsearch.SavedSearch{{ID: 7, Query: "skills=rust,go"}}}}
		got, err := savedsearch.New(repo).EnsureForQuery(ctx, 1, "", "skills=go,rust")
		if err != nil || got.ID != 7 || len(repo.names) != 0 {
			t.Errorf("got %+v, %v, creates %v; want reuse of 7", got, err, repo.names)
		}
	})

	t.Run("suffixes a colliding generated name", func(t *testing.T) {
		repo := &ensureRepo{fakeRepo: fakeRepo{listRet: []savedsearch.SavedSearch{{ID: 1, Query: "q=other"}}}}
		got, err := savedsearch.New(repo).EnsureForQuery(ctx, 1, "", "skills=go")
		if err != nil || got.Name != "skills: go (2)" {
			t.Errorf("got %+v, %v; want name %q", got, err, "skills: go (2)")
		}
	})

	t.Run("a chosen name that collides is an error", func(t *testing.T) {
		repo := &ensureRepo{}
		_, err := savedsearch.New(repo).EnsureForQuery(ctx, 1, "Mine", "skills=go")
		if !errors.Is(err, savedsearch.ErrDuplicateName) || len(repo.names) != 1 {
			t.Errorf("err = %v, creates %v; want ErrDuplicateName after one try", err, repo.names)
		}
	})
}
