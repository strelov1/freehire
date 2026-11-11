package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// statsPublication counts what one publication of insights_company_stats changed.
type statsPublication struct{ inserted, updated, deleted int64 }

// publishCompanyStats makes insights_company_stats equal rows inside the caller's
// publication transaction, writing only the difference.
//
// The table is a full recomputation, but almost all of it is the same as yesterday: a
// (company, day) row changes when that day's added/removed moves, or when an earlier day
// of the same company does (open is a running total), and the great majority of companies
// do neither. Deleting and re-copying every row rewrote ~5M rows and both indexes a day
// for that, so the result is staged and compared by key (company_slug, day) instead:
//
//   - a key absent from rows is deleted (a vanished company, a reopened job's old day, a
//     merged or renamed slug), and an empty rows empties the table, as a full rebuild did;
//   - a key whose added/removed/open differs is updated, by value, never by "today only":
//     a correction to an old day shifts the running total of every later day;
//   - a key the table lacks is inserted; an identical row is not touched, so its
//     tuple, xmin and index entries stay as they were.
//
// The stage is a temp table dropped at commit: invisible to every other session, and
// nothing is left behind when the publication rolls back. Its heap and index data are
// unlogged, so staging 5M rows writes no WAL for them; the temp-table DDL itself still
// writes catalog (pg_class, pg_attribute, ...) WAL. The update and the insert are separate
// statements on purpose: INSERT ... ON CONFLICT DO UPDATE locks every conflicting row
// before it evaluates its WHERE, which would write a lock to every unchanged row and
// defeat the point.
func publishCompanyStats(ctx context.Context, tx pgx.Tx, rows []db.ComputeInsightsCompanyStatsRow) (statsPublication, error) {
	var res statsPublication
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE company_stats_stage
		(LIKE insights_company_stats INCLUDING DEFAULTS, PRIMARY KEY (company_slug, day))
		ON COMMIT DROP`); err != nil {
		return res, fmt.Errorf("create company stats stage: %w", err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"company_stats_stage"},
		[]string{"company_slug", "day", "added", "removed", "open"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.CompanySlug, r.Day, r.Added, r.Removed, r.Open}, nil
		})); err != nil {
		return res, fmt.Errorf("stage company stats: %w", err)
	}
	// A temp table is never autovacuumed or analyzed, and the joins below are planned on it.
	if _, err := tx.Exec(ctx, `ANALYZE company_stats_stage`); err != nil {
		return res, fmt.Errorf("analyze company stats stage: %w", err)
	}

	del, err := tx.Exec(ctx, `DELETE FROM insights_company_stats t
		WHERE NOT EXISTS (SELECT 1 FROM company_stats_stage s
		                  WHERE s.company_slug = t.company_slug AND s.day = t.day)`)
	if err != nil {
		return res, fmt.Errorf("delete vanished company stats: %w", err)
	}
	upd, err := tx.Exec(ctx, `UPDATE insights_company_stats t
		SET added = s.added, removed = s.removed, open = s.open
		FROM company_stats_stage s
		WHERE t.company_slug = s.company_slug AND t.day = s.day
		  AND (t.added, t.removed, t.open) IS DISTINCT FROM (s.added, s.removed, s.open)`)
	if err != nil {
		return res, fmt.Errorf("update changed company stats: %w", err)
	}
	ins, err := tx.Exec(ctx, `INSERT INTO insights_company_stats (company_slug, day, added, removed, open)
		SELECT s.company_slug, s.day, s.added, s.removed, s.open
		FROM company_stats_stage s
		WHERE NOT EXISTS (SELECT 1 FROM insights_company_stats t
		                  WHERE t.company_slug = s.company_slug AND t.day = s.day)`)
	if err != nil {
		return res, fmt.Errorf("insert new company stats: %w", err)
	}
	res = statsPublication{inserted: ins.RowsAffected(), updated: upd.RowsAffected(), deleted: del.RowsAffected()}
	log.Printf("rollup-company: insights_company_stats: %d rows computed, diff to publish: %d inserted, %d updated, %d deleted",
		len(rows), res.inserted, res.updated, res.deleted)
	return res, nil
}
