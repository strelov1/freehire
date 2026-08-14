// Package calsync reads a candidate's calendar for the interviews their applications
// have earned, and stores nothing else.
//
// It mirrors internal/gmailsync deliberately: a narrow Store over db.Queries, a reader
// behind a factory so the worker is unit-tested with a fake, and a best-effort pass in
// which one candidate's revoked grant does not stop the rest. The two syncs share a
// Google grant and nothing else — different APIs, different windows, different
// persistence — so they are separate packages rather than one with a mode.
//
// The rule that matters here is what does NOT happen. A calendar holds medical
// appointments, family, a current employer's meetings, and interviews with employers the
// candidate never told us about. The window is read into memory, matched, and discarded;
// only a meeting internal/calmatch could attach to one of the candidate's own
// applications is written. The schema enforces the same thing from below —
// application_interviews.application_id is NOT NULL — so a mistake here cannot become a
// stored dentist appointment.
package calsync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/calmatch"
	"github.com/strelov1/freehire/internal/tokencrypt"
)

// The window read on each run: enough behind to recover interviews already sat, enough
// ahead to hold anything arranged. One page for a normal calendar.
const windowDays = 90

// SourceGoogleCalendar names where a meeting was read from, in the manner of
// application_events.source. A subscribed ICS feed would be a second value and nothing
// else would change.
const SourceGoogleCalendar = "calendar_google"

// StatusConfirmed is the only status this worker writes: a meeting the invitation's own
// identifier attached. The schema also knows `cancelled`, which the cancel path sets.
//
// There is no `suggested`. A weaker tier would need a way for the candidate to confirm or
// dismiss what it produced, and until that exists a guess would be permanent — see
// internal/calmatch for what the first attempt at one actually did.
const StatusConfirmed = "confirmed"

// Connection is a candidate whose Google grant covers the calendar. The query filters on
// the recorded scopes, so a grant that predates the calendar consent never reaches here
// and never costs an API call to discover.
//
// Only the id: gmailsync carries the address to skip the candidate's own replies to a
// thread, and a calendar has no equivalent notion. A field nobody reads is a question the
// next reader has to answer.
type Connection struct {
	UserID int64
}

// Meeting is one calendar entry as the reader returns it — the fields needed to keep an
// appointment, and none of the ones that describe a private life. No attendees and no
// description. Title is carried because a matched meeting shows one; nothing MATCHES on
// it, which is the difference between a label and evidence.
type Meeting struct {
	UID string
	// ProviderID is the calendar provider's own event id — a different thing from UID,
	// and the only identifier a cancellation is guaranteed to carry.
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

// CalendarReader reads one candidate's events over a window. Behind an interface so the
// worker's rules are tested without Google.
type CalendarReader interface {
	ListEvents(ctx context.Context, from, to time.Time) ([]Meeting, error)
}

// ReaderFactory builds a reader from a decrypted refresh token.
type ReaderFactory func(ctx context.Context, refreshToken string) CalendarReader

// Store is the persistence the worker needs — a subset of db.Queries.
//
// There is deliberately no way to store an unattached meeting: the only write it offers
// carries an application id, so the privacy rule is not something a caller has to
// remember.
type Store interface {
	ListCalendarConnections(ctx context.Context) ([]Connection, error)
	RefreshToken(ctx context.Context, userID int64) (encToken string, err error)
	Candidates(ctx context.Context, userID int64) ([]calmatch.Candidate, error)
	UpsertInterview(ctx context.Context, in StoredInterview) error
	// CancelInterview matches on either identifier: a cancellation may name the meeting
	// by its iCalUID or by the provider's own event id, and Google guarantees only the
	// second.
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
// Best-effort per candidate, and the count of failures comes back as an error so the
// command can exit non-zero. A run that swallowed a revoked grant would look identical
// to a run with nothing to do, which is how a broken sync stays broken for weeks.
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

// cancelKey names the meeting a cancellation refers to. The iCalUID when it survived,
// the provider's own id otherwise — a deleted event is documented to carry only the
// second, and the store matches on either.
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
		// Only the grant saying no costs the candidate their connection. A 500 or a rate
		// limit is Google having a bad day, and the flag is SHARED with mail — treating
		// every failure as a revocation would disconnect every mailbox we hold during one
		// Google incident, each needing a restricted-scope consent to restore.
		var authErr *AuthError
		if !errors.As(err, &authErr) {
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
			// Ahead of the match gate, not behind it: Google guarantees only the
			// provider's own id for a cancelled event, never the iCalUID Resolve keys
			// on, so gating on Resolve first would drop every cancellation whose UID
			// didn't survive. Marking beats storing: the row may already exist, and an
			// organiser who called a meeting off has not scheduled a new one.
			if err := w.store.CancelInterview(ctx, u.UserID, cancelKey(ev)); err != nil {
				return fmt.Errorf("cancel %s: %w", cancelKey(ev), err)
			}
			continue
		}
		match := calmatch.Resolve(calmatch.Event{UID: ev.UID}, candidates)
		if match.ApplicationID == 0 {
			// Not this candidate's business with us. It is not stored, not logged, and
			// not counted — the window was read into memory and is discarded with it.
			continue
		}
		if !match.Tier.Links() {
			// Resolved to an application by something weaker than the invitation's own
			// identifier. There is nothing weaker today, and the guard stays so that
			// adding one cannot start writing rows before somebody decides it may.
			continue
		}
		if err := w.store.UpsertInterview(ctx, StoredInterview{
			UserID:        u.UserID,
			ApplicationID: match.ApplicationID,
			UID:           ev.UID,
			Title:         ev.Title,
			StartsAt:      ev.StartsAt,
			EndsAt:        ev.EndsAt,
			JoinURL:       ev.JoinURL,
			Status:        StatusConfirmed,
		}); err != nil {
			return fmt.Errorf("store %s: %w", ev.UID, err)
		}
	}
	return nil
}
