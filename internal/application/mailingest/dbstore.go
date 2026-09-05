package mailingest

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// DBStore adapts *db.Queries to the worker's Store.
type DBStore struct {
	q *db.Queries
}

// NewDBStore wraps the generated queries.
func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

// MailboxByAddress resolves an inbound recipient to its owning user: strip the
// domain, look up the local-part as an account username, then confirm that
// account is actually enrolled in the hosted mailbox (a known username with no
// mailbox row is not a recipient — same as an unknown one). See the
// add-username-claim change for why this is no longer a single address lookup:
// the address is derived from users.username rather than stored on mailboxes.
func (s *DBStore) MailboxByAddress(ctx context.Context, address string) (int64, bool, error) {
	local := address
	if at := strings.IndexByte(local, '@'); at >= 0 {
		local = local[:at]
	}
	userID, err := s.q.GetUserIDByUsername(ctx, pgconv.Text(local))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := s.q.GetMailboxByUser(ctx, userID); errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	} else if err != nil {
		return 0, false, err
	}
	return userID, true, nil
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
		IcalUid:    m.CalendarUID,
	})
}
