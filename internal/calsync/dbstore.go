package calsync

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/calmatch"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/gmailsync"
)

// DBStore adapts *db.Queries to the sync worker's Store.
type DBStore struct {
	q *db.Queries
}

// NewDBStore wraps the generated queries.
func NewDBStore(q *db.Queries) *DBStore { return &DBStore{q: q} }

// ListCalendarConnections returns the candidates whose grant actually covers the
// calendar. The scope check is in the query rather than here: a grant that predates the
// calendar consent is not a connection to retry, and finding that out from the API would
// spend a quota unit per candidate per run on an answer we already hold.
func (s *DBStore) ListCalendarConnections(ctx context.Context) ([]Connection, error) {
	rows, err := s.q.ListCalendarConnections(ctx, gmailsync.CalendarScope)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, id := range rows {
		out = append(out, Connection{UserID: id})
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

// Candidates returns the caller's applications with the identifiers of the invitations
// already linked to them — everything calmatch needs, in one query per candidate rather
// than one lookup per calendar event.
func (s *DBStore) Candidates(ctx context.Context, userID int64) ([]calmatch.Candidate, error) {
	rows, err := s.q.ListCalendarMatchCandidates(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]calmatch.Candidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, calmatch.Candidate{
			ApplicationID: r.ApplicationID,
			Company:       r.CompanySlug,
			UIDs:          r.IcalUids,
		})
	}
	return out, nil
}

func (s *DBStore) UpsertInterview(ctx context.Context, in StoredInterview) error {
	_, err := s.q.UpsertApplicationInterview(ctx, db.UpsertApplicationInterviewParams{
		UserID:        in.UserID,
		ApplicationID: in.ApplicationID,
		IcalUid:       in.UID,
		StartsAt:      pgtype.Timestamptz{Time: in.StartsAt, Valid: true},
		// An all-day or open-ended entry has no end, and NULL says so rather than
		// inventing a duration the organiser did not give.
		EndsAt:  pgtype.Timestamptz{Time: in.EndsAt, Valid: !in.EndsAt.IsZero()},
		Title:   in.Title,
		JoinUrl: in.JoinURL,
		Status:  in.Status,
		Source:  SourceGoogleCalendar,
		// The ledger's own vocabulary, passed rather than written into the SQL so the
		// pin test in appevent stays the single place a source is spelled.
		EventSource: appevent.SourceCalendarGoogle,
	})
	return err
}

func (s *DBStore) CancelInterview(ctx context.Context, userID int64, uid string) error {
	_, err := s.q.CancelApplicationInterview(ctx, db.CancelApplicationInterviewParams{
		UserID: userID, IcalUid: uid,
	})
	return err
}

// SetNeedsReconsent flags the grant, which is shared with mail: one Google grant covers
// both scopes, so a revoked one takes the mailbox with it and the candidate is asked once
// rather than twice.
func (s *DBStore) SetNeedsReconsent(ctx context.Context, userID int64) error {
	return s.q.SetGmailStatus(ctx, db.SetGmailStatusParams{UserID: userID, Status: "needs_reconsent"})
}
