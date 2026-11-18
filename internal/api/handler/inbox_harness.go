// The bring-your-own-harness tier: a user's own mail client pushes what it fetched, and
// their own agent records its verdict. "Harness", not "agent", because the in-app
// assistant is a second agent on this store and reaches internal/application/inbox
// directly. Shared rules live in that package; this tier only brings its own transport
// and classifier.

package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/inbox"
)

// ingestMessage is one message a caller's mail client fetched — the wire shape. Its rules
// and the batch atomicity belong to internal/application/inbox.
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

// IngestEmails stores a batch of messages the caller's own harness fetched, under source
// 'external'. It only maps and renders; inbox.Service does the write.
//
// The body travels as raw JSON and is decoded here, as the payload is open-ended: a
// batch the service refuses is its own 400 with its own text.
func (h *inboxHandlers) IngestEmails(ctx context.Context, body jx.Raw) (*apigen.IngestEmailsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if h.inbox == nil {
		return nil, fiber.NewError(fiber.StatusNotImplemented, "mail ingest is not configured")
	}
	var req ingestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	msgs := make([]inbox.IncomingMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = m.incoming()
	}
	// A refused batch arrives as inbox.BatchError and renders as a 400 through classify.
	out, err := h.inbox.Ingest(ctx, userID, msgs)
	if err != nil {
		return nil, err
	}
	return &apigen.IngestEmailsOK{Data: apigen.InboxIngested{Inserted: int64(out.Inserted), Updated: int64(out.Updated)}}, nil
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

// TriageEmail records an agent-produced verdict for one message. The rules — the
// label vocabulary, the write that lands status, link and stamp together, and the
// monotonic-forward stage advance — live in inbox.Service.Triage, which the in-app
// assistant calls for the same operation.
func (h *inboxHandlers) TriageEmail(ctx context.Context, req *apigen.InboxTriage, params apigen.TriageEmailParams) (*apigen.TriageEmailOK, error) {
	verdict := inbox.Verdict{Confidence: ptrOf(req.Confidence.Get)}
	verdict.Signal, _ = req.Signal.Get()
	verdict.Slug, _ = req.Slug.Get()
	msg, err := h.renderMutation(ctx, params.ID, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.Triage(ctx, userID, id, verdict)
	})
	if err != nil {
		return nil, err
	}
	return &apigen.TriageEmailOK{Data: msg}, nil
}
