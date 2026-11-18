package gmailsync

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
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
		out = append(out, Connection{UserID: r.UserID, Email: r.Email, Cursor: r.SyncCursor, SentCursor: r.SentSyncCursor})
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
		Direction: e.Message.Direction, ToAddrs: e.Message.ToAddrs, CcAddrs: e.Message.CcAddrs,
		UserID:     e.UserID,
		ExternalID: e.Message.ID,
		ThreadID:   e.Message.ThreadID,
		FromAddr:   e.Message.FromAddr,
		FromName:   e.Message.FromName,
		Subject:    e.Message.Subject,
		BodyText:   e.Message.BodyText,
		BodyHtml:   e.Message.BodyHTML,
		ReceivedAt: pgtype.Timestamptz{Time: e.Message.ReceivedAt, Valid: true},
		IcalUid:    e.Message.CalendarUID,
	})
}

func (s *DBStore) SetSynced(ctx context.Context, userID, cursorUnix, sentCursor int64) error {
	return s.q.SetGmailSynced(ctx, db.SetGmailSyncedParams{UserID: userID, SyncCursor: cursorUnix, SentSyncCursor: sentCursor})
}

func (s *DBStore) SetNeedsReconsent(ctx context.Context, userID int64) error {
	return s.q.SetGmailStatus(ctx, db.SetGmailStatusParams{UserID: userID, Status: "needs_reconsent"})
}

// Observe records one confident job-mail sighting for a domain (self-learning
// cache write side), returning the running count. Implements LearnedDomainStore.
func (s *DBStore) Observe(ctx context.Context, domain string) (int, error) {
	hits, err := s.q.ObserveLearnedDomain(ctx, domain)
	return int(hits), err
}

// Miss counts one non-job sighting against the learned domain covering host.
// Implements LearnedDomainStore.
func (s *DBStore) Miss(ctx context.Context, host string) error {
	return s.q.MissLearnedDomain(ctx, host)
}

// Promoted returns the domains that reached the promotion threshold with at most
// MaxOtherShare of their mail `other` — the read side the sync worker unions into the query.
func (s *DBStore) Promoted(ctx context.Context) ([]string, error) {
	return s.q.PromotedDomains(ctx, db.PromotedDomainsParams{
		Threshold:     PromoteThreshold,
		MaxOtherShare: MaxOtherShare,
	})
}

func (s *DBStore) ListThreadIDs(ctx context.Context, userID int64) ([]string, error) {
	return s.q.ListGmailThreadIDs(ctx, userID)
}

func (s *DBStore) StoredMessages(ctx context.Context, userID int64, ids []string) (map[string]StoredRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q.ListStoredGmailMessages(ctx, db.ListStoredGmailMessagesParams{UserID: userID, ExternalIds: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[string]StoredRef, len(rows))
	for _, r := range rows {
		out[r.ExternalID] = StoredRef{ThreadID: r.ThreadID, ReceivedAt: r.ReceivedAt.Time, Incoming: r.Direction != DirectionOutgoing}
	}
	return out, nil
}

func (s *DBStore) CooldownUntil(ctx context.Context, userID int64) (time.Time, error) {
	v, err := s.q.GmailCooldownUntil(ctx, userID)
	if v.InfinityModifier != 0 {
		return time.Time{}, err
	}
	return v.Time, err
}
func (s *DBStore) SetCooldown(ctx context.Context, userID int64) error {
	return s.q.SetGmailCooldown(ctx, userID)
}
