package mailrecall

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// DBStore adapts *db.Queries to the service's Store.
type DBStore struct {
	q *db.Queries
}

// NewDBStore wraps the generated queries.
func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

// ListForRecall runs the net. Both body columns come across unresolved: choosing between
// them is the service's rule, and doing it here would hide it from unit tests.
func (s *DBStore) ListForRecall(ctx context.Context, userID int64, since, until time.Time, limit int32) ([]Message, error) {
	rows, err := s.q.ListEmailsForRecall(ctx, db.ListEmailsForRecallParams{
		UserID: userID,
		Since:  pgtype.Timestamptz{Time: since, Valid: true},
		Until:  pgtype.Timestamptz{Time: until, Valid: true},
		Lim:    limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, Message{
			ID: r.ID, FromAddr: r.FromAddr, FromName: r.FromName, Subject: r.Subject,
			BodyText: r.BodyText, BodyHTML: r.BodyHtml,
			ReceivedAt: r.ReceivedAt.Time, ICalUID: r.IcalUid,
		})
	}
	return out, nil
}

// Suggest records the proposal. The statement changes nothing on linked or deleted mail,
// so a candidate claimed underneath the run reports zero rows instead of overwriting a link.
func (s *DBStore) Suggest(ctx context.Context, emailID, userID, jobID int64, confidence float32) (int64, error) {
	return s.q.SuggestJobForEmail(ctx, db.SuggestJobForEmailParams{
		ID: emailID, UserID: userID, SuggestedJobID: jobID, Confidence: confidence,
	})
}
