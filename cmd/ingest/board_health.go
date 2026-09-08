package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// boardHealth adapts *db.Queries to pipeline.BoardHealth: it reads a board's cooldown
// and records each crawl's outcome, applying the backoff policy (pipeline.CooldownFor)
// to the failure count the DB returns. It also piggybacks the boards table's
// pending -> active transition on the same success signal (see RecordSuccess) — a
// board's crawl outcome is the one thing both tables react to, so there is no need for
// a second place in the pipeline to learn it.
type boardHealth struct {
	q       *db.Queries
	catalog boardcatalog.Repository
}

var _ pipeline.BoardHealth = (*boardHealth)(nil)

func newBoardHealth(pool *pgxpool.Pool) *boardHealth {
	q := db.New(pool)
	return &boardHealth{q: q, catalog: boardcatalog.NewQueriesRepository(q)}
}

// Cooldown returns the board's cooldown_until; an absent row or a NULL cooldown means
// eligible.
func (h *boardHealth) Cooldown(ctx context.Context, provider, board, region string) (time.Time, bool, error) {
	ts, err := h.q.GetBoardCooldown(ctx, db.GetBoardCooldownParams{Provider: provider, Board: board, Region: region})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if !ts.Valid {
		return time.Time{}, false, nil
	}
	return ts.Time, true, nil
}

// RecordSuccess clears the board's failure state and stamps freshness. It also flips a
// pending boards row to active on this, its first successful crawl (a no-op — found=false,
// no error — for a board that is already active, or that has no boards row at all, e.g.
// one crawled before this migration).
func (h *boardHealth) RecordSuccess(ctx context.Context, provider, board, region string, ingested int) error {
	if err := h.q.RecordBoardSuccess(ctx, db.RecordBoardSuccessParams{
		Provider:          provider,
		Board:             board,
		Region:            region,
		LastIngestedCount: pgtype.Int4{Int32: int32(ingested), Valid: true},
	}); err != nil {
		return err
	}
	_, err := h.catalog.Activate(ctx, provider, board, region)
	return err
}

// RecordFailure bumps the failure count (the query returns the new count), then applies
// the Go-owned backoff policy: it sets a cooldown only once the count crosses the
// threshold.
func (h *boardHealth) RecordFailure(ctx context.Context, provider, board, region, errMsg string) error {
	failures, err := h.q.RecordBoardFailure(ctx, db.RecordBoardFailureParams{
		Provider:  provider,
		Board:     board,
		Region:    region,
		LastError: pgtype.Text{String: errMsg, Valid: true},
	})
	if err != nil {
		return err
	}
	d, cool := pipeline.CooldownFor(int(failures))
	if !cool {
		return nil
	}
	return h.q.SetBoardCooldown(ctx, db.SetBoardCooldownParams{
		Provider:      provider,
		Board:         board,
		Region:        region,
		CooldownUntil: pgtype.Timestamptz{Time: time.Now().Add(d), Valid: true},
	})
}

// CooledBoards returns up to limit (board, region) pairs of the provider currently in an
// active cooldown.
func (h *boardHealth) CooledBoards(ctx context.Context, provider string, limit int) ([]pipeline.CooledBoard, error) {
	rows, err := h.q.ListCooledBoards(ctx, db.ListCooledBoardsParams{Provider: provider, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.CooledBoard, len(rows))
	for i, r := range rows {
		out[i] = pipeline.CooledBoard{Board: r.Board, Region: r.Region}
	}
	return out, nil
}

// ClearCooldowns clears the active cooldown and failure count of every currently-cooled
// board of the provider, returning how many were cleared.
func (h *boardHealth) ClearCooldowns(ctx context.Context, provider string) (int, error) {
	n, err := h.q.ClearProviderCooldowns(ctx, provider)
	return int(n), err
}

// unhealthyBoardsCap bounds how many boards the per-run summary names.
//
// The list is FLEET-WIDE — it is not filtered to the provider being crawled — and every one of
// the ~200 hourly ingest units prints it, so the line's cost is its length times the whole
// fleet's crawl rate. Uncapped, a 7000-board backlog turned that into ~260KB per run and
// gigabytes of syslog per day, which filled the host's disk far enough that cmd/reindex hit its
// REINDEX_MIN_FREE_GB floor and refused to rebuild for a week (2026-08-14). Twenty is enough to
// see what is worst at a glance; board_health answers the rest in SQL, which is what the summary
// was always a shortcut for.
const unhealthyBoardsCap = 20

// logUnhealthyBoards emits one summary line naming the worst boards currently failing or
// cooled, and how many there are in total — so an operator sees the ingest fleet's health in
// the run log without a query. Best-effort: a read error is logged and ignored (it never fails
// the run).
func logUnhealthyBoards(ctx context.Context, q *db.Queries) {
	rows, err := q.ListUnhealthyBoards(ctx, unhealthyBoardsCap)
	if err != nil {
		log.Printf("ingest health: list unhealthy boards: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	// Every row carries the same pre-LIMIT count; the first one is as good as any.
	log.Printf("ingest health: %s", unhealthyBoardsSummary(rows, rows[0].Total, time.Now()))
}

// unhealthyBoardsSummary renders the summary line's body: each named board as
// "provider/board[/region](fails=N[,cooled_until=…])", plus how many the cap left out. The
// count reported is the FULL one, never len(rows) — a capped list that printed its own length
// would read as "only 20 boards are broken".
func unhealthyBoardsSummary(rows []db.ListUnhealthyBoardsRow, total int64, now time.Time) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		id := r.Provider + "/" + r.Board
		if r.Region != "" {
			id += "/" + r.Region
		}
		desc := fmt.Sprintf("%s(fails=%d", id, r.ConsecutiveFailures)
		if r.CooldownUntil.Valid && r.CooldownUntil.Time.After(now) {
			desc += ",cooled_until=" + r.CooldownUntil.Time.UTC().Format(time.RFC3339)
		}
		parts = append(parts, desc+")")
	}
	if omitted := total - int64(len(rows)); omitted > 0 {
		return fmt.Sprintf("%d unhealthy board(s), worst %d: %s (%d more)",
			total, len(rows), strings.Join(parts, " "), omitted)
	}
	return fmt.Sprintf("%d unhealthy board(s): %s", total, strings.Join(parts, " "))
}

// chronicBoardsCap bounds how many chronic boards the per-run report names, same log-size
// concern as unhealthyBoardsCap — applied to the strictly smaller chronic subset, since a board
// only qualifies after chronicBoardWindowDays of unbroken failure, not after a handful.
const chronicBoardsCap = 20

// chronicBoardWindowDaysDefault is how long a board must have gone without a successful crawl
// before the per-run report calls it chronic (see ingest-board-health spec) — deliberately far
// above the sweep's ~24h cooldown ceiling, so a board merely backing off from a recent run of
// failures is never mistaken for one that has proven itself broken. CHRONIC_BOARD_WINDOW_DAYS
// overrides it without a deploy.
const chronicBoardWindowDaysDefault = 30

// logChronicBoards emits one summary line naming the worst chronic boards — those that have
// gone the reporting window without a single successful crawl, as opposed to merely cooling
// down from a recent run of failures (openspec change close-chronically-unreachable-boards,
// issue #2017). Distinct from logUnhealthyBoards's line so a curator can tell "still within an
// ordinary backoff" apart from "has not worked in over a month and needs a decision". Best-
// effort, like its sibling: a read or config error is logged and ignored, never fails the run.
func logChronicBoards(ctx context.Context, q *db.Queries) {
	days, err := worker.EnvInt64("CHRONIC_BOARD_WINDOW_DAYS", chronicBoardWindowDaysDefault)
	if err != nil {
		log.Printf("ingest health: chronic board window: %v", err)
		return
	}
	rows, err := q.ListChronicBoards(ctx, db.ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: int32(days), Valid: true},
		MaxBoards: chronicBoardsCap,
	})
	if err != nil {
		log.Printf("ingest health: list chronic boards: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	log.Printf("ingest health: %s", chronicBoardsSummary(rows, rows[0].Total, time.Now()))
}

// chronicBoardsSummary renders the chronic-board line: each named board as
// "provider/board[/region](days=N)" — or just "provider(days=N)" for a boardless provider's
// own record — where N is days since the board's evidence anchor: last_success_at for a board
// that has succeeded at least once, first_seen_at for one that never has (see
// ingest-board-health spec). The region matters here specifically: a board name repeated
// across regions (e.g. Adzuna's "it-jobs" once per country) can be chronic in one and healthy
// in another, and without the region a curator reading this line cannot tell which — the exact
// ambiguity cmd/close-chronic-boards refuses to act on (isRegionAmbiguous). Same
// region-appending and cap/total convention as unhealthyBoardsSummary.
func chronicBoardsSummary(rows []db.ListChronicBoardsRow, total int64, now time.Time) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		since := r.FirstSeenAt.Time
		if r.LastSuccessAt.Valid {
			since = r.LastSuccessAt.Time
		}
		days := int(now.Sub(since).Hours() / 24)
		id := r.Provider
		if r.Board != "" {
			id += "/" + r.Board
		}
		if r.Region != "" {
			id += "/" + r.Region
		}
		parts = append(parts, fmt.Sprintf("%s(days=%d)", id, days))
	}
	if omitted := total - int64(len(rows)); omitted > 0 {
		return fmt.Sprintf("%d chronic board(s), worst %d: %s (%d more)",
			total, len(rows), strings.Join(parts, " "), omitted)
	}
	return fmt.Sprintf("%d chronic board(s): %s", total, strings.Join(parts, " "))
}
