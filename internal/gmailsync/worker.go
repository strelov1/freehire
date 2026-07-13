package gmailsync

import (
	"context"
	"log"

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
	UserID      int64
	Message     Message
	SubjectNorm string
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
// token — the real factory uses the Connector + Gmail API; tests inject a fake.
type ReaderFactory func(ctx context.Context, refreshToken string) GmailReader

// Worker syncs every connected user's ATS mail. Best-effort per user: a token or
// API failure marks that user and continues.
type Worker struct {
	store     Store
	cipher    *tokencrypt.Cipher
	newReader ReaderFactory
}

// NewWorker builds the sync worker.
func NewWorker(store Store, cipher *tokencrypt.Cipher, newReader ReaderFactory) *Worker {
	return &Worker{store: store, cipher: cipher, newReader: newReader}
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
	reader := w.newReader(ctx, refresh)

	ids, err := reader.ListATSMessageIDs(ctx, u.Cursor)
	if err != nil {
		// A revoked/expired grant surfaces here; flag it for re-consent and move on.
		log.Printf("gmail-sync: user %d: list: %v — marking needs_reconsent", u.UserID, err)
		if err := w.store.SetNeedsReconsent(ctx, u.UserID); err != nil {
			log.Printf("gmail-sync: user %d: set status: %v", u.UserID, err)
		}
		return
	}

	newest := u.Cursor
	for _, id := range ids {
		msg, err := reader.GetMessage(ctx, id)
		if err != nil {
			log.Printf("gmail-sync: user %d: get %s: %v", u.UserID, id, err)
			continue
		}
		if err := w.store.UpsertEmail(ctx, StoredEmail{
			UserID:      u.UserID,
			Message:     msg,
			SubjectNorm: NormalizeSubject(msg.Subject),
		}); err != nil {
			log.Printf("gmail-sync: user %d: store %s: %v", u.UserID, id, err)
			continue
		}
		if ts := msg.ReceivedAt.Unix(); ts > newest {
			newest = ts
		}
	}

	if err := w.store.SetSynced(ctx, u.UserID, newest); err != nil {
		log.Printf("gmail-sync: user %d: set synced: %v", u.UserID, err)
	}
}
