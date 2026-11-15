package gmailsync

import (
	"context"
	"time"
)

const (
	DirectionIncoming = "incoming"
	DirectionOutgoing = "outgoing"
)

// MessageRef identifies a message without reading its contents.
type MessageRef struct{ ID, ThreadID string }

// Message is one fetched Gmail message with decoded bodies and provider direction.
type Message struct {
	Draft              bool
	SystemNotification bool
	Direction          string
	ToAddrs            string
	CcAddrs            string
	ID                 string
	ThreadID           string
	FromAddr           string
	FromName           string
	Subject            string
	BodyText           string
	BodyHTML           string
	ReceivedAt         time.Time
	// CalendarUID identifies the meeting an invitation attaches ("" if none). It is the only
	// link calmatch may make between a calendar entry and this mail without asking the candidate.
	CalendarUID string
}

// GmailReader reads one user's ATS mail via the Gmail API.
type GmailReader interface {
	// ListATSMessageIDs returns the ids of hiring-shaped messages received after the Unix
	// watermark (0 = full backfill), excluding mail the connected address itself sent.
	//
	// The address is a parameter because a reader built for a search (see MailboxSearcher) has
	// no business carrying it.
	ListATSMessageIDs(ctx context.Context, selfAddr string, afterUnix int64) ([]string, error)
	// ListSentMessages returns metadata only; callers fetch bodies only for known hiring threads.
	ListSentMessages(ctx context.Context, afterUnix int64) ([]MessageRef, error)
	// ListThreadMessageIDs returns the ids of every message in a thread, so replies
	// that carry no ATS marker (personal recruiters, scheduling) are ingested
	// alongside the matched message that anchors the thread.
	ListThreadMessageIDs(ctx context.Context, threadID string) ([]string, error)
	// GetMessage fetches one message in full.
	GetMessage(ctx context.Context, id string) (Message, error)
}

// MailboxSearcher runs an arbitrary search over one user's mailbox.
//
// Separate from GmailReader: that interface and its query belong to the sync worker.
type MailboxSearcher interface {
	// Search returns whole messages matching the query, newest first, capped.
	Search(ctx context.Context, query string, limit int) ([]Message, error)
}

// MessageImporter stores one message the caller picked out of a search.
//
// A searched message is NOT in our store (the recall sweep keeps nothing), so it must be written
// when a person links it. The write is the sync's own upsert on (source, external_id), so an
// already-fetched message is updated, not duplicated.
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
