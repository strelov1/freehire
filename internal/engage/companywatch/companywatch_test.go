package companywatch

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeQueries answers from fixed tables and records what was written.
type fakeQueries struct {
	companies []db.ListWatchableCompaniesByFoldedSlugRow
	aliases   []db.ResolveCompanySlugAliasesRow
	full      bool

	addedSlug  string
	addedBatch []string
	listArg    db.ListWatchlistParams
}

func (f *fakeQueries) ListWatchlist(_ context.Context, arg db.ListWatchlistParams) ([]db.ListWatchlistRow, error) {
	f.listArg = arg
	at := pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true}
	return []db.ListWatchlistRow{{Slug: "acme", Name: "Acme", AddedAt: at, Total: 7}}, nil
}

func (f *fakeQueries) ListWatchlistSlugs(context.Context, int64) ([]string, error) {
	return []string{"acme"}, nil
}

func (f *fakeQueries) IsWatchingCompany(context.Context, db.IsWatchingCompanyParams) (bool, error) {
	return true, nil
}

func (f *fakeQueries) ResolveWatchableCompany(_ context.Context, slug string) (db.ResolveWatchableCompanyRow, error) {
	for _, c := range f.companies {
		if c.Slug == slug {
			return db.ResolveWatchableCompanyRow{Slug: c.Slug, Name: c.Name}, nil
		}
	}
	return db.ResolveWatchableCompanyRow{}, pgx.ErrNoRows
}

func (f *fakeQueries) AddToWatchlist(_ context.Context, arg db.AddToWatchlistParams) (pgtype.Timestamptz, error) {
	if f.full {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	f.addedSlug = arg.Slug
	return pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true}, nil
}

func (f *fakeQueries) AddManyToWatchlist(_ context.Context, arg db.AddManyToWatchlistParams) (db.AddManyToWatchlistRow, error) {
	f.addedBatch = arg.Slugs
	if f.full {
		return db.AddManyToWatchlistRow{Fresh: int64(len(arg.Slugs))}, nil
	}
	// The first slug is already on the list.
	return db.AddManyToWatchlistRow{Fresh: int64(len(arg.Slugs)) - 1, Inserted: int64(len(arg.Slugs)) - 1}, nil
}

func (f *fakeQueries) RemoveFromWatchlist(context.Context, db.RemoveFromWatchlistParams) error {
	return nil
}

func (f *fakeQueries) ResolveCompanySlugAliases(_ context.Context, keys []string) ([]db.ResolveCompanySlugAliasesRow, error) {
	var out []db.ResolveCompanySlugAliasesRow
	for _, a := range f.aliases {
		for _, k := range keys {
			if a.FoldedKey == k {
				out = append(out, a)
			}
		}
	}
	return out, nil
}

func (f *fakeQueries) ListWatchableCompaniesByFoldedSlug(_ context.Context, arg db.ListWatchableCompaniesByFoldedSlugParams) ([]db.ListWatchableCompaniesByFoldedSlugRow, error) {
	var out []db.ListWatchableCompaniesByFoldedSlugRow
	for _, c := range f.companies {
		for _, k := range arg.FoldedKeys {
			if foldedSlug(c.Slug) == k {
				out = append(out, c)
			}
		}
		for _, s := range arg.Slugs {
			if c.Slug == s && !containsRow(out, c.Slug) {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func foldedSlug(s string) string {
	out := ""
	for _, r := range s {
		if r != '-' {
			out += string(r)
		}
	}
	return out
}

func containsRow(rows []db.ListWatchableCompaniesByFoldedSlugRow, slug string) bool {
	for _, r := range rows {
		if r.Slug == slug {
			return true
		}
	}
	return false
}

func catalogue() *fakeQueries {
	return &fakeQueries{
		companies: []db.ListWatchableCompaniesByFoldedSlugRow{
			{Slug: "acme", Name: "Acme", JobCount: 4},
			{Slug: "closed-co", Name: "Closed Co", JobCount: 0},
			{Slug: "dollar-tree", Name: "Dollar Tree", JobCount: 9},
			{Slug: "vk", Name: "VK", JobCount: 3},
			{Slug: "v-k", Name: "V K", JobCount: 2},
			{Slug: "twin", Name: "Twin", JobCount: 5},
			{Slug: "tw-in", Name: "Tw In", JobCount: 0},
			{Slug: "same", Name: "Same", JobCount: 1},
			{Slug: "sa-me", Name: "Sa Me", JobCount: 1},
		},
		aliases: []db.ResolveCompanySlugAliasesRow{{FoldedKey: "dollartree", CanonicalSlug: "dollar-tree"}},
	}
}

func TestImportResolvesNamesTheWayTheCatalogueDoes(t *testing.T) {
	svc := New(catalogue())
	got, err := svc.Import(context.Background(), 1, []string{
		"Acme Inc.",  // a legal form folds away
		"closed-co",  // a company with no open job still counts, typed as its slug
		"DollarTree", // through the alias registry
		"Twin",       // two namesakes, only one of them hiring
		"VK",         // two namesakes both hiring: undecided
		"Same",       // two namesakes neither of which a hiring one separates
		"Nowhere",    // names nothing
		"  acme  ",   // the same company again, once trimmed
		"",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"acme", "closed-co", "dollar-tree", "twin"}
	f := svc.q.(*fakeQueries)
	if !reflect.DeepEqual(f.addedBatch, want) {
		t.Errorf("written slugs = %v, want %v", f.addedBatch, want)
	}
	wantUnmatched := []string{"VK", "Same", "Nowhere"}
	if !reflect.DeepEqual(got.Unmatched, wantUnmatched) {
		t.Errorf("unmatched = %v, want %v", got.Unmatched, wantUnmatched)
	}
	if got.Added != 3 || got.Already != 1 {
		t.Errorf("added/already = %d/%d, want 3/1", got.Added, got.Already)
	}
}

func TestImportRefusesWhatCannotFit(t *testing.T) {
	svc := New(catalogue())
	if _, err := svc.Import(context.Background(), 1, []string{" ", ""}); !errors.Is(err, ErrNothingToImport) {
		t.Errorf("blank import: err = %v, want ErrNothingToImport", err)
	}
	many := make([]string, MaxEntries+1)
	for i := range many {
		many[i] = "company " + time.Duration(i).String()
	}
	if _, err := svc.Import(context.Background(), 1, many); !errors.Is(err, ErrTooManyToImport) {
		t.Errorf("oversized import: err = %v, want ErrTooManyToImport", err)
	}
	f := catalogue()
	f.full = true
	if _, err := New(f).Import(context.Background(), 1, []string{"acme"}); !errors.Is(err, ErrFull) {
		t.Errorf("import into a full list: err = %v, want ErrFull", err)
	}
}

func TestImportOfOnlyUnmatchedWritesNothing(t *testing.T) {
	f := catalogue()
	got, err := New(f).Import(context.Background(), 1, []string{"Nowhere"})
	if err != nil {
		t.Fatal(err)
	}
	if f.addedBatch != nil || got.Added != 0 || !reflect.DeepEqual(got.Unmatched, []string{"Nowhere"}) {
		t.Errorf("got %+v, wrote %v", got, f.addedBatch)
	}
}

func TestAdd(t *testing.T) {
	f := catalogue()
	svc := New(f)
	e, err := svc.Add(context.Background(), 1, "closed-co")
	if err != nil {
		t.Fatal(err)
	}
	if e.Slug != "closed-co" || e.Name != "Closed Co" || !e.AddedAt.Equal(time.Unix(200, 0)) || f.addedSlug != "closed-co" {
		t.Errorf("entry = %+v, wrote %q", e, f.addedSlug)
	}
	if _, err := svc.Add(context.Background(), 1, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown company: err = %v, want ErrNotFound", err)
	}
	f.full = true
	if _, err := svc.Add(context.Background(), 1, "acme"); !errors.Is(err, ErrFull) {
		t.Errorf("full list: err = %v, want ErrFull", err)
	}
}

func TestListTrimsTheFilterAndReportsTheTotal(t *testing.T) {
	f := catalogue()
	got, total, err := New(f).List(context.Background(), 3, "  ac ", 20, 40)
	if err != nil {
		t.Fatal(err)
	}
	if total != 7 || len(got) != 1 || got[0].Slug != "acme" || !got[0].AddedAt.Equal(time.Unix(100, 0)) {
		t.Errorf("got %+v total %d", got, total)
	}
	if f.listArg.Q != "ac" || f.listArg.PageLimit != 20 || f.listArg.PageOffset != 40 || f.listArg.UserID != 3 {
		t.Errorf("query args = %+v", f.listArg)
	}
}
