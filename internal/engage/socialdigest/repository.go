package socialdigest

import (
	"context"
	"fmt"
	"html"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// PostgresRepository is the Repository backed by the generated queries. It holds no
// rules — the day guard, the floor, the cap and the quarantine window all live in the
// package above it, where they can be read and changed without a database.
type PostgresRepository struct {
	q *db.Queries
}

// NewPostgresRepository wraps the generated query set.
func NewPostgresRepository(q *db.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) LatestViewDay(ctx context.Context) (time.Time, bool, error) {
	d, err := r.q.LatestJobViewDay(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	// max() over an empty table is SQL NULL, which arrives as an invalid Date. That is
	// "the rollup has produced nothing", not "the newest day is the zero time".
	if !d.Valid {
		return time.Time{}, false, nil
	}
	return d.Time, true, nil
}

func (r *PostgresRepository) TopPageViewed(ctx context.Context, day time.Time, limit int) ([]Posting, error) {
	rows, err := r.q.TopPageViewedJobsForDay(ctx, db.TopPageViewedJobsForDayParams{
		Day: pgDate(day),
		Lim: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Posting, 0, len(rows))
	for _, row := range rows {
		out = append(out, postingFromRow(row))
	}
	return out, nil
}

// postingFromRow turns one stored row into something publishable.
//
// The HTML entities are unescaped here, at the one boundary where stored text becomes
// text we are about to show, rather than in each publisher. Titles arrive from source
// feeds carrying them — "Senior Specialist- Learning Design &amp; Capacity" is a real
// row that would have gone out with the "&amp;" in it — and every channel would
// otherwise have to remember this separately, which is the kind of thing exactly one
// of them eventually forgets.
func postingFromRow(row db.TopPageViewedJobsForDayRow) Posting {
	return Posting{
		JobID:       row.ID,
		Slug:        row.PublicSlug,
		Title:       html.UnescapeString(row.Title),
		Company:     html.UnescapeString(row.Company),
		CompanySlug: row.CompanySlug,
		Location:    html.UnescapeString(row.Location),
		Remote:      row.Remote,
		PageUniques: int(row.PageUniques),
	}
}

func (r *PostgresRepository) RecentlyDigested(ctx context.Context, since, before time.Time) (map[int64]bool, error) {
	ids, err := r.q.RecentlyDigestedJobIDs(ctx, db.RecentlyDigestedJobIDsParams{
		Since:  pgDate(since),
		Before: pgDate(before),
	})
	if err != nil {
		return nil, err
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

func (r *PostgresRepository) PublishedForChannel(ctx context.Context, day time.Time, channel string) (bool, error) {
	return r.q.DigestPublishedForChannel(ctx, db.DigestPublishedForChannelParams{
		Day:     pgDate(day),
		Channel: channel,
	})
}

func (r *PostgresRepository) RecordPublished(ctx context.Context, day time.Time, channel string, items []Posting) error {
	if len(items) == 0 {
		return nil
	}
	params := make([]db.RecordDigestPostParams, 0, len(items))
	for i, item := range items {
		params = append(params, db.RecordDigestPostParams{
			Day:     pgDate(day),
			Channel: channel,
			JobID:   item.JobID,
			Slot:    int32(i + 1),
		})
	}

	br := r.q.RecordDigestPost(ctx, params)
	var firstErr error
	br.Exec(func(_ int, err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	})
	if cerr := br.Close(); cerr != nil && firstErr == nil {
		firstErr = cerr
	}
	if firstErr != nil {
		return fmt.Errorf("record %d digest rows: %w", len(params), firstErr)
	}
	return nil
}

func pgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: truncateDay(t), Valid: true}
}
