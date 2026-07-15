package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailclassify"
)

// inboxSources is the account-switcher vocabulary: "" means all accounts.
var inboxSources = map[string]bool{"": true, "gmail": true, "hosted": true}

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

// inboxMessage is one row in the flat inbox listing.
type inboxMessage struct {
	ID         int64     `json:"id"`
	Source     string    `json:"source"`
	ExternalID string    `json:"external_id"`
	FromAddr   string    `json:"from_addr"`
	FromName   string    `json:"from_name"`
	Subject    string    `json:"subject"`
	Snippet    string    `json:"snippet"`
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
// ?source=(gmail|hosted), ?unread=1, ?status=<signal>, ?q=<term>.
type inboxFilters struct {
	Source string
	Unread bool
	Status string
	Q      string
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
	return inboxFilters{Source: src, Unread: c.QueryBool("unread"), Status: status, Q: c.Query("q")}, nil
}

// GetInbox returns the caller's mail as a flat list, newest first, excluding
// soft-deleted messages. Optional filters: ?source= (account switcher), ?unread=1
// (hide read), ?status= (one classified label), ?q= (subject/sender/body search);
// standard limit/offset pagination.
func (a *API) GetInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	f, err := parseInboxFilters(c)
	if err != nil {
		return err
	}
	limit, offset := pageParams(c) // default 20, clamped
	rows, err := a.queries.ListEmails(c.Context(), db.ListEmailsParams{
		UserID: userID, Src: f.Source, Unread: f.Unread, Status: f.Status, Q: f.Q,
		Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := a.queries.CountEmails(c.Context(), db.CountEmailsParams{
		UserID: userID, Src: f.Source, Unread: f.Unread, Status: f.Status, Q: f.Q,
	})
	if err != nil {
		return err
	}
	out := make([]inboxMessage, 0, len(rows))
	for _, r := range rows {
		out = append(out, inboxMessage{
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
		})
	}
	return listResponse(c, out, total, limit, offset)
}

// GetEmail returns one message body, scoped to the caller (404 for another user's),
// and marks it read on open (best-effort — a failed mark never blocks reading).
func (a *API) GetEmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	row, err := a.queries.GetEmail(c.Context(), db.GetEmailParams{ID: int64(id), UserID: userID})
	if err != nil {
		return err // pgx.ErrNoRows → 404 via the central error handler
	}
	_ = a.queries.MarkEmailRead(c.Context(), db.MarkEmailReadParams{ID: row.ID, UserID: userID})
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
func (a *API) MarkAllReadInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	f, err := parseInboxFilters(c)
	if err != nil {
		return err
	}
	marked, err := a.queries.MarkAllEmailsRead(c.Context(), db.MarkAllEmailsReadParams{
		UserID: userID, Src: f.Source, Status: f.Status, Q: f.Q,
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"marked": marked}})
}

// DeleteEmail soft-deletes one message, scoped to the caller (404 if not theirs).
func (a *API) DeleteEmail(c *fiber.Ctx) error {
	return a.setEmailDeleted(c, true)
}

// RestoreEmail undoes a soft-delete, scoped to the caller (404 if not theirs).
func (a *API) RestoreEmail(c *fiber.Ctx) error {
	return a.setEmailDeleted(c, false)
}

// setEmailDeleted flips one message's soft-delete flag (delete or restore),
// scoped to the caller. A message that is not theirs matches no row → 404.
func (a *API) setEmailDeleted(c *fiber.Ctx, deleted bool) error {
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
		n, err = a.queries.SoftDeleteEmail(c.Context(), db.SoftDeleteEmailParams{ID: int64(id), UserID: userID})
	} else {
		n, err = a.queries.RestoreEmail(c.Context(), db.RestoreEmailParams{ID: int64(id), UserID: userID})
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return c.SendStatus(fiber.StatusOK)
}
