// The bring-your-own-harness tier: a user's own mail client pushes what it
// fetched, and their own agent records its verdict. It is called the HARNESS
// surface rather than the "agent" surface because there are now two agents on this
// store — this one, an external process holding an API key, and the in-app
// assistant, which issues no HTTP request at all and reaches internal/inbox
// directly. The rules they share live in that package; what lives here is only
// what this tier does differently: it brings its own transport and its own
// classifier, and it is therefore the tier that costs us nothing.

package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/inbox"
)

// maxIngestBatch bounds one push. A harness syncing a mailbox pages through it;
// an unbounded batch would be one transaction holding an arbitrary number of rows.
// An oversized batch is refused rather than truncated, so "inserted" is never a
// partial truth the caller has to guess at.
const maxIngestBatch = 100

// ingestMessage is one message a caller's own mail client fetched and handed over.
// ExternalID is the deduplication key — a Message-ID in practice — and is the only
// required field beyond a timestamp: everything else may legitimately be empty in
// real ATS mail.
type ingestMessage struct {
	ExternalID string    `json:"external_id"`
	ThreadID   string    `json:"thread_id"`
	FromAddr   string    `json:"from_addr"`
	FromName   string    `json:"from_name"`
	Subject    string    `json:"subject"`
	BodyText   string    `json:"body_text"`
	BodyHTML   string    `json:"body_html"`
	ReceivedAt time.Time `json:"received_at"`
}

// ingestRequest is the push payload: a bounded batch of messages.
type ingestRequest struct {
	Messages []ingestMessage `json:"messages"`
}

// ingestResult reports how the batch landed, so a syncing agent can tell new mail
// from a re-run of the same window without diffing anything itself.
type ingestResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
}

// IngestEmails stores a batch of messages the caller's own harness fetched, under
// source 'external'. freehire provides no transport here: the caller owns the mail
// client, and this endpoint is where its output enters the ordinary inbox.
//
// The whole batch is one transaction. A partially-applied batch would leave the
// caller unable to tell what to retry, and the reported counts would be a lie.
func (h *inboxHandlers) IngestEmails(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req ingestRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if err := validateIngestBatch(req.Messages); err != nil {
		return err
	}

	tx, err := h.pool.Begin(c.Context())
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(c.Context()) }()

	qtx := h.queries.WithTx(tx)
	var out ingestResult
	for _, m := range req.Messages {
		row, err := qtx.UpsertExternalEmail(c.Context(), m.upsertParams(userID))
		if err != nil {
			return err
		}
		if row.Inserted {
			out.Inserted++
		} else {
			out.Updated++
		}
	}
	if err := tx.Commit(c.Context()); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": out})
}

// validateIngestBatch rejects a batch before any of it is written, so a bad
// message at the end cannot leave the earlier ones stored under a 400.
func validateIngestBatch(msgs []ingestMessage) error {
	if len(msgs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "messages required")
	}
	if len(msgs) > maxIngestBatch {
		return fiber.NewError(fiber.StatusBadRequest,
			"batch too large: at most "+strconv.Itoa(maxIngestBatch)+" messages per request")
	}
	for i, m := range msgs {
		if m.ExternalID == "" {
			return fiber.NewError(fiber.StatusBadRequest,
				"messages["+strconv.Itoa(i)+"]: external_id is required — it is the deduplication key")
		}
	}
	return nil
}

// upsertParams projects a pushed message onto the store's parameters, defaulting
// a missing timestamp to now so a client that omits one still orders sensibly.
func (m ingestMessage) upsertParams(userID int64) db.UpsertExternalEmailParams {
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
		ReceivedAt: pgtype.Timestamptz{Time: receivedAt, Valid: true},
		// No meeting identifier: this tier receives a JSON projection, not MIME, so
		// there is no text/calendar part to read one out of. Pushed mail therefore
		// yields no automatic calendar link, which is the same shape as the tier's
		// existing bargain — it is never classified server-side either. Accepting a
		// caller-supplied ical_uid would change the ingest contract and can wait for
		// a harness that wants it.
	}
}

// triageRequest is an agent's verdict for one message: what the message is, and
// optionally which of the caller's applications it belongs to.
type triageRequest struct {
	Signal string `json:"signal"`
	Slug   string `json:"slug"`
	// Confidence is the agent's own confidence in the verdict, stored for display
	// and debugging. It gates nothing — the caller asked for this classification.
	Confidence *float32 `json:"confidence"`
}

// TriageEmail records an agent-produced verdict for one message. The rules — the
// label vocabulary, the write that lands status, link and stamp together, and the
// monotonic-forward stage advance — live in inbox.Service.Triage, which the in-app
// assistant calls for the same operation.
func (h *inboxHandlers) TriageEmail(c *fiber.Ctx) error {
	var req triageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return h.renderMutation(c, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.Triage(c.Context(), userID, id, inbox.Verdict{
			Signal: req.Signal, Slug: req.Slug, Confidence: req.Confidence,
		})
	})
}
