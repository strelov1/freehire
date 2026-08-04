package handler

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/inbox"
)

// harnessPageMax caps a listing that carries bodies. Bodies are the one listing
// payload heavy enough to matter, and an agent sweeping its backlog has no reason
// to pull an unbounded page.
//
// The assistant's own listing tool caps itself far lower (see
// assistantInboxBodyMax): a harness reads a page once, while a tool result is
// replayed into the model's context on every later turn of the session.
const harnessPageMax = 50

// emailLinking is the classification/link overlay carried by every inbox message
// shape: the classified status and, when resolved, the linked application (slug +
// company) or a pending suggestion the reading pane confirms inline.
type emailLinking struct {
	StatusSignal     string `json:"status_signal,omitempty"`
	LinkSource       string `json:"link_source,omitempty"`
	LinkedSlug       string `json:"linked_slug,omitempty"`
	LinkedCompany    string `json:"linked_company,omitempty"`
	SuggestedSlug    string `json:"suggested_slug,omitempty"`
	SuggestedCompany string `json:"suggested_company,omitempty"`
}

// linkingOf projects the service's message onto the wire overlay.
func linkingOf(m inbox.Message) emailLinking {
	return emailLinking{
		StatusSignal:     m.StatusSignal,
		LinkSource:       m.LinkSource,
		LinkedSlug:       m.LinkedSlug,
		LinkedCompany:    m.LinkedCompany,
		SuggestedSlug:    m.SuggestedSlug,
		SuggestedCompany: m.SuggestedCompany,
	}
}

// pgStr unwraps a nullable text column to a plain string ("" when NULL).
func pgStr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// inboxMessage is one row in the flat inbox listing. BodyText is empty unless the
// caller asked for bodies (?body=1) — the agent's read path, which lets a harness
// triage a whole page without a GetEmail per message (and so without marking any
// of them read).
type inboxMessage struct {
	ID         int64     `json:"id"`
	Source     string    `json:"source"`
	ExternalID string    `json:"external_id"`
	FromAddr   string    `json:"from_addr"`
	FromName   string    `json:"from_name"`
	Subject    string    `json:"subject"`
	Snippet    string    `json:"snippet"`
	BodyText   string    `json:"body_text,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
	Read       bool      `json:"read"`
	emailLinking
}

// emailBody is the single-message wire shape. s3_key (the internal raw-MIME
// pointer for hosted mail) is deliberately not exposed.
type emailBody struct {
	ID         int64     `json:"id"`
	Source     string    `json:"source"`
	ExternalID string    `json:"external_id"`
	FromAddr   string    `json:"from_addr"`
	FromName   string    `json:"from_name"`
	Subject    string    `json:"subject"`
	BodyText   string    `json:"body_text"`
	BodyHTML   string    `json:"body_html"`
	ReceivedAt time.Time `json:"received_at"`
	Read       bool      `json:"read"`
	emailLinking
}

// bodyOf projects the service's message onto the single-message wire shape.
func bodyOf(m inbox.Message) emailBody {
	return emailBody{
		ID: m.ID, Source: m.Source, ExternalID: m.ExternalID,
		FromAddr: m.FromAddr, FromName: m.FromName, Subject: m.Subject,
		BodyText: m.BodyText, BodyHTML: m.BodyHTML,
		ReceivedAt: m.ReceivedAt, Read: m.Read,
		emailLinking: linkingOf(m),
	}
}

// parseInboxQuery reads the listing filters off the query string:
// ?source=(gmail|hosted|external), ?unread=1, ?status=<signal>, ?q=<term>,
// ?unclassified=1, ?link=<state>, ?include_other=1. The vocabularies are checked by the
// service, so an unknown value is one 400 wherever it arrives from.
//
// include_other is opt-IN because the listing's default is the applications a person came
// to find, not everything a mailbox holds. What that default costs them is reported as a
// number rather than left to be discovered.
func parseInboxQuery(c *fiber.Ctx) inbox.Query {
	return inbox.Query{
		Source:       c.Query("source"),
		Status:       c.Query("status"),
		Link:         c.Query("link"),
		Q:            c.Query("q"),
		Unread:       c.QueryBool("unread"),
		Unclassified: c.QueryBool("unclassified"),
		IncludeOther: c.QueryBool("include_other"),
	}
}

// GetInbox returns the caller's mail as a flat list, newest first, excluding
// soft-deleted messages. Optional filters: ?source= (account switcher), ?unread=1
// (hide read), ?status= (one classified label), ?unclassified=1 (awaiting triage),
// ?link= (one link state — 'suggested' is the confirmation queue, 'unlinked' the
// mail with nowhere to go yet), ?q= (subject/sender/body search); standard
// limit/offset pagination.
//
// ?body=1 additionally returns each message's readable body. That is the agent's
// read path: it triages a page in one request, and — unlike GetEmail — marks
// nothing read, so a harness sweeping the backlog never zeroes its owner's unread
// count. Pages carrying bodies are capped at harnessPageMax.
func (h *inboxHandlers) GetInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	q := parseInboxQuery(c)
	q.WithBody = c.QueryBool("body")
	ceiling := maxLimit
	if q.WithBody {
		ceiling = harnessPageMax
	}
	limit, offset := pageParamsBounded(c, defaultLimit, ceiling)
	q.Limit, q.Offset = limit, offset

	page, err := h.inbox.Search(c.Context(), userID, q)
	if err != nil {
		return err
	}
	out := make([]inboxMessage, 0, len(page.Messages))
	for _, m := range page.Messages {
		out = append(out, inboxMessage{
			ID: m.ID, Source: m.Source, ExternalID: m.ExternalID,
			FromAddr: m.FromAddr, FromName: m.FromName, Subject: m.Subject,
			Snippet: m.Snippet, BodyText: m.BodyText,
			ReceivedAt: m.ReceivedAt, Read: m.Read,
			emailLinking: linkingOf(m),
		})
	}
	// The hidden count rides in the listing's meta rather than in a second request: a
	// number nobody fetches is a number nobody sees, and this one exists precisely so a
	// misclassification is findable.
	return listResponseWithHidden(c, out, page.Total, page.Hidden, limit, offset)
}

// GetEmail returns one message body, scoped to the caller (404 for another user's),
// and marks it read on open (best-effort — a failed mark never blocks reading).
//
// Marking read is why this path is not part of internal/inbox and is not offered to
// the assistant: read_at means "a human saw this", and a reader sweeping a backlog
// through here would zero its owner's unread count. Sweeping goes through GetInbox.
func (h *inboxHandlers) GetEmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	msg, err := h.inbox.Get(c.Context(), userID, int64(id))
	if err != nil {
		return err // pgx.ErrNoRows → 404 via the central error handler
	}
	if err := h.queries.MarkEmailRead(c.Context(), db.MarkEmailReadParams{ID: msg.ID, UserID: userID}); err != nil {
		log.Printf("inbox: mark read user=%d email=%d: %v", userID, msg.ID, err)
	}
	return c.JSON(fiber.Map{"data": bodyOf(msg)})
}

// MarkAllReadInbox marks every unread message matching the caller's active
// filters (source/status/search) as read and reports how many it marked. The
// unread filter is implicit — the query only ever touches unread rows.
func (h *inboxHandlers) MarkAllReadInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	q := parseInboxQuery(c)
	if err := q.Validate(); err != nil {
		return err
	}
	marked, err := h.queries.MarkAllEmailsRead(c.Context(), db.MarkAllEmailsReadParams{
		UserID: userID, Src: q.Source, Status: q.Status, Q: q.Q, Link: q.Link,
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"marked": marked}})
}

// DeleteEmail soft-deletes one message, scoped to the caller (404 if not theirs).
func (h *inboxHandlers) DeleteEmail(c *fiber.Ctx) error {
	return h.setEmailDeleted(c, true)
}

// RestoreEmail undoes a soft-delete, scoped to the caller (404 if not theirs).
func (h *inboxHandlers) RestoreEmail(c *fiber.Ctx) error {
	return h.setEmailDeleted(c, false)
}

// setEmailDeleted flips one message's soft-delete flag (delete or restore),
// scoped to the caller. A message that is not theirs matches no row → 404.
//
// It answers 204, not Fiber's SendStatus(200): that helper writes the status text
// "OK" as the body, which is neither of this API's response shapes, and a client
// that decodes a 2xx body fails on a call that actually succeeded.
func (h *inboxHandlers) setEmailDeleted(c *fiber.Ctx, deleted bool) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	var n int64
	if deleted {
		n, err = h.queries.SoftDeleteEmail(c.Context(), db.SoftDeleteEmailParams{ID: int64(id), UserID: userID})
	} else {
		n, err = h.queries.RestoreEmail(c.Context(), db.RestoreEmailParams{ID: int64(id), UserID: userID})
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
