package handler

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailclassify"
	"github.com/strelov1/freehire/internal/maillink"
)

// inboxSources is the account-switcher vocabulary: "" means all accounts.
// 'external' is mail the caller's own agent harness pushed; it reads like any
// other source, it is simply never classified server-side.
var inboxSources = map[string]bool{"": true, "gmail": true, "hosted": true, "external": true}

// agentPageMax caps a listing that carries bodies. Bodies are the one listing
// payload heavy enough to matter, and an agent sweeping its backlog has no reason
// to pull an unbounded page.
const agentPageMax = 50

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

// inboxFilters are the shared listing filters carried by the query string:
// ?source=(gmail|hosted|external), ?unread=1, ?status=<signal>, ?q=<term>,
// ?unclassified=1.
type inboxFilters struct {
	Source string
	// IsUnread hides mail the reader has already opened.
	IsUnread bool
	Status   string
	Q        string
	// IsUnclassified narrows to mail carrying no classification stamp — the agent's
	// work queue. It is distinct from IsUnread: read_at tracks a human's attention,
	// classified_at tracks whether anything has judged the message yet.
	IsUnclassified bool
}

// parseInboxFilters reads and validates the inbox filter query params. Source and
// status are validated against their vocabularies; an unknown value is a 400
// rather than a silently empty listing.
func parseInboxFilters(c *fiber.Ctx) (inboxFilters, error) {
	src := c.Query("source")
	if !inboxSources[src] {
		return inboxFilters{}, fiber.NewError(fiber.StatusBadRequest, "unknown source")
	}
	status := c.Query("status")
	if status != "" && !mailclassify.IsValidSignal(status) {
		return inboxFilters{}, fiber.NewError(fiber.StatusBadRequest, "unknown label")
	}
	return inboxFilters{
		Source: src, IsUnread: c.QueryBool("unread"), Status: status, Q: c.Query("q"),
		IsUnclassified: c.QueryBool("unclassified"),
	}, nil
}

// GetInbox returns the caller's mail as a flat list, newest first, excluding
// soft-deleted messages. Optional filters: ?source= (account switcher), ?unread=1
// (hide read), ?status= (one classified label), ?unclassified=1 (awaiting triage),
// ?q= (subject/sender/body search); standard limit/offset pagination.
//
// ?body=1 additionally returns each message's readable body. That is the agent's
// read path: it triages a page in one request, and — unlike GetEmail — marks
// nothing read, so a harness sweeping the backlog never zeroes its owner's unread
// count. Pages carrying bodies are capped at agentPageMax.
func (h *inboxHandlers) GetInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	f, err := parseInboxFilters(c)
	if err != nil {
		return err
	}
	withBody := c.QueryBool("body")
	ceiling := maxLimit
	if withBody {
		ceiling = agentPageMax
	}
	limit, offset := pageParamsMax(c, ceiling)
	rows, err := h.queries.ListEmails(c.Context(), db.ListEmailsParams{
		UserID: userID, Src: f.Source, Unread: f.IsUnread, Status: f.Status, Q: f.Q,
		Unclassified: f.IsUnclassified, WithBody: withBody,
		Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := h.queries.CountEmails(c.Context(), db.CountEmailsParams{
		UserID: userID, Src: f.Source, Unread: f.IsUnread, Status: f.Status, Q: f.Q,
		Unclassified: f.IsUnclassified,
	})
	if err != nil {
		return err
	}
	out := make([]inboxMessage, 0, len(rows))
	for _, r := range rows {
		m := inboxMessage{
			ID: r.ID, Source: r.Source, ExternalID: r.ExternalID,
			FromAddr: r.FromAddr, FromName: r.FromName, Subject: r.Subject,
			Snippet: r.Snippet, ReceivedAt: r.ReceivedAt.Time, Read: r.Read,
			emailLinking: emailLinking{
				StatusSignal:     pgStr(r.StatusSignal),
				LinkSource:       pgStr(r.LinkSource),
				LinkedSlug:       pgStr(r.LinkedSlug),
				LinkedCompany:    pgStr(r.LinkedCompany),
				SuggestedSlug:    pgStr(r.SuggestedSlug),
				SuggestedCompany: pgStr(r.SuggestedCompany),
			},
		}
		if withBody {
			// The same body the classification worker reads, so what our LLM judges
			// and what a caller's agent judges cannot drift.
			m.BodyText = maillink.ReadableBody(r.BodyText, r.BodyHtml)
		}
		out = append(out, m)
	}
	return listResponse(c, out, total, limit, offset)
}

// GetEmail returns one message body, scoped to the caller (404 for another user's),
// and marks it read on open (best-effort — a failed mark never blocks reading).
func (h *inboxHandlers) GetEmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	row, err := h.queries.GetEmail(c.Context(), db.GetEmailParams{ID: int64(id), UserID: userID})
	if err != nil {
		return err // pgx.ErrNoRows → 404 via the central error handler
	}
	if err := h.queries.MarkEmailRead(c.Context(), db.MarkEmailReadParams{ID: row.ID, UserID: userID}); err != nil {
		log.Printf("inbox: mark read user=%d email=%d: %v", userID, row.ID, err)
	}
	return c.JSON(fiber.Map{"data": emailBody{
		ID: row.ID, Source: row.Source, ExternalID: row.ExternalID,
		FromAddr: row.FromAddr, FromName: row.FromName, Subject: row.Subject,
		BodyText: row.BodyText, BodyHTML: row.BodyHtml,
		ReceivedAt: row.ReceivedAt.Time, Read: row.Read,
		emailLinking: emailLinking{
			StatusSignal:     pgStr(row.StatusSignal),
			LinkSource:       pgStr(row.LinkSource),
			LinkedSlug:       pgStr(row.LinkedSlug),
			LinkedCompany:    pgStr(row.LinkedCompany),
			SuggestedSlug:    pgStr(row.SuggestedSlug),
			SuggestedCompany: pgStr(row.SuggestedCompany),
		},
	}})
}

// MarkAllReadInbox marks every unread message matching the caller's active
// filters (source/status/search) as read and reports how many it marked. The
// unread filter is implicit — the query only ever touches unread rows.
func (h *inboxHandlers) MarkAllReadInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	f, err := parseInboxFilters(c)
	if err != nil {
		return err
	}
	marked, err := h.queries.MarkAllEmailsRead(c.Context(), db.MarkAllEmailsReadParams{
		UserID: userID, Src: f.Source, Status: f.Status, Q: f.Q,
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
