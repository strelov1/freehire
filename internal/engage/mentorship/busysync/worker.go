// Package busysync reads a mentor's Google Calendar free/busy state and keeps
// mentor_busy_intervals current with it — the calendar-sync half
// mentor-google-meet-link's design deferred, finally shipped here.
//
// It mirrors internal/application/calsync deliberately: a narrow Store over db.Queries, a
// reader behind a factory so the worker is unit-tested with a fake, and a best-effort pass
// in which one mentor's revoked grant does not stop the rest. It differs from calsync in
// what it reads and how it reconciles: calsync matches individual events against
// applications and needs their identifiers, while this package only ever needs to know
// whether a stretch of time is occupied — so it reads Google's freeBusy endpoint (bounds
// only, no title, no attendee, no identifier at all) and reconciles by replacing the whole
// synced window each run rather than diffing individual rows. See
// openspec/changes/mentor-calendar-busy-sync/design.md for why.
package busysync

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// busyWindowDays is the fixed forward window every run reads and reconciles — not tied
// to any mentor's own booking horizon (a per-mentor, per-session-type value), which would
// need a join and per-mentor branching for a window a slot-engine read already narrows
// further. Forward only: past busy time can never block a future booking.
const busyWindowDays = 60

// Connection is a mentor whose grant covers busy-sync — both ids, since the grant is
// keyed by user_id but the busy set it feeds is keyed by mentor_id.
type Connection struct {
	MentorID int64
	UserID   int64
}

// BusyPeriod is one interval Google reports as occupied. Bounds only, deliberately: see
// the package doc for why this worker never reads more.
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
	// windowEnd: every currently-reported period is written, and nothing else in that
	// window survives. Transactional, so a concurrent booking attempt's ListBusy never
	// observes a partially-reconciled set.
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
// Best-effort per mentor, and the count of failures comes back as an error so the
// command can exit non-zero. A run that swallowed a revoked grant would look identical
// to a run with nothing to do, which is how a broken sync stays broken for weeks.
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
		// Only the grant saying no costs the mentor their connection. A 500 or a rate
		// limit is Google having a bad day, and the flag is SHARED with every other
		// Google feature this account may use — treating every failure as a revocation
		// would disconnect it during one Google incident.
		//
		// A real revocation does not arrive as a freeBusy 403 at all: the client is built
		// from a stored refresh token, so Google refuses it at the TOKEN endpoint before
		// this request is made. RevokedGrant covers both carriers.
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
