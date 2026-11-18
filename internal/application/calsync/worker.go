// Package calsync reads a candidate's calendar for the interviews their applications
// have earned, and stores nothing else.
//
// It mirrors internal/application/gmailsync (narrow Store, reader behind a factory, best-effort
// pass per candidate) but shares only the Google grant with it, so it is a separate package.
//
// A calendar holds private appointments and meetings with employers the candidate never told us
// about. The window is read into memory, matched, and discarded; only a meeting
// internal/application/calmatch could attach to one of the candidate's own applications is
// written. application_interviews.application_id NOT NULL enforces this from below.
package calsync

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/application/calmatch"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// The window read on each run: behind enough to recover interviews already sat, ahead enough
// to hold anything arranged.
const windowDays = 90

// SourceGoogleCalendar names where a meeting was read from, like application_events.source.
const SourceGoogleCalendar = "calendar_google"

// StatusConfirmed is the only status this worker writes: a meeting the invitation's own
// identifier attached. The cancel path sets `cancelled`.
//
// There is no `suggested`: without a way for the candidate to confirm or dismiss it, a guess
// would be permanent (see internal/application/calmatch).
const StatusConfirmed = "confirmed"

// Connection is a candidate whose Google grant covers the calendar. The query filters on the
// recorded scopes, so a grant that predates the calendar consent never costs an API call.
type Connection struct {
	UserID int64
}

// Meeting is one calendar entry as the reader returns it. No attendees and no description.
// Title is shown for a matched meeting; nothing matches on it.
type Meeting struct {
	UID string
	// ProviderID is the provider's own event id, distinct from UID and the only identifier a
	// cancellation is guaranteed to carry.
	ProviderID string
	Title      string
	StartsAt   time.Time
	EndsAt     time.Time
	JoinURL    string
	Cancelled  bool
}

// StoredInterview is a meeting that attached to an application, ready to persist.
type StoredInterview struct {
	UserID        int64
	ApplicationID int64
	UID           string
	ProviderID    string
	Title         string
	StartsAt      time.Time
	EndsAt        time.Time
	JoinURL       string
	Status        string
}

// CalendarReader reads one candidate's events over a window.
type CalendarReader interface {
	ListEvents(ctx context.Context, from, to time.Time) ([]Meeting, error)
}

// ReaderFactory builds a reader from a decrypted refresh token.
type ReaderFactory func(ctx context.Context, refreshToken string) CalendarReader

// Store is the persistence the worker needs — a subset of db.Queries. It has no way to store an
// unattached meeting: the only write carries an application id.
type Store interface {
	ListCalendarConnections(ctx context.Context) ([]Connection, error)
	RefreshToken(ctx context.Context, userID int64) (encToken string, err error)
	Candidates(ctx context.Context, userID int64) ([]calmatch.Candidate, error)
	UpsertInterview(ctx context.Context, in StoredInterview) error
	// CancelInterview matches on either the iCalUID or the provider's event id.
	CancelInterview(ctx context.Context, userID int64, eventID string) error
	SetNeedsReconsent(ctx context.Context, userID int64) error
}

// Worker syncs every connected candidate's interviews.
type Worker struct {
	store     Store
	cipher    *tokencrypt.Cipher
	newReader ReaderFactory
	// now is injected so the window is deterministic under test.
	now func() time.Time
}

// NewWorker builds the sync worker.
func NewWorker(store Store, cipher *tokencrypt.Cipher, newReader ReaderFactory) *Worker {
	return &Worker{store: store, cipher: cipher, newReader: newReader, now: time.Now}
}

// RunOnce syncs every connected candidate once.
//
// Best-effort per candidate; the failure count comes back as an error so the command exits
// non-zero. Swallowed failures would look like a run with nothing to do.
func (w *Worker) RunOnce(ctx context.Context) error {
	users, err := w.store.ListCalendarConnections(ctx)
	if err != nil {
		return err
	}
	var failed int
	for _, u := range users {
		if err := w.syncUser(ctx, u); err != nil {
			failed++
			log.Printf("cal-sync: user %d: %v", u.UserID, err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("cal-sync: %d of %d connections failed", failed, len(users))
	}
	return nil
}

// cancelKey names the meeting a cancellation refers to: the iCalUID when it survived, else the
// provider's id (a deleted event carries only that).
func cancelKey(ev Meeting) string {
	if ev.UID != "" {
		return ev.UID
	}
	return ev.ProviderID
}

func (w *Worker) syncUser(ctx context.Context, u Connection) error {
	encToken, err := w.store.RefreshToken(ctx, u.UserID)
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}
	refresh, err := w.cipher.Decrypt(encToken)
	if err != nil {
		return fmt.Errorf("decrypt token: %w", err)
	}

	now := w.now()
	events, err := w.newReader(ctx, refresh).
		ListEvents(ctx, now.AddDate(0, 0, -windowDays), now.AddDate(0, 0, windowDays))
	if err != nil {
		// Only a revoked grant disconnects the candidate. The flag is shared with mail, so
		// treating a 500 or rate limit as revocation would disconnect every mailbox during a
		// Google incident. A real revocation is refused at the token endpoint, not as a
		// calendar 403; RevokedGrant covers both.
		if !gmailsync.RevokedGrant(err) {
			return fmt.Errorf("list events: %w", err)
		}
		if markErr := w.store.SetNeedsReconsent(ctx, u.UserID); markErr != nil {
			return fmt.Errorf("list events: %w (and marking re-consent failed: %v)", err, markErr)
		}
		return fmt.Errorf("list events: %w — marked needs_reconsent", err)
	}

	candidates, err := w.store.Candidates(ctx, u.UserID)
	if err != nil {
		return fmt.Errorf("load candidates: %w", err)
	}

	for _, ev := range events {
		if ev.Cancelled {
			// Before the match gate: a cancelled event carries only the provider id, not the
			// iCalUID Resolve keys on, so gating first would drop it. Marking beats storing:
			// the row may already exist.
			if err := w.store.CancelInterview(ctx, u.UserID, cancelKey(ev)); err != nil {
				return fmt.Errorf("cancel %s: %w", cancelKey(ev), err)
			}
			continue
		}
		match := calmatch.Resolve(calmatch.Event{UID: ev.UID}, candidates)
		if match.ApplicationID == 0 {
			// Not this candidate's business with us: not stored, logged or counted.
			continue
		}
		if !match.Tier.Links() {
			// Resolved by something weaker than the invitation's identifier. None exists today;
			// the guard keeps a new tier from writing rows before it is decided.
			continue
		}
		if err := w.store.UpsertInterview(ctx, StoredInterview{
			UserID:        u.UserID,
			ApplicationID: match.ApplicationID,
			UID:           ev.UID,
			// Must be stored: a cancelled occurrence of a recurring series arrives with no
			// iCalUID, so CancelApplicationInterview can only match on this.
			ProviderID: ev.ProviderID,
			Title:      ev.Title,
			StartsAt:   ev.StartsAt,
			EndsAt:     ev.EndsAt,
			JoinURL:    ev.JoinURL,
			Status:     StatusConfirmed,
		}); err != nil {
			return fmt.Errorf("store %s: %w", ev.UID, err)
		}
	}
	return nil
}
