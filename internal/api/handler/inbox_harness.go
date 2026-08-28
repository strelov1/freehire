// The bring-your-own-harness tier: a user's own mail client pushes what it
// fetched, and their own agent records its verdict. It is called the HARNESS
// surface rather than the "agent" surface because there are now two agents on this
// store — this one, an external process holding an API key, and the in-app
// assistant, which issues no HTTP request at all and reaches internal/application/inbox
// directly. The rules they share live in that package; what lives here is only
// what this tier does differently: it brings its own transport and its own
// classifier, and it is therefore the tier that costs us nothing.

package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/application/inbox"
)

// ingestMessage is one message a caller's own mail client fetched and handed over — the WIRE
// shape. The rules it must satisfy, and the atomicity of the batch, belong to
// internal/application/inbox: this tier is not the only reader of that store.
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

// ingestRequest is the push payload: a batch of messages.
type ingestRequest struct {
	Messages []ingestMessage `json:"messages"`
}

// ingestResult reports how the batch landed, so a syncing agent can tell new mail from a
// re-run of the same window without diffing anything itself.
type ingestResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
}

// IngestEmails stores a batch of messages the caller's own harness fetched, under source
// 'external'. freehire provides no transport here: the caller owns the mail client, and this
// endpoint is where its output enters the ordinary inbox.
//
// It maps and renders. It used to open its own pgx transaction and write through sqlc — the
// only handler in this package that did — which is why the in-app assistant, which reaches
// inbox.Service directly and issues no HTTP request, could not ingest mail at all.
func (h *inboxHandlers) IngestEmails(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if h.inbox == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "mail ingest is not configured")
	}
	var req ingestRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	msgs := make([]inbox.IncomingMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = m.incoming()
	}
	// A refused batch arrives as inbox.BatchError and renders as a 400 through classify —
	// the same rule the assistant's tools will report to the model.
	out, err := h.inbox.Ingest(c.Context(), userID, msgs)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": ingestResult{Inserted: out.Inserted, Updated: out.Updated}})
}

// incoming maps the wire shape onto the service's own.
func (m ingestMessage) incoming() inbox.IncomingMessage {
	return inbox.IncomingMessage{
		ExternalID: m.ExternalID,
		ThreadID:   m.ThreadID,
		FromAddr:   m.FromAddr,
		FromName:   m.FromName,
		Subject:    m.Subject,
		BodyText:   m.BodyText,
		BodyHTML:   m.BodyHTML,
		ReceivedAt: m.ReceivedAt,
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
