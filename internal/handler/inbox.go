package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
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

// GetInbox returns the caller's mail as a flat list, newest first. An optional
// ?source= (gmail|hosted) filters to one account (the switcher); ?q= searches
// subject/sender/body; standard limit/offset pagination.
func (a *API) GetInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	src := c.Query("source")
	if !inboxSources[src] {
		return fiber.NewError(fiber.StatusBadRequest, "unknown source")
	}
	q := c.Query("q")
	limit, offset := pageParams(c) // default 20, clamped
	rows, err := a.queries.ListEmails(c.Context(), db.ListEmailsParams{
		UserID: userID, Src: src, Q: q, Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := a.queries.CountEmails(c.Context(), db.CountEmailsParams{UserID: userID, Src: src, Q: q})
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
