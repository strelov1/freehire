package gmailsync

import (
	"context"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/platform/tokencrypt"
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

// Stats reports what a run did, so the command can say it and exit accordingly.
//
// Without this a run that reached nobody was byte-identical to a clean one: RunOnce
// returned nil either way, cmd/gmail-sync exited 0, and freehire_worker_last_run_success
// read 1 straight through a total failure. Nothing else measures this sync.
type Stats struct {
	Connections int
	Synced      int
	// Failed counts connections whose sync did not complete — a token that would not
	// decrypt, a provider having a bad day, a message that would not store. The next run
	// retries them.
	Failed int
	// Reconsent counts connections whose grant was refused and flagged. No later run
	// retries those: only a browser consent clears the flag.
	Reconsent int
}

// outcome is what one connection's pass amounted to.
type outcome int

const (
	outcomeSynced outcome = iota
	outcomeFailed
	outcomeReconsent
)

// RunOnce syncs all connected users once.
func (w *Worker) RunOnce(ctx context.Context) (Stats, error) {
	users, err := w.store.ListConnected(ctx)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Connections: len(users)}
	for _, u := range users {
		switch w.syncUser(ctx, u) {
		case outcomeSynced:
			stats.Synced++
		case outcomeReconsent:
			stats.Reconsent++
		default:
			stats.Failed++
		}
	}
	return stats, nil
}

// markRevoked flags the connection for re-consent, once a call has already logged which
// one revealed the revoked grant. Shared by every call site that can reveal one (the
// list call, a message fetch, a thread-sibling listing) so the store write and the
// outcome it reports cannot drift between them the way ListThreadMessageIDs's own check
// once did.
func (w *Worker) markRevoked(ctx context.Context, userID int64) outcome {
	if err := w.store.SetNeedsReconsent(ctx, userID); err != nil {
		// The status did not move, so the mailbox is still `connected` and the next
		// run will meet the same revoked grant and try again. Counting this as a
		// re-consent would report a transition that did not happen — and this is the
		// one outcome the run must not call clean, because nothing else will notice:
		// the user is told to reconnect by a status that was never written.
		log.Printf("gmail-sync: user %d: set status: %v", userID, err)
		return outcomeFailed
	}
	return outcomeReconsent
}

// SyncUser syncs one connected user's mail on demand (a manual refresh from the
// inbox), reusing the same best-effort per-user path as the cron worker. The outcome is
// dropped: the caller is a background goroutine behind a button, and the user reads the
// result by watching their inbox fill.
func (w *Worker) SyncUser(ctx context.Context, u Connection) { _ = w.syncUser(ctx, u) }

func (w *Worker) syncUser(ctx context.Context, u Connection) outcome {
	encToken, err := w.store.RefreshToken(ctx, u.UserID)
	if err != nil {
		log.Printf("gmail-sync: user %d: read token: %v", u.UserID, err)
		return outcomeFailed
	}
	refresh, err := w.cipher.Decrypt(encToken)
	if err != nil {
		log.Printf("gmail-sync: user %d: decrypt token: %v", u.UserID, err)
		return outcomeFailed
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
		if !RevokedGrant(err) {
			// A rate limit, a 500, a reset connection, or this process being stopped
			// mid-run. None of them says anything about the grant, and the flag they
			// used to set is shared with the calendar and cleared only by a browser
			// consent — so one Google incident, or one redeploy, disconnected every
			// mailbox we hold. It is reachable from a button, too: the on-demand sync
			// runs this same path.
			log.Printf("gmail-sync: user %d: list: %v", u.UserID, err)
			return outcomeFailed
		}
		log.Printf("gmail-sync: user %d: list: %v — marking needs_reconsent", u.UserID, err)
		return w.markRevoked(ctx, u.UserID)
	}

	newest := u.Cursor
	sawFailure := false
	revoked := false
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
	//
	// A message fetch can reveal a revoked grant exactly as the list call can —
	// RevokedGrant reads the status code, not which endpoint sent it — and must be
	// asked here too: otherwise a grant that lost message-read access but still
	// lists fine re-fetches, and re-403s, the same unadvancing watermark's worth of
	// messages every run, forever, instead of being flagged for re-consent once.
	fetch := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		msg, err := reader.GetMessage(ctx, id)
		if err != nil {
			if RevokedGrant(err) {
				log.Printf("gmail-sync: user %d: get %s: %v — marking needs_reconsent", u.UserID, id, err)
				revoked = true
				return
			}
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
		if revoked {
			break
		}
	}
	// Thread expansion: pull every sibling of a matched message's thread so a
	// reply with no ATS marker (a personal recruiter, a scheduling follow-up) is
	// ingested behind the anchor the search already found. Skipped once the grant
	// is known revoked — every further call would just 403 too.
	for _, tid := range threadIDs {
		if revoked {
			break
		}
		siblings, err := reader.ListThreadMessageIDs(ctx, tid)
		if err != nil {
			if RevokedGrant(err) {
				log.Printf("gmail-sync: user %d: thread %s: %v — marking needs_reconsent", u.UserID, tid, err)
				revoked = true
				break
			}
			log.Printf("gmail-sync: user %d: thread %s: %v", u.UserID, tid, err)
			continue
		}
		for _, id := range siblings {
			fetch(id)
			if revoked {
				break
			}
		}
	}

	if revoked {
		return w.markRevoked(ctx, u.UserID)
	}

	if sawFailure {
		newest = u.Cursor
	}
	if err := w.store.SetSynced(ctx, u.UserID, newest); err != nil {
		log.Printf("gmail-sync: user %d: set synced: %v", u.UserID, err)
		return outcomeFailed
	}
	if sawFailure {
		return outcomeFailed
	}
	return outcomeSynced
}
