package handler

import (
	"context"
	"log"
	"net/url"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/platform/db"
)

// harnessPageMax caps a listing that carries bodies. Bodies are the one listing
// payload heavy enough to matter, and an agent sweeping its backlog has no reason
// to pull an unbounded page.
//
// The assistant's own listing tool caps itself far lower (see
// assistantInboxBodyMax): a harness reads a page once, while a tool result is
// replayed into the model's context on every later turn of the session.
const harnessPageMax = 50

// pgStr unwraps a nullable text column to a plain string ("" when NULL).
func pgStr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// emailPathID reads the ":id" of an email route. One that is not a number names no email, so
// it is the same 404 as an id that is not the caller's, not the 400 of the other id routes.
func emailPathID(raw string) (int64, error) {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return int64(id), nil
}

// bodyOf projects the service's message onto the single-message wire shape. s3_key (the
// internal raw-MIME pointer for hosted mail) is deliberately not exposed.
func bodyOf(m inbox.Message) apigen.InboxEmail {
	return apigen.InboxEmail{
		ID: m.ID, Source: m.Source, ExternalID: m.ExternalID,
		FromAddr: m.FromAddr, FromName: m.FromName, Subject: m.Subject,
		BodyText: m.BodyText, BodyHTML: m.BodyHTML,
		ReceivedAt: m.ReceivedAt, Read: m.Read,
		StatusSignal: optString(m.StatusSignal), LinkSource: optString(m.LinkSource),
		LinkedSlug: optString(m.LinkedSlug), LinkedCompany: optString(m.LinkedCompany),
		SuggestedSlug: optString(m.SuggestedSlug), SuggestedCompany: optString(m.SuggestedCompany),
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
func parseInboxQuery(query url.Values) inbox.Query {
	return inbox.Query{
		Source:       query.Get("source"),
		Status:       query.Get("status"),
		Link:         query.Get("link"),
		Q:            query.Get("q"),
		Unread:       queryBool(query, "unread"),
		Unclassified: queryBool(query, "unclassified"),
		IncludeOther: queryBool(query, "include_other"),
	}
}

// queryBool is Fiber's QueryBool over a parsed query: false when absent or unreadable.
func queryBool(query url.Values, key string) bool {
	b, _ := strconv.ParseBool(query.Get(key))
	return b
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
func (h *inboxHandlers) GetInbox(ctx context.Context, _ apigen.GetInboxParams) (*apigen.InboxPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	query := contractQuery(ctx)
	q := parseInboxQuery(query)
	q.WithBody = queryBool(query, "body")
	ceiling := maxLimit
	if q.WithBody {
		ceiling = harnessPageMax
	}
	limit, offset := contractPageBounded(ctx, defaultLimit, ceiling)
	q.Limit, q.Offset = limit, offset

	page, err := h.inbox.Search(ctx, userID, q)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.InboxMessage, 0, len(page.Messages))
	for _, m := range page.Messages {
		out = append(out, apigen.InboxMessage{
			ID: m.ID, Source: m.Source, ExternalID: m.ExternalID,
			FromAddr: m.FromAddr, FromName: m.FromName, Subject: m.Subject,
			Snippet: m.Snippet, BodyText: optString(m.BodyText),
			ReceivedAt: m.ReceivedAt, Read: m.Read,
			StatusSignal: optString(m.StatusSignal), LinkSource: optString(m.LinkSource),
			LinkedSlug: optString(m.LinkedSlug), LinkedCompany: optString(m.LinkedCompany),
			SuggestedSlug: optString(m.SuggestedSlug), SuggestedCompany: optString(m.SuggestedCompany),
		})
	}
	// The hidden count rides in the listing's meta rather than in a second request: a
	// number nobody fetches is a number nobody sees, and this one exists precisely so a
	// misclassification is findable.
	return &apigen.InboxPage{
		Data: out,
		Meta: apigen.InboxMeta{Total: page.Total, Limit: int64(limit), Offset: int64(offset), Hidden: page.Hidden},
	}, nil
}

// GetEmail returns one message body, scoped to the caller (404 for another user's),
// and marks it read on open (best-effort — a failed mark never blocks reading).
//
// Marking read is why this path is not part of internal/application/inbox and is not offered to
// the assistant: read_at means "a human saw this", and a reader sweeping a backlog
// through here would zero its owner's unread count. Sweeping goes through GetInbox.
func (h *inboxHandlers) GetEmail(ctx context.Context, params apigen.GetEmailParams) (*apigen.GetEmailOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := emailPathID(params.ID)
	if err != nil {
		return nil, err
	}
	msg, err := h.inbox.Get(ctx, userID, id)
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404 via the central error handler
	}
	if err := h.queries.MarkEmailRead(ctx, db.MarkEmailReadParams{ID: msg.ID, UserID: userID}); err != nil {
		log.Printf("inbox: mark read user=%d email=%d: %v", userID, msg.ID, err)
	}
	return &apigen.GetEmailOK{Data: bodyOf(msg)}, nil
}

// MarkAllReadInbox marks every unread message matching the caller's active
// filters as read and reports how many it marked. The unread filter is implicit —
// the query only ever touches unread rows.
//
// Every filter parseInboxQuery reads is passed on, because "mark all read" means the page
// in front of the caller and there is no undo. `include_other` goes through ShowsOther for
// the same reason the listing does: `?status=other` is a request FOR that label, and
// passing IncludeOther raw would mark nothing.
func (h *inboxHandlers) MarkAllReadInbox(ctx context.Context, _ apigen.MarkAllReadInboxParams) (*apigen.MarkAllReadInboxOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	q := parseInboxQuery(contractQuery(ctx))
	if err := q.Validate(); err != nil {
		return nil, err
	}
	marked, err := h.queries.MarkAllEmailsRead(ctx, db.MarkAllEmailsReadParams{
		UserID: userID, Src: q.Source, Status: q.Status, Q: q.Q, Link: q.Link,
		Unclassified: q.Unclassified, IncludeOther: q.ShowsOther(),
	})
	if err != nil {
		return nil, err
	}
	return &apigen.MarkAllReadInboxOK{Data: apigen.InboxMarked{Marked: marked}}, nil
}

// DeleteEmail soft-deletes one message, scoped to the caller (404 if not theirs).
func (h *inboxHandlers) DeleteEmail(ctx context.Context, params apigen.DeleteEmailParams) error {
	return h.setEmailDeleted(ctx, params.ID, true)
}

// RestoreEmail undoes a soft-delete, scoped to the caller (404 if not theirs).
func (h *inboxHandlers) RestoreEmail(ctx context.Context, params apigen.RestoreEmailParams) error {
	return h.setEmailDeleted(ctx, params.ID, false)
}

// setEmailDeleted flips one message's soft-delete flag (delete or restore),
// scoped to the caller. A message that is not theirs matches no row → 404.
//
// It answers 204, not Fiber's SendStatus(200): that helper writes the status text
// "OK" as the body, which is neither of this API's response shapes, and a client
// that decodes a 2xx body fails on a call that actually succeeded.
func (h *inboxHandlers) setEmailDeleted(ctx context.Context, rawID string, deleted bool) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := emailPathID(rawID)
	if err != nil {
		return err
	}
	var n int64
	if deleted {
		n, err = h.queries.SoftDeleteEmail(ctx, db.SoftDeleteEmailParams{ID: id, UserID: userID})
	} else {
		n, err = h.queries.RestoreEmail(ctx, db.RestoreEmailParams{ID: id, UserID: userID})
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return nil
}
