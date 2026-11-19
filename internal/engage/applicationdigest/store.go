package applicationdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/silence"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

type DBStore struct{ q *db.Queries }

func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }
func (s *DBStore) Candidates(ctx context.Context, now time.Time, after int64, limit int32) ([]Candidate, error) {
	rows, err := s.q.ListApplicationDigestCandidates(ctx, db.ListApplicationDigestCandidatesParams{NowAt: pgconv.Instant(now), BatchSize: limit, AfterUserID: after})
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
	r, err := s.q.GetApplicationDigestSnapshot(ctx, db.GetApplicationDigestSnapshotParams{UserID: c.UserID, NowAt: pgconv.Instant(now)})
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{ActivityCount: r.ActivityCount, AsOf: now, AppliedCount: r.Applied, RepliesCount: r.Replies, InterviewCount: r.Interviews, OfferCount: r.Offers, StageCounts: map[string]int64{}}
	var stages []stageCount
	if err := json.Unmarshal(r.Stages, &stages); err != nil {
		return Snapshot{}, fmt.Errorf("decode application stages: %w", err)
	}
	for _, stage := range stages {
		out.StageCounts[stage.Stage] = stage.Count
	}
	rows, err := s.q.ListApplicationDigestSilentCandidates(ctx, db.ListApplicationDigestSilentCandidatesParams{UserID: c.UserID, NowAt: pgconv.Instant(now)})
	if err != nil {
		return Snapshot{}, err
	}
	out.Followups = selectFollowups(rows, now)
	return out, nil
}
func selectFollowups(rows []db.ListApplicationDigestSilentCandidatesRow, now time.Time) []Followup {
	var out []Followup
	for _, r := range rows {
		if !r.AppliedAt.Valid {
			continue
		}
		last := r.AppliedAt.Time
		if r.LastActivityAt.Valid {
			last = r.LastActivityAt.Time
		}
		days := silence.Days(now, last)
		if silence.StateFor(r.Stage, days, r.HasPendingSuggestion) != silence.Silent {
			continue
		}
		item := Followup{ID: r.ID, RoleTitle: r.RoleTitle, CompanySlug: r.CompanySlug, JobSlug: r.JobSlug, Stage: r.Stage, DaysSilent: days}
		if r.FollowedUpAt.Valid {
			item.FollowedUpAt = r.FollowedUpAt.Time
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysSilent == out[j].DaysSilent {
			return out[i].ID < out[j].ID
		}
		return out[i].DaysSilent > out[j].DaysSilent
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
func (s *DBStore) Claim(ctx context.Context, c Candidate, _ Snapshot, now time.Time) (bool, error) {
	n, err := s.q.ClaimApplicationDigest(ctx, db.ClaimApplicationDigestParams{UserID: c.UserID, Email: c.Email, Timezone: c.Timezone, EnabledAt: pgconv.Instant(c.EnabledAt), NowAt: pgconv.Instant(now), LocalWeek: localWeek(c, now)})
	return n == 1, err
}
func (s *DBStore) MarkSent(ctx context.Context, c Candidate, now time.Time) error {
	n, err := s.q.MarkApplicationDigestSent(ctx, db.MarkApplicationDigestSentParams{UserID: c.UserID, LocalWeek: localWeek(c, now), SentAt: pgconv.Instant(now)})
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("digest week claim disappeared")
	}
	return nil
}
func (s *DBStore) ReleasePaused(ctx context.Context, c Candidate, now time.Time) error {
	return s.q.ReleasePausedApplicationDigest(ctx, db.ReleasePausedApplicationDigestParams{UserID: c.UserID, LocalWeek: localWeek(c, now)})
}
func localWeek(c Candidate, now time.Time) pgtype.Date {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(local.Weekday()) + 6) % 7
	return pgtype.Date{Time: day.AddDate(0, 0, -offset), Valid: true}
}
