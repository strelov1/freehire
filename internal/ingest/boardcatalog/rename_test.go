package boardcatalog

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

type fakeJobRenamer struct {
	canonical string // ResolveCompanySlugAliases answer, "" for none
	movedFrom []string
	jobsErr   error
	gotJobs   db.RenamePlaceholderBoardJobsParams
	aliases   []db.InsertCompanySlugAliasParams
}

func (f *fakeJobRenamer) ResolveCompanySlugAliases(context.Context, []string) ([]db.ResolveCompanySlugAliasesRow, error) {
	if f.canonical == "" {
		return nil, nil
	}
	return []db.ResolveCompanySlugAliasesRow{{CanonicalSlug: f.canonical}}, nil
}

func (f *fakeJobRenamer) RenamePlaceholderBoardJobs(_ context.Context, p db.RenamePlaceholderBoardJobsParams) ([]string, error) {
	f.gotJobs = p
	return f.movedFrom, f.jobsErr
}

func (f *fakeJobRenamer) InsertCompanySlugAlias(_ context.Context, p db.InsertCompanySlugAliasParams) (int64, error) {
	f.aliases = append(f.aliases, p)
	return 1, nil
}

func boardWrote(n int64) func(context.Context) (int64, error) {
	return func(context.Context) (int64, error) { return n, nil }
}

func TestRenameBoardJobsMovesJobsAndAliasesOnlyWhenAsked(t *testing.T) {
	r := BoardRename{Provider: "oracle", Board: "h.example/CX_1", OldName: "h.example/CX_1", Name: "Pella"}
	for _, alias := range []bool{false, true} {
		f := &fakeJobRenamer{movedFrom: []string{"h-example-cx-1", "b", "h-example-cx-1"}}
		r.AliasOldSlug = alias
		slug, old, err := RenameBoardJobs(context.Background(), f, r, boardWrote(1))
		if err != nil {
			t.Fatalf("alias=%t: %v", alias, err)
		}
		if slug != "pella" || f.gotJobs.NewSlug != "pella" || f.gotJobs.BoardPattern != externalid.BoardPattern(r.Board) || f.gotJobs.OldName != r.OldName {
			t.Fatalf("alias=%t: slug %q, jobs params %+v", alias, slug, f.gotJobs)
		}
		if want := []string{"b", "h-example-cx-1"}; !reflect.DeepEqual(old, want) {
			t.Fatalf("alias=%t: old slugs %v, want sorted distinct %v", alias, old, want)
		}
		if got := len(f.aliases); (got == 1) != alias {
			t.Fatalf("alias=%t: %d alias row(s) written", alias, got)
		}
	}
}

func TestRenameBoardJobsUsesTheCanonicalSlug(t *testing.T) {
	f := &fakeJobRenamer{canonical: "pella-corporation"}
	slug, _, err := RenameBoardJobs(context.Background(), f, BoardRename{Provider: "oracle", Board: "h/x", OldName: "Old", Name: "Pella"}, boardWrote(1))
	if err != nil || slug != "pella-corporation" || f.gotJobs.NewSlug != "pella-corporation" {
		t.Fatalf("slug %q, jobs slug %q, err %v; want the alias registry's canonical slug", slug, f.gotJobs.NewSlug, err)
	}
}

func TestRenameBoardJobsFailsWhenTheBoardDidNotChange(t *testing.T) {
	f := &fakeJobRenamer{}
	if _, _, err := RenameBoardJobs(context.Background(), f, BoardRename{Provider: "oracle", Board: "h/x", OldName: "Old", Name: "New", AliasOldSlug: true}, boardWrote(0)); err == nil {
		t.Fatal("a board write of 0 rows succeeded; it must fail so the jobs half rolls back")
	}
	if len(f.aliases) != 0 {
		t.Fatal("an alias was written although the board write failed")
	}
}

func TestRenameBoardJobsReportsAJobsError(t *testing.T) {
	f := &fakeJobRenamer{jobsErr: errors.New("boom")}
	if _, _, err := RenameBoardJobs(context.Background(), f, BoardRename{Provider: "oracle", Board: "h/x", OldName: "Old", Name: "New"}, boardWrote(1)); err == nil {
		t.Fatal("a failed jobs move was not reported")
	}
}
