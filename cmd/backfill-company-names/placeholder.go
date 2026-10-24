package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"golang.org/x/sync/errgroup"

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

// placeholderPerProvider bounds simultaneous fetches against ONE provider. The pass's providers
// each serve every board from one host or a handful (recruiting.paylocity.com is all of
// Paylocity), so resolveConcurrency — a bound across many hosts — lands on one: the first prod
// run, at 24, drew 429s from Paylocity within minutes and crawled at its retry backoff.
const placeholderPerProvider = 4

// resolvePlaceholders fetches each placeholder board's name and keeps the ones
// AcceptPlaceholder admits — the providers in parallel, each held to placeholderPerProvider.
// unresolved counts the boards left as they are — no resolver, a failed fetch, or nothing
// usable — none of which is an error: the board keeps the placeholder it already had and the
// next run asks again. Order-independent; the caller sorts.
func resolvePlaceholders(ctx context.Context, rows []db.ListPlaceholderCompanyBoardsRow, registry companyname.Registry) ([]placeholderRename, int) {
	var (
		mu         sync.Mutex
		renames    []placeholderRename
		unresolved int
	)

	byProvider := map[string][]db.ListPlaceholderCompanyBoardsRow{}
	for _, row := range rows {
		byProvider[row.Provider] = append(byProvider[row.Provider], row)
	}

	var wg sync.WaitGroup
	for provider, boards := range byProvider {
		resolver, ok := registry[provider]
		if !ok {
			// Under the lock: a provider started earlier in this loop writes the same counter.
			mu.Lock()
			unresolved += len(boards)
			mu.Unlock()
			continue
		}
		wg.Go(func() {
			var g errgroup.Group
			g.SetLimit(placeholderPerProvider)
			for _, row := range boards {
				g.Go(func() error {
					candidate, err := resolver.Name(ctx, row.Board)
					if err != nil {
						// A resolver returns "" beside an error, which AcceptPlaceholder refuses.
						log.Printf("resolve %s %s: %v", row.Provider, row.Board, err)
					}
					name, ok := companyname.AcceptPlaceholder(row.Board, candidate)
					mu.Lock()
					defer mu.Unlock()
					if !ok {
						unresolved++
						return nil // a single board failing must not abort the run
					}
					renames = append(renames, placeholderRename{provider: row.Provider, board: row.Board, oldName: row.Company, name: name})
					return nil
				})
			}
			_ = g.Wait()
		})
	}
	wg.Wait()

	return renames, unresolved
}

// placeholderWriter is the slice of db.Queries writePlaceholderRename needs.
type placeholderWriter interface {
	ResolveCompanySlugAliases(context.Context, []string) ([]db.ResolveCompanySlugAliasesRow, error)
	RenamePlaceholderBoardJobs(context.Context, db.RenamePlaceholderBoardJobsParams) (int64, error)
	RenameSlugBoardCompany(context.Context, db.RenameSlugBoardCompanyParams) (int64, error)
}

// writePlaceholderRename applies one resolved name to the board's stored jobs and to the board,
// reporting whether any job changed. The caller runs it in ONE transaction, and that is what
// the order cannot give: this pass finds its work by the BOARD, so a board renamed without its
// jobs is never listed again and its stored postings keep the placeholder for good, while jobs
// renamed without the board are put back by the next crawl. Each half alone is a lost write.
//
// The slug is the one the pipeline writes for the same name — CompanySlug, then the merge
// worker's alias registry — so a posting the next crawl rewrites lands on the company this
// rename created instead of re-keying it again.
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
	return n > 0, nil
}
