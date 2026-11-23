package boardcatalog

import (
	"context"
	"fmt"
	"slices"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// BoardRename moves one board's stored jobs from OldName to Name.
type BoardRename struct {
	Provider string
	Board    string
	OldName  string
	Name     string
	// AliasOldSlug records the old name's slug as an alias of the new one, so its /companies
	// page 301s. Only for a placeholder old name (the board id): a curator correcting a WRONG
	// employer must not alias that real employer's slug onto this one, or every source's
	// postings for it would be re-keyed here.
	AliasOldSlug bool
}

// JobRenamer is the slice of db.Queries RenameBoardJobs needs.
type JobRenamer interface {
	ResolveCompanySlugAliases(context.Context, []string) ([]db.ResolveCompanySlugAliasesRow, error)
	RenamePlaceholderBoardJobs(context.Context, db.RenamePlaceholderBoardJobsParams) ([]string, error)
	InsertCompanySlugAlias(context.Context, db.InsertCompanySlugAliasParams) (int64, error)
}

// RenameBoardJobs applies a new name to the board's stored jobs and, through writeBoard, to
// the board row, reporting the slug the jobs moved onto and the distinct slugs they moved away
// from (none when no job changed). The caller runs it in ONE transaction: a crawl only
// REWRITES a posting that changed, so a board renamed without its jobs leaves them on the old
// name for good, while jobs renamed without the board are put back by the next crawl.
// writeBoard returns the board rows it changed; 0 fails the call so the jobs half rolls back.
//
// The slug is the one the pipeline writes for the same name (CompanySlug, then the merge
// worker's alias registry), so the next crawl lands on the company this rename created. Under
// AliasOldSlug the OLD slug is recorded as an alias so its already-announced /companies page
// 301s instead of 404ing. The caller reconciles the companies rows (SyncCompaniesForSlugs for the new slug,
// DeleteOrphanCompaniesForSlugs for the old ones).
func RenameBoardJobs(ctx context.Context, q JobRenamer, r BoardRename, writeBoard func(context.Context) (int64, error)) (string, []string, error) {
	slug := normalize.CompanySlug(r.Name)
	aliases, err := q.ResolveCompanySlugAliases(ctx, []string{normalize.FoldSlug(slug)})
	if err != nil {
		return "", nil, fmt.Errorf("aliases: %w", err)
	}
	if len(aliases) > 0 {
		slug = aliases[0].CanonicalSlug
	}
	oldSlugs, err := q.RenamePlaceholderBoardJobs(ctx, db.RenamePlaceholderBoardJobsParams{
		Name:         r.Name,
		NewSlug:      slug,
		Source:       r.Provider,
		BoardPattern: externalid.BoardPattern(r.Board),
		OldName:      r.OldName,
		LabelName:    sources.PlaceholderLabel(r.Provider, r.OldName),
	})
	if err != nil {
		return "", nil, fmt.Errorf("jobs: %w", err)
	}
	boardRows, err := writeBoard(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("board: %w", err)
	}
	// The board changed since it was read (a curator named it meanwhile). Failing rolls the
	// jobs half back, which would otherwise carry this name under a board stating another.
	if boardRows == 0 {
		return "", nil, fmt.Errorf("board: %s %s no longer carries %q", r.Provider, r.Board, r.OldName)
	}
	if oldSlug := normalize.CompanySlug(r.OldName); r.AliasOldSlug && oldSlug != "" && oldSlug != slug {
		if _, err := q.InsertCompanySlugAlias(ctx, db.InsertCompanySlugAliasParams{
			AliasSlug:     oldSlug,
			CanonicalSlug: slug,
			FoldedKey:     normalize.FoldSlug(oldSlug),
			Reason:        "board_name",
		}); err != nil {
			return "", nil, fmt.Errorf("alias: %w", err)
		}
	}
	slices.Sort(oldSlugs)
	return slug, slices.Compact(oldSlugs), nil
}
