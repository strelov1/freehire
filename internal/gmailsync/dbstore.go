package gmailsync

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// DBStore adapts *db.Queries to the sync worker's Store.
type DBStore struct {
	q *db.Queries
}

// NewDBStore wraps the generated queries.
func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

func (s *DBStore) ListConnected(ctx context.Context) ([]Connection, error) {
	rows, err := s.q.ListConnectedGmailUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, Connection{UserID: r.UserID, Email: r.Email, Cursor: r.SyncCursor})
	}
	return out, nil
}

func (s *DBStore) RefreshToken(ctx context.Context, userID int64) (string, error) {
	r, err := s.q.GetGmailRefreshToken(ctx, userID)
	if err != nil {
		return "", err
	}
	return r.RefreshTokenEnc, nil
}

func (s *DBStore) UpsertEmail(ctx context.Context, e StoredEmail) error {
	return s.q.UpsertEmail(ctx, db.UpsertEmailParams{
		UserID:      e.UserID,
		GmailMsgID:  e.Message.ID,
		ThreadID:    e.Message.ThreadID,
		FromAddr:    e.Message.FromAddr,
		FromName:    e.Message.FromName,
		Subject:     e.Message.Subject,
		SubjectNorm: e.SubjectNorm,
		BodyText:    e.Message.BodyText,
		BodyHtml:    e.Message.BodyHTML,
		ReceivedAt:  pgtype.Timestamptz{Time: e.Message.ReceivedAt, Valid: true},
	})
}

func (s *DBStore) SetSynced(ctx context.Context, userID, cursorUnix int64) error {
	return s.q.SetGmailSynced(ctx, db.SetGmailSyncedParams{UserID: userID, SyncCursor: cursorUnix})
}

func (s *DBStore) SetNeedsReconsent(ctx context.Context, userID int64) error {
	return s.q.SetGmailStatus(ctx, db.SetGmailStatusParams{UserID: userID, Status: "needs_reconsent"})
}
