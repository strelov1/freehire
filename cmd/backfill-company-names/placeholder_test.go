package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestResolvePlaceholders(t *testing.T) {
	registry := companyname.Registry{
		"ukgready": stubResolver{byBoard: map[string]string{
			"secure.saashr.com/oma": "Oceanside Museum Of Art", // accepted
			"secure.saashr.com/dud": "",                        // the ATS named nothing
		}},
	}
	rows := []db.ListPlaceholderCompanyBoardsRow{
		{Provider: "ukgready", Board: "secure.saashr.com/oma", Company: "secure.saashr.com/oma"},
		{Provider: "ukgready", Board: "secure.saashr.com/dud", Company: "secure.saashr.com/dud"},
		{Provider: "taleo", Board: "x.taleo.net/2", Company: "x.taleo.net/2"}, // no resolver
	}

	renames, unresolved := resolvePlaceholders(context.Background(), rows, registry)

	want := placeholderRename{provider: "ukgready", board: "secure.saashr.com/oma", oldName: "secure.saashr.com/oma", name: "Oceanside Museum Of Art"}
	if len(renames) != 1 || renames[0] != want {
		t.Fatalf("renames = %+v, want [%+v]", renames, want)
	}
	if unresolved != 2 {
		t.Errorf("unresolved = %d, want 2 (dud, taleo)", unresolved)
	}
}

// fakePlaceholderWriter records the writes writePlaceholderRename makes, in order.
type fakePlaceholderWriter struct {
	calls     []string
	alias     map[string]string // folded key -> canonical slug
	boardRows int64
}

func (f *fakePlaceholderWriter) ResolveCompanySlugAliases(_ context.Context, keys []string) ([]db.ResolveCompanySlugAliasesRow, error) {
	var out []db.ResolveCompanySlugAliasesRow
	for _, k := range keys {
		if c, ok := f.alias[k]; ok {
			out = append(out, db.ResolveCompanySlugAliasesRow{FoldedKey: k, CanonicalSlug: c})
		}
	}
	return out, nil
}

func (f *fakePlaceholderWriter) RenamePlaceholderBoardJobs(_ context.Context, arg db.RenamePlaceholderBoardJobsParams) (int64, error) {
	f.calls = append(f.calls, "jobs "+arg.Source+" "+arg.BoardPattern+" "+arg.OldName+"->"+arg.Name+" ("+arg.NewSlug+")")
	return 3, nil
}

func (f *fakePlaceholderWriter) RenameSlugBoardCompany(_ context.Context, arg db.RenameSlugBoardCompanyParams) (int64, error) {
	f.calls = append(f.calls, "board "+arg.Provider+"/"+arg.Board+" "+arg.OldName+"->"+arg.Name)
	return f.boardRows, nil
}

func (f *fakePlaceholderWriter) InsertCompanySlugAlias(_ context.Context, arg db.InsertCompanySlugAliasParams) (int64, error) {
	f.calls = append(f.calls, "alias "+arg.AliasSlug+"->"+arg.CanonicalSlug+" "+arg.FoldedKey+" "+arg.Reason)
	return 1, nil
}

// The slug is the one the next crawl would write — CompanySlug drops the legal form, and an
// alias the merge worker recorded wins — so the rename does not re-key the company again the
// first time a posting changes.
func TestWritePlaceholderRename(t *testing.T) {
	f := &fakePlaceholderWriter{alias: map[string]string{"coppermooncafes": "copper-moon"}, boardRows: 1}
	r := placeholderRename{provider: "ukgready", board: "secure.saashr.com/1553aps", oldName: "secure.saashr.com/1553aps", name: "Copper Moon Cafes LLC"}

	changed, err := writePlaceholderRename(context.Background(), f, r)
	if err != nil || !changed {
		t.Fatalf("writePlaceholderRename = %v, %v; want true, nil", changed, err)
	}
	want := []string{
		"jobs ukgready secure.saashr.com/1553aps:% secure.saashr.com/1553aps->Copper Moon Cafes LLC (copper-moon)",
		"board ukgready/secure.saashr.com/1553aps secure.saashr.com/1553aps->Copper Moon Cafes LLC",
		// The old page 301s to the new one.
		"alias secure-saashr-com-1553aps->copper-moon securesaashrcom1553aps board_name",
	}
	if len(f.calls) != 3 || f.calls[0] != want[0] || f.calls[1] != want[1] || f.calls[2] != want[2] {
		t.Fatalf("calls = %q, want %q", f.calls, want)
	}
}

// A board renamed by a curator after it was listed matches no row; the write must fail so the
// caller's transaction rolls the jobs half back.
func TestWritePlaceholderRename_BoardChangedMeanwhile(t *testing.T) {
	f := &fakePlaceholderWriter{boardRows: 0}
	r := placeholderRename{provider: "oracle", board: "a.oraclecloud.com/CX", oldName: "a.oraclecloud.com/CX", name: "Acme"}

	if _, err := writePlaceholderRename(context.Background(), f, r); err == nil {
		t.Fatal("writePlaceholderRename: want an error when the board matched no row, got nil")
	}
}
