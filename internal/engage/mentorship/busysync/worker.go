// Package busysync reads a mentor's Google Calendar free/busy state and keeps
// mentor_busy_intervals current with it.
//
// It mirrors internal/application/calsync: a narrow Store over db.Queries, a reader behind
// a factory, and a best-effort pass in which one mentor's revoked grant does not stop the
// rest. Unlike calsync it only needs to know whether time is occupied, so it reads the
// freeBusy endpoint (bounds only) and replaces the whole synced window each run rather
// than diffing rows.
package busysync

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// busyWindowDays is the fixed forward window every run reads and reconciles, not tied to
// a mentor's booking horizon. Forward only: past busy time never blocks a booking.
const busyWindowDays = 60

// Connection is a mentor whose grant covers busy-sync — both ids, since the grant is
// keyed by user_id but the busy set it feeds is keyed by mentor_id.
type Connection struct {
	MentorID int64
	UserID   int64
}

// BusyPeriod is one interval Google reports as occupied. Bounds only, by design.
type BusyPeriod struct {
	Start time.Time
	End   time.Time
}

// FreeBusyReader reads one mentor's busy periods over a window. Behind an interface so
// the worker's rules are tested without Google.
type FreeBusyReader interface {
	ListBusy(ctx context.Context, from, to time.Time) ([]BusyPeriod, error)
}

// ReaderFactory builds a reader from a decrypted refresh token.
type ReaderFactory func(ctx context.Context, refreshToken string) FreeBusyReader

// Store is the persistence the worker needs.
type Store interface {
	ListConnections(ctx context.Context) ([]Connection, error)
	RefreshToken(ctx context.Context, userID int64) (encToken string, err error)
	// ReplaceBusyWindow reconciles one mentor's stored busy set for the window ending at
	// windowEnd: reported periods are written and nothing else in the window survives.
	// Transactional, so ListBusy never sees a partial set.
	ReplaceBusyWindow(ctx context.Context, mentorID int64, windowEnd time.Time, periods []BusyPeriod) error
	SetNeedsReconsent(ctx context.Context, userID int64) error
}

// Worker syncs every connected mentor's busy time once.
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

// RunOnce syncs every connected mentor once.
//
// Best-effort per mentor; the failure count comes back as an error so the command exits
// non-zero, since a swallowed failure looks like a run with nothing to do.
func (w *Worker) RunOnce(ctx context.Context) error {
	conns, err := w.store.ListConnections(ctx)
	if err != nil {
		return err
	}
	var failed int
	for _, c := range conns {
		if err := w.syncMentor(ctx, c); err != nil {
			failed++
			log.Printf("mentor-busy-sync: mentor %d: %v", c.MentorID, err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("mentor-busy-sync: %d of %d connections failed", failed, len(conns))
	}
	return nil
}

func (w *Worker) syncMentor(ctx context.Context, c Connection) error {
	encToken, err := w.store.RefreshToken(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}
	refresh, err := w.cipher.Decrypt(encToken)
	if err != nil {
		return fmt.Errorf("decrypt token: %w", err)
	}

	now := w.now()
	windowEnd := now.AddDate(0, 0, busyWindowDays)
	periods, err := w.newReader(ctx, refresh).ListBusy(ctx, now, windowEnd)
	if err != nil {
		// Only the grant saying no costs the mentor their connection: the flag is shared with
		// every other Google feature, so a 500 or rate limit must not disconnect it.
		//
		// A real revocation is usually refused at the token endpoint, before this request is
		// made; RevokedGrant covers both carriers.
		if !gmailsync.RevokedGrant(err) {
			return fmt.Errorf("list busy: %w", err)
		}
		if markErr := w.store.SetNeedsReconsent(ctx, c.UserID); markErr != nil {
			return fmt.Errorf("list busy: %w (and marking re-consent failed: %v)", err, markErr)
		}
		return fmt.Errorf("list busy: %w — marked needs_reconsent", err)
	}

	if err := w.store.ReplaceBusyWindow(ctx, c.MentorID, windowEnd, periods); err != nil {
		return fmt.Errorf("replace busy window: %w", err)
	}
	return nil
}
