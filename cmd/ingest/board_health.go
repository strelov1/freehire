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

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pipeline"
)

// boardHealth adapts *db.Queries to pipeline.BoardHealth: it reads a board's cooldown
// and records each crawl's outcome, applying the backoff policy (pipeline.CooldownFor)
// to the failure count the DB returns.
type boardHealth struct{ q *db.Queries }

var _ pipeline.BoardHealth = (*boardHealth)(nil)

func newBoardHealth(pool *pgxpool.Pool) *boardHealth { return &boardHealth{q: db.New(pool)} }

// Cooldown returns the board's cooldown_until; an absent row or a NULL cooldown means
// eligible.
func (h *boardHealth) Cooldown(ctx context.Context, provider, board string) (time.Time, bool, error) {
	ts, err := h.q.GetBoardCooldown(ctx, db.GetBoardCooldownParams{Provider: provider, Board: board})
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

// RecordSuccess clears the board's failure state and stamps freshness.
func (h *boardHealth) RecordSuccess(ctx context.Context, provider, board string, ingested int) error {
	return h.q.RecordBoardSuccess(ctx, db.RecordBoardSuccessParams{
		Provider:          provider,
		Board:             board,
		LastIngestedCount: pgtype.Int4{Int32: int32(ingested), Valid: true},
	})
}

// RecordFailure bumps the failure count (the query returns the new count), then applies
// the Go-owned backoff policy: it sets a cooldown only once the count crosses the
// threshold.
func (h *boardHealth) RecordFailure(ctx context.Context, provider, board, errMsg string) error {
	failures, err := h.q.RecordBoardFailure(ctx, db.RecordBoardFailureParams{
		Provider:  provider,
		Board:     board,
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
		CooldownUntil: pgtype.Timestamptz{Time: time.Now().Add(d), Valid: true},
	})
}

// logUnhealthyBoards emits one summary line naming every board currently failing or
// cooled — so an operator sees the ingest fleet's health in the run log without a
// query. Best-effort: a read error is logged and ignored (it never fails the run).
func logUnhealthyBoards(ctx context.Context, q *db.Queries) {
	rows, err := q.ListUnhealthyBoards(ctx)
	if err != nil {
		log.Printf("ingest health: list unhealthy boards: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		desc := fmt.Sprintf("%s/%s(fails=%d", r.Provider, r.Board, r.ConsecutiveFailures)
		if r.CooldownUntil.Valid && r.CooldownUntil.Time.After(time.Now()) {
			desc += ",cooled_until=" + r.CooldownUntil.Time.UTC().Format(time.RFC3339)
		}
		parts = append(parts, desc+")")
	}
	log.Printf("ingest health: %d unhealthy board(s): %s", len(rows), strings.Join(parts, " "))
}
