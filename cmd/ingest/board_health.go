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
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// boardHealth adapts *db.Queries to pipeline.BoardHealth: it reads a board's cooldown
// and records each crawl's outcome, applying the backoff policy (pipeline.CooldownFor)
// to the failure count the DB returns. It also drives the boards table's pending -> active
// transition from the same success signal (see RecordSuccess).
type boardHealth struct {
	q       *db.Queries
	catalog boardcatalog.Repository
}

var _ pipeline.BoardHealth = (*boardHealth)(nil)

func newBoardHealth(pool *pgxpool.Pool) *boardHealth {
	q := db.New(pool)
	return &boardHealth{q: q, catalog: boardcatalog.NewQueriesRepository(q)}
}

// Pauses returns the board's failure cooldown and quiet pause in one lookup; an absent row or a
// NULL column means that pause is not set.
func (h *boardHealth) Pauses(ctx context.Context, provider, board, region string) (pipeline.BoardPauses, error) {
	row, err := h.q.GetBoardCooldown(ctx, db.GetBoardCooldownParams{Provider: provider, Board: board, Region: region})
	if errors.Is(err, pgx.ErrNoRows) {
		return pipeline.BoardPauses{}, nil
	}
	if err != nil {
		return pipeline.BoardPauses{}, err
	}
	var p pipeline.BoardPauses
	if row.CooldownUntil.Valid {
		p.Cooldown = row.CooldownUntil.Time
	}
	if row.QuietUntil.Valid {
		p.Quiet = row.QuietUntil.Time
	}
	return p, nil
}

// RecordSuccess clears the board's failure state and stamps freshness. It also flips a
// pending boards row to active on its first successful crawl (a no-op, found=false, for an
// active board or one with no boards row).
//
// A non-empty fingerprint feeds the quiet-board backoff: the query compares it with the stored
// one and returns the resulting quiet streak, and an unchanged listing gets the Go-owned pause
// (pipeline.QuietFor) applied the way RecordFailure applies a cooldown — CAS-guarded on the
// streak it was computed from, so a stale writer cannot pause a board a newer crawl just reset.
func (h *boardHealth) RecordSuccess(ctx context.Context, provider, board, region string, ingested int, reached bool, fingerprint string) error {
	streak, err := h.q.RecordBoardSuccess(ctx, db.RecordBoardSuccessParams{
		Provider:          provider,
		Board:             board,
		Region:            region,
		LastIngestedCount: pgtype.Int4{Int32: int32(ingested), Valid: true},
		Reached:           reached,
		Fingerprint:       pgconv.Text(fingerprint),
	})
	if err != nil {
		return err
	}
	if fingerprint != "" {
		if d, quiet := pipeline.QuietFor(int(streak)); quiet {
			if _, err := h.q.SetBoardQuietUntil(ctx, db.SetBoardQuietUntilParams{
				Provider:    provider,
				Board:       board,
				Region:      region,
				QuietUntil:  pgconv.Instant(time.Now().Add(d)),
				QuietStreak: streak,
			}); err != nil {
				return err
			}
		}
	}
	_, err = h.catalog.Activate(ctx, provider, board, region)
	return err
}

// RecordFailure bumps the failure count (the query returns the new count), then applies
// the Go-owned backoff policy: it sets a cooldown only once the count crosses the
// threshold.
//
// SetBoardCooldown is CAS-guarded on the failures value the cooldown was computed from: the
// worker pool can process one board twice in a run, and without the guard the last writer
// would win, applying a stale shorter cooldown or clobbering one a concurrent
// RecordSuccess/ClearProviderCooldowns just cleared. Zero rows affected means a newer writer
// moved on; that is expected, so it is only logged.
func (h *boardHealth) RecordFailure(ctx context.Context, provider, board, region string, cause error) error {
	failures, err := h.q.RecordBoardFailure(ctx, db.RecordBoardFailureParams{
		Provider:  provider,
		Board:     board,
		Region:    region,
		LastError: pgconv.TextValid(cause.Error()),
		BoardGone: errors.Is(cause, sources.ErrBoardGone),
	})
	if err != nil {
		return err
	}
	d, cool := pipeline.CooldownFor(int(failures))
	if !cool {
		return nil
	}
	rows, err := h.q.SetBoardCooldown(ctx, db.SetBoardCooldownParams{
		Provider:            provider,
		Board:               board,
		Region:              region,
		CooldownUntil:       pgconv.Instant(time.Now().Add(d)),
		ConsecutiveFailures: failures,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		log.Printf("ingest health: %s/%s/%s cooldown not applied (consecutive_failures moved past %d) — a concurrent writer already superseded it",
			provider, board, region, failures)
	}
	return nil
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
// The list is FLEET-WIDE (not filtered to the provider being crawled) and every hourly ingest
// unit prints it, so its length multiplies by the whole fleet's crawl rate; uncapped it filled
// the host's disk. Twenty shows what is worst; board_health answers the rest in SQL.
const unhealthyBoardsCap = 20

// logUnhealthyBoards emits one summary line naming the worst boards currently failing or
// cooled, and how many there are in total. Best-effort: a read error is logged and ignored.
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
// count reported is the FULL one, never len(rows), or it would read as "only 20 are broken".
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

// chronicBoardsCap bounds how many chronic boards the per-run report names, for the same
// log-size reason as unhealthyBoardsCap.
const chronicBoardsCap = 20

// chronicBoardWindowDaysDefault is how long a board must have gone without a successful crawl
// to be called chronic, far above the sweep's ~24h cooldown ceiling so a board merely backing
// off is never mistaken for a broken one. CHRONIC_BOARD_WINDOW_DAYS overrides it.
const chronicBoardWindowDaysDefault int32 = 30

// logChronicBoards emits one summary line naming the worst chronic boards: those with no
// successful crawl in the reporting window, as opposed to merely cooling down. A separate line
// from logUnhealthyBoards so a curator can tell ordinary backoff from "needs a decision".
// Best-effort: a read or config error is logged and ignored.
func logChronicBoards(ctx context.Context, q *db.Queries) {
	days, err := worker.EnvInt32("CHRONIC_BOARD_WINDOW_DAYS", chronicBoardWindowDaysDefault)
	if err != nil {
		log.Printf("ingest health: chronic board window: %v", err)
		return
	}
	rows, err := q.ListChronicBoards(ctx, db.ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: days, Valid: true},
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
// "provider/board[/region](days=N)" (or "provider(days=N)" for a boardless provider) where N
// is days since last_success_at, or first_seen_at for a board that never succeeded. The
// region matters: a board name repeated across regions (Adzuna's "it-jobs") can be chronic in
// one and healthy in another, the ambiguity cmd/close-chronic-boards refuses to act on
// (isRegionAmbiguous). Same cap/total convention as unhealthyBoardsSummary.
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
