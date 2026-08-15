// Package backfillpage holds the keyset-pagination walk shared by the one-off
// per-source repair workers (cmd/backfill-echojobs, ...): page every row for one
// source (id > last seen, so concurrent writes cannot skip or repeat rows), stopping
// once a page comes back short of the batch size. Only that walk is shared — each
// worker's actual repair (re-fetch a detail body, parse a stored URL, ...)
// differs enough that factoring it in too would cost more clarity than it
// saves.
package backfillpage

import (
	"context"

	"github.com/strelov1/freehire/internal/db"
)

// BatchSize bounds how many rows are read per keyset page.
const BatchSize = 500

// Lister pages one source's rows by keyset; *db.Queries's ListJobsBySourceAfter
// satisfies it.
type Lister func(ctx context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error)

// Rows walks every row for source in keyset batches, calling visit once per
// row. visit's own error aborts the whole walk immediately — for a failure
// the caller wants to count and skip instead (a single row's detail fetch or
// URL parse failing, say), visit must handle that itself and return nil.
func Rows(ctx context.Context, list Lister, source string, visit func(db.Job) error) error {
	var afterID int64
	for {
		jobs, err := list(ctx, db.ListJobsBySourceAfterParams{
			Source:    source,
			AfterID:   afterID,
			BatchSize: BatchSize,
		})
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			break
		}
		afterID = jobs[len(jobs)-1].ID

		for _, j := range jobs {
			if err := visit(j); err != nil {
				return err
			}
		}

		if len(jobs) < BatchSize {
			break
		}
	}
	return nil
}
