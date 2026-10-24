package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// lastRunKey identifies a board_health row the way that table does: board case-folded,
// since its identity is (provider, lower(board), region) — see RecordBoardSuccess.
type lastRunKey struct{ board, region string }

func newLastRunKey(board, region string) lastRunKey {
	return lastRunKey{board: strings.ToLower(board), region: region}
}

// loadLastRuns reads when each of provider's boards was last crawled. A board with no row,
// or a row with no last_run_at, is absent from the map: never crawled.
func loadLastRuns(ctx context.Context, q *db.Queries, provider string) (map[lastRunKey]time.Time, error) {
	rows, err := q.ListBoardLastRuns(ctx, provider)
	if err != nil {
		return nil, err
	}
	lastRun := make(map[lastRunKey]time.Time, len(rows))
	for _, r := range rows {
		if r.LastRunAt.Valid {
			lastRun[newLastRunKey(r.Board, r.Region)] = r.LastRunAt.Time
		}
	}
	return lastRun, nil
}

// orderOldestRunFirst reorders entries so the boards waited on longest start first: every
// never-crawled board, then the rest by last_run_at ascending. Ties keep their catalog order.
//
// The pipeline starts boards in (roughly) slice order, and a run that hits its deadline abandons the
// ones still queued. With the catalog's fixed id order those were always the same boards —
// the newest — so a provider whose run cannot finish never crawled a board added after the
// cut-off at all: successfactors reached ~320 of each shard's 850 boards per run, and
// careers.ey.com (9k postings) sat `pending` for eleven days without one attempt. Starting
// from the stalest board turns the same truncation into a rotation, so every board is
// reached within a few runs instead of never.
//
// It must run AFTER sources.Config.Shard: a shard is assigned by the order a company first
// appears, so reordering before sharding would move companies between shards from one run to
// the next.
func orderOldestRunFirst(entries []sources.CompanyEntry, lastRun map[lastRunKey]time.Time) {
	slices.SortStableFunc(entries, func(a, b sources.CompanyEntry) int {
		ta, aRan := lastRun[newLastRunKey(a.Board, a.Region)]
		tb, bRan := lastRun[newLastRunKey(b.Board, b.Region)]
		switch {
		case !aRan && !bRan:
			return 0
		case !aRan:
			return -1
		case !bRan:
			return 1
		}
		return ta.Compare(tb)
	})
}
