package handler

import (
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailclassify"
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

// TriageEmail records an agent-produced verdict for one message and advances the
// linked application's stage by the same rules the classification worker uses.
//
// It is SetEmailClassification's sibling, deliberately: status, link, provenance
// and the classified stamp are written together, so a message is never left
// classified-but-unstamped or linked-but-unclassified — states the worker never
// produces and no reader expects.
//
// Omitting a slug means "I am not deciding the link", not "clear it": clearing
// stays the explicit unlink action, so a classify-only pass cannot silently
// detach an application.
func (h *inboxHandlers) TriageEmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	var req triageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if !mailclassify.IsValidSignal(req.Signal) {
		return fiber.NewError(fiber.StatusBadRequest, "unknown signal")
	}

	// Resolve the slug before writing: an unknown one is a 404 that changes nothing.
	var jobID pgtype.Int8
	if req.Slug != "" {
		job, err := h.queries.GetJobBySlug(c.Context(), req.Slug)
		if err != nil {
			return err // ErrNoRows → 404
		}
		jobID = pgtype.Int8{Int64: job.ID, Valid: true}
	}

	var confidence pgtype.Float4
	if req.Confidence != nil {
		confidence = pgtype.Float4{Float32: *req.Confidence, Valid: true}
	}
	rows, err := h.queries.AgentTriageEmail(c.Context(), db.AgentTriageEmailParams{
		ID:           int64(id),
		UserID:       userID,
		StatusSignal: pgtype.Text{String: req.Signal, Valid: true},
		JobID:        jobID,
		Confidence:   confidence,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}

	if jobID.Valid {
		h.advanceStage(c, userID, jobID.Int64, mailclassify.StatusSignal(req.Signal))
	}
	return h.renderEmail(c, userID, int64(id))
}

// advanceStage moves a linked application forward when the verdict implies
// progress, by the same monotonic-forward rules the classification worker uses.
// It is best-effort: the verdict is already durable, and a failed advance must
// not fail the triage the agent successfully recorded.
func (h *inboxHandlers) advanceStage(c *fiber.Ctx, userID, jobID int64, sig mailclassify.StatusSignal) {
	current, err := h.queries.GetUserJobStage(c.Context(), db.GetUserJobStageParams{UserID: userID, JobID: jobID})
	if err != nil {
		// No application row means the caller linked mail to a job they do not
		// track; there is simply no stage to advance.
		return
	}
	next, ok := mailclassify.AdvanceStage(current, sig)
	if !ok {
		return
	}
	err = h.queries.AdvanceUserJobStage(c.Context(), db.AdvanceUserJobStageParams{
		UserID: userID, JobID: jobID, Stage: pgtype.Text{String: next, Valid: true},
	})
	if err != nil {
		log.Printf("inbox: advance stage user=%d job=%d: %v", userID, jobID, err)
	}
}
