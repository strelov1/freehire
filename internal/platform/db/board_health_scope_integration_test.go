//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestBoardHealthCatalogScope(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)
	_, err := pool.Exec(ctx, `TRUNCATE board_health, boards;
 INSERT INTO boards (provider, board, region, company, status) VALUES
 ('ukg','ACTIVE','','a','active'), ('ukg','pending','','p','pending'),
 ('ukg','retired','','r','retired'), ('ukg','rejected','','r','rejected'),
 ('ukg','regional','ca','c','active');
 INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at, last_success_at) VALUES
 ('ukg','active','',3,now()-interval '50 days',now()-interval '40 days'),
 ('ukg','pending','',3,now()-interval '50 days',NULL),
 ('ukg','retired','',3,now()-interval '50 days',NULL),
 ('ukg','rejected','',3,now()-interval '50 days',NULL),
 ('ukg','orphan','',3,now()-interval '50 days',NULL),
 ('ukg','regional','us',3,now()-interval '50 days',NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	unhealthy, err := q.ListUnhealthyBoards(ctx, 1)
	if err != nil || len(unhealthy) != 1 || unhealthy[0].Total != 2 {
		t.Fatalf("unhealthy = %+v, %v", unhealthy, err)
	}
	chronic, err := q.ListChronicBoards(ctx, ListChronicBoardsParams{AgeWindow: pgtype.Interval{Days: 30, Valid: true}, MaxBoards: 10})
	if err != nil || len(chronic) != 2 || chronic[0].Total != 2 {
		t.Fatalf("chronic = %+v, %v", chronic, err)
	}
	rolls, err := q.ProviderHealthRollup(ctx)
	if err != nil || len(rolls) != 1 || rolls[0].TotalBoards != 1 {
		t.Fatalf("rollup = %+v, %v", rolls, err)
	}
	// Scope is read-only: excluded failures remain intact as historical evidence.
	var failures int
	if err := pool.QueryRow(ctx, `SELECT sum(consecutive_failures) FROM board_health`).Scan(&failures); err != nil || failures != 18 {
		t.Fatalf("failure history = %d, %v", failures, err)
	}
}

func TestProviderHealthQuietFreshnessKeepsActualSuccess(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)
	_, err := pool.Exec(ctx, `TRUNCATE board_health, boards;
 INSERT INTO boards (provider,board,company,status) VALUES ('ukg','quiet','q','active');
 INSERT INTO board_health (provider,board,last_success_at,quiet_until,consecutive_failures)
 VALUES ('ukg','quiet',now()-interval '60 hours',now()+interval '12 hours',0)`)
	if err != nil {
		t.Fatal(err)
	}
	rolls, err := q.ProviderHealthRollup(ctx)
	if err != nil || len(rolls) != 1 {
		t.Fatalf("rollup = %+v, %v", rolls, err)
	}
	r := rolls[0]
	if time.Since(r.LastSuccessAt.Time) < 59*time.Hour || time.Since(r.FreshnessAt.Time) > time.Minute {
		t.Fatalf("quiet rollup = %+v", r)
	}
	// Stale quiet state must never launder subsequent failure into freshness.
	if _, err := pool.Exec(ctx, `UPDATE board_health SET consecutive_failures=3, cooldown_until=now()+interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	rolls, err = q.ProviderHealthRollup(ctx)
	if err != nil || !rolls[0].FreshnessAt.Time.Equal(r.LastSuccessAt.Time) || rolls[0].CooledBoards != 1 {
		t.Fatalf("failed quiet rollup = %+v, %v", rolls, err)
	}
}
