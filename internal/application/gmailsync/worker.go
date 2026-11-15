package gmailsync

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// Connection is a connected user the sync worker processes.
type Connection struct {
	UserID     int64
	Email      string
	Cursor     int64 // Unix watermark of the newest synced incoming message
	SentCursor int64 // Run-start watermark of the last complete sent scan
}

// StoredRef is what the store already holds of a message the sync would otherwise fetch again.
type StoredRef struct {
	ThreadID   string
	ReceivedAt time.Time
	Incoming   bool
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
	SetSynced(ctx context.Context, userID, cursorUnix, sentCursor int64) error
	ListThreadIDs(ctx context.Context, userID int64) ([]string, error)
	// StoredMessages returns which of ids are already stored for the user, keyed by id.
	StoredMessages(ctx context.Context, userID int64, ids []string) (map[string]StoredRef, error)
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
// Without it a run that reached nobody would look clean: RunOnce returns nil either way.
type Stats struct {
	Connections int
	Synced      int
	// Failed counts connections whose sync did not complete (token, provider or store error).
	// The next run retries them.
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
// one revealed the revoked grant. Shared by every call site that can reveal one (list, message
// fetch, thread-sibling listing) so the store write and the reported outcome cannot drift.
func (w *Worker) markRevoked(ctx context.Context, userID int64) outcome {
	if err := w.store.SetNeedsReconsent(ctx, userID); err != nil {
		// The status did not move, so the mailbox is still `connected` and the next run retries.
		// Counting a re-consent would report a transition that did not happen, and the run must
		// not look clean: the user is told to reconnect by a status that was never written.
		log.Printf("gmail-sync: user %d: set status: %v", userID, err)
		return outcomeFailed
	}
	return outcomeReconsent
}

// SyncUser syncs one connected user's mail on demand (a manual refresh from the
// inbox), reusing the cron worker's best-effort path. The outcome is dropped: the caller is a
// background goroutine behind a button.
func (w *Worker) SyncUser(ctx context.Context, u Connection) { _ = w.syncUser(ctx, u) }

// firstSentScanThreads caps how many stored conversations, newest first, the first
// sent-mail scan of a mailbox expands.
const firstSentScanThreads = 50

func (w *Worker) syncUser(ctx context.Context, u Connection) outcome {
	started := time.Now().Unix()
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
			// A rate limit, a 500, a reset connection, or this process being stopped says nothing
			// about the grant, and the flag is shared with the calendar and cleared only by a
			// browser consent. The on-demand sync runs this same path.
			log.Printf("gmail-sync: user %d: list: %v", u.UserID, err)
			return outcomeFailed
		}
		log.Printf("gmail-sync: user %d: list: %v — marking needs_reconsent", u.UserID, err)
		return w.markRevoked(ctx, u.UserID)
	}

	newest := u.Cursor
	sawFailure := false
	revoked := false
	// throttled stops the wave on a rate-limit answer: further calls would be refused too. The
	// cursors stay put, so the next run retries.
	throttled := false
	seen := make(map[string]bool)
	seenThread := make(map[string]bool)
	var threadIDs []string
	// stored holds the listed and sibling messages an earlier run already persisted. They are
	// not fetched again: a run stopped by the rate limit leaves its cursor where it was, so
	// without this every later run re-fetches the same head of the mailbox and stops at the
	// same limit, never getting through a large first sync.
	stored := make(map[string]StoredRef)
	loadStored := func(ids []string) {
		refs, err := w.store.StoredMessages(ctx, u.UserID, ids)
		if err != nil {
			// Only a saving, not a gate: fetching them again is what the sync did before.
			log.Printf("gmail-sync: user %d: stored messages: %v", u.UserID, err)
			return
		}
		for id, ref := range refs {
			stored[id] = ref
		}
	}

	// fetch persists one message, deduping by id, tracking the watermark, and
	// recording its thread for expansion. Both directions are retained; newest tracks incoming
	// ReceivedAt seen regardless of order; once any message in the wave fails, it is reset to
	// u.Cursor before SetSynced rather than frozen where it was: a newer message can succeed
	// before an older sibling fails, and the watermark must not jump past the failed one.
	//
	// A message fetch can reveal a revoked grant like the list call can, so RevokedGrant is
	// asked here too; otherwise a grant that lost message-read access but still lists would
	// re-403 the same messages every run instead of being flagged once.
	fetch := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		if ref, ok := stored[id]; ok {
			if ref.ThreadID != "" && !seenThread[ref.ThreadID] {
				seenThread[ref.ThreadID] = true
				threadIDs = append(threadIDs, ref.ThreadID)
			}
			if ts := ref.ReceivedAt.Unix(); ref.Incoming && ts > newest {
				newest = ts
			}
			return
		}
		msg, err := reader.GetMessage(ctx, id)
		if err != nil {
			if missingMessage(err) {
				return
			}
			if RevokedGrant(err) {
				log.Printf("gmail-sync: user %d: get %s: %v — marking needs_reconsent", u.UserID, id, err)
				revoked = true
				return
			}
			log.Printf("gmail-sync: user %d: get %s: %v", u.UserID, id, err)
			throttled = throttled || LimitHit(err)
			sawFailure = true
			return
		}
		if msg.SystemNotification {
			if ts := msg.ReceivedAt.Unix(); ts > newest {
				newest = ts
			}
			return
		}
		if msg.Draft {
			return
		}
		if msg.ThreadID != "" && !seenThread[msg.ThreadID] {
			seenThread[msg.ThreadID] = true
			threadIDs = append(threadIDs, msg.ThreadID)
		}
		if msg.Direction == "" {
			msg.Direction = DirectionIncoming
		}
		if u.Email != "" && strings.EqualFold(msg.FromAddr, u.Email) {
			msg.Direction = DirectionOutgoing
		}
		if err := w.store.UpsertEmail(ctx, StoredEmail{UserID: u.UserID, Message: msg}); err != nil {
			log.Printf("gmail-sync: user %d: store %s: %v", u.UserID, id, err)
			sawFailure = true
			return
		}
		if ts := msg.ReceivedAt.Unix(); msg.Direction == DirectionIncoming && ts > newest {
			newest = ts
		}
	}

	loadStored(ids)
	for _, id := range ids {
		fetch(id)
		if revoked || throttled {
			break
		}
	}
	// Existing stored threads are the privacy boundary for outgoing discovery.
	knownThreads, err := w.store.ListThreadIDs(ctx, u.UserID)
	if err != nil {
		log.Printf("gmail-sync: user %d: known threads: %v", u.UserID, err)
		return outcomeFailed
	}
	known := make(map[string]bool, len(knownThreads))
	for i, tid := range knownThreads {
		known[tid] = true
		// The first scan (no sent cursor yet) re-reads stored conversations to pick up earlier
		// replies; it is capped because an unbounded burst gets rate-limited by Google.
		// shortcut: older conversations never get their past replies, raise the cap or
		// make the scan resumable if users ask for them.
		if u.SentCursor == 0 && i < firstSentScanThreads && !seenThread[tid] {
			seenThread[tid] = true
			threadIDs = append(threadIDs, tid)
		}
	}
	if u.SentCursor != 0 && !revoked && !throttled {
		refs, err := reader.ListSentMessages(ctx, u.SentCursor)
		if err != nil {
			if RevokedGrant(err) {
				return w.markRevoked(ctx, u.UserID)
			}
			log.Printf("gmail-sync: user %d: sent messages: %v", u.UserID, err)
			throttled = throttled || LimitHit(err)
			sawFailure = true
		} else {
			for _, ref := range refs {
				if known[ref.ThreadID] || seenThread[ref.ThreadID] {
					fetch(ref.ID)
					if revoked || throttled {
						break
					}
				}
			}
		}
	}

	// Thread expansion: pull every sibling of a matched message's thread so a
	// reply with no ATS marker (a personal recruiter, a scheduling follow-up) is
	// ingested behind the anchor the search already found. Skipped once the grant
	// is known revoked — every further call would just 403 too.
	for _, tid := range threadIDs {
		if revoked || throttled {
			break
		}
		siblings, err := reader.ListThreadMessageIDs(ctx, tid)
		if err != nil {
			if missingMessage(err) {
				continue
			}
			if RevokedGrant(err) {
				log.Printf("gmail-sync: user %d: thread %s: %v — marking needs_reconsent", u.UserID, tid, err)
				revoked = true
				break
			}
			log.Printf("gmail-sync: user %d: thread %s: %v", u.UserID, tid, err)
			throttled = throttled || LimitHit(err)
			sawFailure = true
			continue
		}
		loadStored(siblings)
		for _, id := range siblings {
			fetch(id)
			if revoked || throttled {
				break
			}
		}
	}

	if revoked {
		return w.markRevoked(ctx, u.UserID)
	}

	if sawFailure {
		newest = u.Cursor
		started = u.SentCursor
	}
	if err := w.store.SetSynced(ctx, u.UserID, newest, started); err != nil {
		log.Printf("gmail-sync: user %d: set synced: %v", u.UserID, err)
		return outcomeFailed
	}
	if sawFailure {
		return outcomeFailed
	}
	return outcomeSynced
}

// A permanently removed provider resource has no content left to synchronize.
func missingMessage(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
