package gmailsync

import (
	"context"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/tokencrypt"
)

// Connection is a connected user the sync worker processes.
type Connection struct {
	UserID int64
	Email  string
	Cursor int64 // Unix watermark of the newest synced message
}

// StoredEmail is a fetched message ready to persist.
type StoredEmail struct {
	UserID  int64
	Message Message
}

// Store is the persistence the worker needs (subset of db.Queries).
type Store interface {
	ListConnected(ctx context.Context) ([]Connection, error)
	RefreshToken(ctx context.Context, userID int64) (encToken string, err error)
	UpsertEmail(ctx context.Context, e StoredEmail) error
	SetSynced(ctx context.Context, userID, cursorUnix int64) error
	SetNeedsReconsent(ctx context.Context, userID int64) error
}

// ReaderFactory builds a GmailReader for a user from their (decrypted) refresh
// token and the promoted learned domains — the real factory uses the Connector +
// Gmail API; tests inject a fake.
type ReaderFactory func(ctx context.Context, refreshToken string, learned []string) GmailReader

// Worker syncs every connected user's ATS mail. Best-effort per user: a token or
// API failure marks that user and continues.
type Worker struct {
	store     Store
	cipher    *tokencrypt.Cipher
	newReader ReaderFactory
	domains   DomainSource
}

// NewWorker builds the sync worker. Learned domains default to none (hardcoded
// core only); call WithLearnedDomains to wire the self-learning cache.
func NewWorker(store Store, cipher *tokencrypt.Cipher, newReader ReaderFactory) *Worker {
	return &Worker{store: store, cipher: cipher, newReader: newReader, domains: noLearnedDomains{}}
}

// WithLearnedDomains wires the self-learning ATS-domain cache as the query's
// promoted-domain source, returning the worker for chaining.
func (w *Worker) WithLearnedDomains(src DomainSource) *Worker {
	w.domains = src
	return w
}

// RunOnce syncs all connected users once.
func (w *Worker) RunOnce(ctx context.Context) error {
	users, err := w.store.ListConnected(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		w.syncUser(ctx, u)
	}
	return nil
}

// SyncUser syncs one connected user's mail on demand (a manual refresh from the
// inbox), reusing the same best-effort per-user path as the cron worker.
func (w *Worker) SyncUser(ctx context.Context, u Connection) { w.syncUser(ctx, u) }

func (w *Worker) syncUser(ctx context.Context, u Connection) {
	encToken, err := w.store.RefreshToken(ctx, u.UserID)
	if err != nil {
		log.Printf("gmail-sync: user %d: read token: %v", u.UserID, err)
		return
	}
	refresh, err := w.cipher.Decrypt(encToken)
	if err != nil {
		log.Printf("gmail-sync: user %d: decrypt token: %v", u.UserID, err)
		return
	}
	learned, err := w.domains.Promoted(ctx)
	if err != nil {
		// The learned cache is an enhancement, not a gate: on error fall back to
		// the hardcoded core rather than skipping the user's sync entirely.
		log.Printf("gmail-sync: user %d: load learned domains: %v — using core only", u.UserID, err)
		learned = nil
	}
	reader := w.newReader(ctx, refresh, learned)

	ids, err := reader.ListATSMessageIDs(ctx, u.Email, u.Cursor)
	if err != nil {
		// A revoked/expired grant surfaces here; flag it for re-consent and move on.
		log.Printf("gmail-sync: user %d: list: %v — marking needs_reconsent", u.UserID, err)
		if err := w.store.SetNeedsReconsent(ctx, u.UserID); err != nil {
			log.Printf("gmail-sync: user %d: set status: %v", u.UserID, err)
		}
		return
	}

	newest := u.Cursor
	sawFailure := false
	seen := make(map[string]bool)
	seenThread := make(map[string]bool)
	var threadIDs []string

	// fetch persists one message, deduping by id, tracking the watermark, and
	// recording its thread for expansion. The user's own in-thread replies are
	// skipped — the inbox stores inbound mail only. newest tracks the highest
	// ReceivedAt seen regardless of order; once any message in the wave fails,
	// it is reset to u.Cursor right before SetSynced (below) rather than merely
	// frozen at whatever it last reached — a newer message can easily succeed
	// (and advance newest) BEFORE an older sibling in the same wave fails, and
	// freezing in place would still let the watermark jump past the failed one.
	fetch := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		msg, err := reader.GetMessage(ctx, id)
		if err != nil {
			log.Printf("gmail-sync: user %d: get %s: %v", u.UserID, id, err)
			sawFailure = true
			return
		}
		if msg.ThreadID != "" && !seenThread[msg.ThreadID] {
			seenThread[msg.ThreadID] = true
			threadIDs = append(threadIDs, msg.ThreadID)
		}
		if strings.EqualFold(msg.FromAddr, u.Email) {
			return
		}
		if err := w.store.UpsertEmail(ctx, StoredEmail{UserID: u.UserID, Message: msg}); err != nil {
			log.Printf("gmail-sync: user %d: store %s: %v", u.UserID, id, err)
			sawFailure = true
			return
		}
		if ts := msg.ReceivedAt.Unix(); ts > newest {
			newest = ts
		}
	}

	for _, id := range ids {
		fetch(id)
	}
	// Thread expansion: pull every sibling of a matched message's thread so a
	// reply with no ATS marker (a personal recruiter, a scheduling follow-up) is
	// ingested behind the anchor the search already found.
	for _, tid := range threadIDs {
		siblings, err := reader.ListThreadMessageIDs(ctx, tid)
		if err != nil {
			log.Printf("gmail-sync: user %d: thread %s: %v", u.UserID, tid, err)
			continue
		}
		for _, id := range siblings {
			fetch(id)
		}
	}

	if sawFailure {
		newest = u.Cursor
	}
	if err := w.store.SetSynced(ctx, u.UserID, newest); err != nil {
		log.Printf("gmail-sync: user %d: set synced: %v", u.UserID, err)
	}
}
