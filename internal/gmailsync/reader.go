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
}

// GmailReader reads one user's ATS mail via the Gmail API. Behind an interface so
// the worker is unit-tested with a fake and the live client is exercised only in
// the dry run.
type GmailReader interface {
	// ListATSMessageIDs returns the ids of ATS messages received after the Unix
	// watermark (0 = full backfill).
	ListATSMessageIDs(ctx context.Context, afterUnix int64) ([]string, error)
	// GetMessage fetches one message in full.
	GetMessage(ctx context.Context, id string) (Message, error)
}
