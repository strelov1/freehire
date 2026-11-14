package gmailsync

import (
	"context"
	"time"
)

// Message is one fetched Gmail message in domain form (bodies already decoded).
type Message struct {
	ID         string
	ThreadID   string
	FromAddr   string
	FromName   string
	Subject    string
	BodyText   string
	BodyHTML   string
	ReceivedAt time.Time
	// CalendarUID identifies the meeting an invitation attaches, and is "" for the mail
	// that carries none. It is what later proves a calendar entry is this same meeting —
	// the only link calmatch may make without asking the candidate.
	CalendarUID string
}

// GmailReader reads one user's ATS mail via the Gmail API. Behind an interface so
// the worker is unit-tested with a fake and the live client is exercised only in
// the dry run.
type GmailReader interface {
	// ListATSMessageIDs returns the ids of hiring-shaped messages received after the Unix
	// watermark (0 = full backfill), excluding mail the connected address itself sent.
	//
	// The address is a parameter rather than reader state because the worker is the one
	// that knows it, and because a reader built for a search (see MailboxSearcher) has no
	// business carrying it.
	ListATSMessageIDs(ctx context.Context, selfAddr string, afterUnix int64) ([]string, error)
	// ListThreadMessageIDs returns the ids of every message in a thread, so replies
	// that carry no ATS marker (personal recruiters, scheduling) are ingested
	// alongside the matched message that anchors the thread.
	ListThreadMessageIDs(ctx context.Context, threadID string) ([]string, error)
	// GetMessage fetches one message in full.
	GetMessage(ctx context.Context, id string) (Message, error)
}

// MailboxSearcher runs an arbitrary search over one user's mailbox.
//
// Separate from GmailReader on purpose. That interface belongs to the sync worker and its
// query is the sync's own — widening it would hand every fake in this package a method it
// has no business implementing, and would blur which query each caller owns.
type MailboxSearcher interface {
	// Search returns whole messages matching the query, newest first, capped.
	Search(ctx context.Context, query string, limit int) ([]Message, error)
}

// MessageImporter stores one message the caller picked out of a search.
//
// It exists because a searched message is NOT in our store: the recall sweep shows what the
// mailbox holds and keeps nothing, so the moment a person links one is the moment it has to
// arrive. The write is the sync's own upsert, keyed on (source, external_id), so a message
// the worker had already fetched is updated rather than duplicated.
type MessageImporter interface {
	Import(ctx context.Context, userID int64, providerID string) error
}

// importer joins a reader to a store for the one message a caller confirmed.
type importer struct {
	reader GmailReader
	store  interface {
		UpsertEmail(ctx context.Context, e StoredEmail) error
	}
}

// NewImporter builds the import path for one user's confirmed message.
func NewImporter(reader GmailReader, store interface {
	UpsertEmail(ctx context.Context, e StoredEmail) error
}) MessageImporter {
	return &importer{reader: reader, store: store}
}

func (i *importer) Import(ctx context.Context, userID int64, providerID string) error {
	msg, err := i.reader.GetMessage(ctx, providerID)
	if err != nil {
		return err
	}
	return i.store.UpsertEmail(ctx, StoredEmail{UserID: userID, Message: msg})
}
