package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// placeholderRename is one board whose company was its own board id, with the name its ATS
// states for it.
type placeholderRename struct {
	provider string
	board    string
	oldName  string
	name     string
}

// resolvePlaceholders fetches each placeholder board's name and keeps the ones
// AcceptPlaceholder admits. unresolved counts the boards left as they are — no resolver, a
// failed fetch, or nothing usable — none of which is an error: the board keeps the
// placeholder it already had and the next run asks again. Order-independent; the caller sorts.
func resolvePlaceholders(ctx context.Context, rows []db.ListPlaceholderCompanyBoardsRow, registry companyname.Registry) ([]placeholderRename, int) {
	var (
		resolvable []db.ListPlaceholderCompanyBoardsRow
		unresolved int
	)
	for _, row := range rows {
		if _, ok := registry[row.Provider]; ok {
			resolvable = append(resolvable, row)
		} else {
			unresolved++
		}
	}

	var (
		mu      sync.Mutex
		renames []placeholderRename
	)
	eachPerSource(resolvable, func(row db.ListPlaceholderCompanyBoardsRow) string { return row.Provider }, func(row db.ListPlaceholderCompanyBoardsRow) {
		candidate, err := registry[row.Provider].Name(ctx, row.Board)
		if err != nil {
			// A resolver returns "" beside an error, which AcceptPlaceholder refuses.
			log.Printf("resolve %s %s: %v", row.Provider, row.Board, err)
		}
		name, ok := companyname.AcceptPlaceholder(row.Board, candidate)
		mu.Lock()
		defer mu.Unlock()
		if !ok {
			unresolved++ // a single board failing must not abort the run
			return
		}
		renames = append(renames, placeholderRename{provider: row.Provider, board: row.Board, oldName: row.Company, name: name})
	})

	return renames, unresolved
}

// placeholderWriter is the slice of db.Queries writePlaceholderRename needs.
type placeholderWriter interface {
	ResolveCompanySlugAliases(context.Context, []string) ([]db.ResolveCompanySlugAliasesRow, error)
	RenamePlaceholderBoardJobs(context.Context, db.RenamePlaceholderBoardJobsParams) (int64, error)
	RenameSlugBoardCompany(context.Context, db.RenameSlugBoardCompanyParams) (int64, error)
	InsertCompanySlugAlias(context.Context, db.InsertCompanySlugAliasParams) (int64, error)
}

// writePlaceholderRename applies one resolved name to the board's stored jobs and to the board,
// reporting whether any job changed. The caller runs it in ONE transaction, and that is what
// the order cannot give: this pass finds its work by the BOARD, so a board renamed without its
// jobs is never listed again and its stored postings keep the placeholder for good, while jobs
// renamed without the board are put back by the next crawl. Each half alone is a lost write.
//
// The slug is the one the pipeline writes for the same name — CompanySlug, then the merge
// worker's alias registry — so a posting the next crawl rewrites lands on the company this
// rename created instead of re-keying it again. The OLD slug is recorded as an alias of it: its
// /companies page had been served and announced to search engines, and an alias is what turns
// that address into a 301 instead of a 404.
func writePlaceholderRename(ctx context.Context, q placeholderWriter, r placeholderRename) (bool, error) {
	slug := normalize.CompanySlug(r.name)
	aliases, err := q.ResolveCompanySlugAliases(ctx, []string{normalize.FoldSlug(slug)})
	if err != nil {
		return false, fmt.Errorf("aliases: %w", err)
	}
	if len(aliases) > 0 {
		slug = aliases[0].CanonicalSlug
	}
	n, err := q.RenamePlaceholderBoardJobs(ctx, db.RenamePlaceholderBoardJobsParams{
		Name:         r.name,
		NewSlug:      slug,
		Source:       r.provider,
		BoardPattern: externalid.BoardPattern(r.board),
		OldName:      r.oldName,
	})
	if err != nil {
		return false, fmt.Errorf("jobs: %w", err)
	}
	boardRows, err := q.RenameSlugBoardCompany(ctx, db.RenameSlugBoardCompanyParams{
		Name:     r.name,
		Provider: r.provider,
		Board:    r.board,
		OldName:  r.oldName,
	})
	if err != nil {
		return false, fmt.Errorf("board: %w", err)
	}
	// The board changed since it was listed — a curator named it meanwhile. Failing rolls the
	// jobs half back, which would otherwise carry this name under a board stating another.
	if boardRows == 0 {
		return false, fmt.Errorf("board: %s %s no longer carries %q", r.provider, r.board, r.oldName)
	}
	if oldSlug := normalize.CompanySlug(r.oldName); oldSlug != "" && oldSlug != slug {
		if _, err := q.InsertCompanySlugAlias(ctx, db.InsertCompanySlugAliasParams{
			AliasSlug:     oldSlug,
			CanonicalSlug: slug,
			FoldedKey:     normalize.FoldSlug(oldSlug),
			Reason:        "board_name",
		}); err != nil {
			return false, fmt.Errorf("alias: %w", err)
		}
	}
	return n > 0, nil
}
