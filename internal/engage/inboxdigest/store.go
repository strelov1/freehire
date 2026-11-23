package inboxdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

type DBStore struct{ q *db.Queries }

func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

func (s *DBStore) Candidates(ctx context.Context, now time.Time, limit int32) ([]Candidate, error) {
	rows, err := s.q.ListInboxDigestCandidates(ctx, db.ListInboxDigestCandidatesParams{NowAt: pgconv.Instant(now), BatchSize: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, Candidate{UserID: r.UserID, Email: r.Email, Timezone: r.Timezone, EnabledAt: r.EnabledAt.Time})
	}
	return out, nil
}
func (s *DBStore) Snapshot(ctx context.Context, c Candidate, now time.Time) (Snapshot, error) {
	row, err := s.q.GetInboxDigestSnapshot(ctx, db.GetInboxDigestSnapshotParams{NowAt: pgconv.Instant(now), UserID: c.UserID, EnabledAt: pgconv.Instant(c.EnabledAt)})
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{IDs: row.EmailIds, Total: row.Total, ActivityIDs: row.SchedulingActivityIds}
	if err := json.Unmarshal(row.Messages, &snapshot.Items); err != nil {
		return Snapshot{}, fmt.Errorf("decode inbox snapshot: %w", err)
	}
	if err := json.Unmarshal(row.SchedulingActivity, &snapshot.Activity); err != nil {
		return Snapshot{}, fmt.Errorf("decode scheduling activity: %w", err)
	}
	return snapshot, nil
}
func (s *DBStore) Claim(ctx context.Context, c Candidate, snapshot Snapshot, now time.Time) (bool, error) {
	n, err := s.q.ClaimInboxDigest(ctx, db.ClaimInboxDigestParams{UserID: c.UserID, Email: c.Email, Timezone: c.Timezone, EnabledAt: pgconv.Instant(c.EnabledAt), EmailIds: snapshot.IDs, SchedulingActivityIds: snapshot.ActivityIDs, NowAt: pgconv.Instant(now), LocalDay: localDay(c, now)})
	return n == 1, err
}
func (s *DBStore) MarkSent(ctx context.Context, c Candidate, now time.Time) error {
	n, err := s.q.MarkInboxDigestSent(ctx, db.MarkInboxDigestSentParams{UserID: c.UserID, LocalDay: localDay(c, now), SentAt: pgconv.Instant(now)})
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("digest day claim disappeared")
	}
	return nil
}
func (s *DBStore) ReleasePaused(ctx context.Context, c Candidate, now time.Time) error {
	return s.q.ReleasePausedInboxDigest(ctx, db.ReleasePausedInboxDigestParams{UserID: c.UserID, LocalDay: localDay(c, now)})
}
func localDay(c Candidate, now time.Time) pgtype.Date {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	return pgtype.Date{Time: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}
