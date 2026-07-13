package mailingest

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// DBStore adapts *db.Queries to the worker's Store.
type DBStore struct {
	q *db.Queries
}

// NewDBStore wraps the generated queries.
func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

func (s *DBStore) MailboxByAddress(ctx context.Context, address string) (int64, bool, error) {
	mb, err := s.q.GetMailboxByAddress(ctx, address)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return mb.UserID, true, nil
}

func (s *DBStore) InsertMessage(ctx context.Context, m HostedMessage) error {
	return s.q.InsertHostedMessage(ctx, db.InsertHostedMessageParams{
		UserID:     m.UserID,
		ExternalID: m.ExternalID,
		S3Key:      pgtype.Text{String: m.S3Key, Valid: m.S3Key != ""},
		FromAddr:   m.FromAddr,
		FromName:   m.FromName,
		Subject:    m.Subject,
		BodyText:   m.BodyText,
		BodyHtml:   m.BodyHTML,
		ReceivedAt: pgtype.Timestamptz{Time: m.ReceivedAt, Valid: true},
	})
}
