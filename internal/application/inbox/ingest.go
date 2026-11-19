package inbox

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// MaxIngestBatch bounds one harness push (one transaction). An oversized batch is REFUSED rather
// than truncated, so the reported counts are never a partial truth.
const MaxIngestBatch = 100

// IncomingMessage is one message a caller's own mail client fetched and handed over.
// ExternalID is the deduplication key — a Message-ID in practice — and is the only required
// field beyond a timestamp: everything else may legitimately be empty in real ATS mail.
type IncomingMessage struct {
	ExternalID string
	ThreadID   string
	FromAddr   string
	FromName   string
	Subject    string
	BodyText   string
	BodyHTML   string
	ReceivedAt time.Time
}

// IngestResult reports how a batch landed, so a syncing caller can tell new mail from a re-run
// of the same window without diffing anything itself.
type IngestResult struct {
	Inserted int
	Updated  int
}

// BatchError refuses a push before any of it is written, naming what is wrong. It is a
// caller mistake, not a fault: the port renders it as a 400.
type BatchError struct{ Reason string }

func (e *BatchError) Error() string { return e.Reason }

// Ingester writes a batch under source 'external'. The WHOLE batch is one transaction, so the
// caller never has to tell what to retry; the implementation owns the pool and that obligation.
type Ingester interface {
	IngestBatch(ctx context.Context, userID int64, msgs []IncomingMessage) (IngestResult, error)
}

// Ingest stores a batch of messages a caller's own harness fetched. freehire provides no
// transport here: the caller owns the mail client, and this is where its output enters the
// ordinary inbox.
//
// The rules (batch ceiling, required deduplication key, refusing the whole push rather than
// storing a prefix) live here, not at the HTTP door, because the in-app assistant also ingests
// through this (see the package comment).
func (s *Service) Ingest(ctx context.Context, userID int64, msgs []IncomingMessage) (IngestResult, error) {
	if s.ingest == nil {
		return IngestResult{}, ErrUnavailable
	}
	if err := validateBatch(msgs); err != nil {
		return IngestResult{}, err
	}
	return s.ingest.IngestBatch(ctx, userID, msgs)
}

// validateBatch rejects a batch before any of it is written, so a bad message at the end
// cannot leave the earlier ones stored under a refusal.
func validateBatch(msgs []IncomingMessage) error {
	if len(msgs) == 0 {
		return &BatchError{Reason: "messages required"}
	}
	if len(msgs) > MaxIngestBatch {
		return &BatchError{Reason: "batch too large: at most " + strconv.Itoa(MaxIngestBatch) + " messages per request"}
	}
	for i, m := range msgs {
		if m.ExternalID == "" {
			return &BatchError{Reason: "messages[" + strconv.Itoa(i) +
				"]: external_id is required — it is the deduplication key"}
		}
	}
	return nil
}

// QueriesIngester is the pool-backed Ingester: one transaction per batch.
type QueriesIngester struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewQueriesIngester builds the Ingester over the pool that owns the transaction.
func NewQueriesIngester(pool *pgxpool.Pool, q *db.Queries) *QueriesIngester {
	return &QueriesIngester{pool: pool, q: q}
}

// IngestBatch upserts every message in ONE transaction, so a caller that retries a window
// never has to reason about a half-stored batch.
func (i *QueriesIngester) IngestBatch(ctx context.Context, userID int64, msgs []IncomingMessage) (IngestResult, error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return IngestResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := i.q.WithTx(tx)
	var out IngestResult
	for _, m := range msgs {
		row, err := qtx.UpsertExternalEmail(ctx, m.upsertParams(userID))
		if err != nil {
			return IngestResult{}, err
		}
		if row.Inserted {
			out.Inserted++
		} else {
			out.Updated++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return IngestResult{}, err
	}
	return out, nil
}

// upsertParams projects a pushed message onto the store's parameters, defaulting a missing
// timestamp to now so a client that omits one still orders sensibly.
func (m IncomingMessage) upsertParams(userID int64) db.UpsertExternalEmailParams {
	receivedAt := m.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	return db.UpsertExternalEmailParams{
		UserID:     userID,
		ExternalID: m.ExternalID,
		ThreadID:   m.ThreadID,
		FromAddr:   m.FromAddr,
		FromName:   m.FromName,
		Subject:    m.Subject,
		BodyText:   m.BodyText,
		BodyHtml:   m.BodyHTML,
		ReceivedAt: pgconv.Instant(receivedAt),
		// No meeting identifier: this tier receives a JSON projection, not MIME, so there is no
		// text/calendar part and pushed mail yields no automatic calendar link.
	}
}
