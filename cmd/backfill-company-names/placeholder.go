package main

import (
	"context"
	"log"
	"sync"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/platform/db"
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
	RenamePlaceholderBoardJobs(context.Context, db.RenamePlaceholderBoardJobsParams) ([]string, error)
	RenameSlugBoardCompany(context.Context, db.RenameSlugBoardCompanyParams) (int64, error)
	InsertCompanySlugAlias(context.Context, db.InsertCompanySlugAliasParams) (int64, error)
}

// writePlaceholderRename applies one resolved name to the board's stored jobs and to the
// board (boardcatalog.RenameBoardJobs; the caller runs it in ONE transaction). This pass finds
// its work by the BOARD, so a board renamed without its jobs is never listed again.
// RenameSlugBoardCompany is guarded on the old name, so a board a curator named meanwhile
// fails the rename instead of being overwritten.
func writePlaceholderRename(ctx context.Context, q placeholderWriter, r placeholderRename) (string, []string, error) {
	return boardcatalog.RenameBoardJobs(ctx, q,
		boardcatalog.BoardRename{Provider: r.provider, Board: r.board, OldName: r.oldName, Name: r.name, AliasOldSlug: true},
		func(ctx context.Context) (int64, error) {
			return q.RenameSlugBoardCompany(ctx, db.RenameSlugBoardCompanyParams{
				Name: r.name, Provider: r.provider, Board: r.board, OldName: r.oldName,
			})
		})
}
