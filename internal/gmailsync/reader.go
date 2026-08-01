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
	// ListATSMessageIDs returns the ids of ATS messages received after the Unix
	// watermark (0 = full backfill).
	ListATSMessageIDs(ctx context.Context, afterUnix int64) ([]string, error)
	// ListThreadMessageIDs returns the ids of every message in a thread, so replies
	// that carry no ATS marker (personal recruiters, scheduling) are ingested
	// alongside the matched message that anchors the thread.
	ListThreadMessageIDs(ctx context.Context, threadID string) ([]string, error)
	// GetMessage fetches one message in full.
	GetMessage(ctx context.Context, id string) (Message, error)
}
