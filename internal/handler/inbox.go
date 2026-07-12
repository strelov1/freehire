package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// inboxSources is the account-switcher vocabulary: "" means all accounts.
var inboxSources = map[string]bool{"": true, "gmail": true, "hosted": true}

// inboxGroup is one subject-grouped bucket in the inbox listing.
type inboxGroup struct {
	Key            string    `json:"key"`     // normalized subject (group key)
	Subject        string    `json:"subject"` // newest message's original subject
	MessageCount   int64     `json:"message_count"`
	UnreadCount    int64     `json:"unread_count"`
	LatestReceived time.Time `json:"latest_received"`
	Senders        []string  `json:"senders"`
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
}

// GetInbox returns the caller's mail grouped by normalized subject, newest group
// first. An optional ?source= (gmail|hosted) filters to one account (the switcher).
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
	rows, err := a.queries.ListInboxGroups(c.Context(), db.ListInboxGroupsParams{
		UserID: userID, Src: src, Q: q, Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := a.queries.CountInboxGroups(c.Context(), db.CountInboxGroupsParams{UserID: userID, Src: src, Q: q})
	if err != nil {
		return err
	}
	out := make([]inboxGroup, 0, len(rows))
	for _, r := range rows {
		out = append(out, inboxGroup{
			Key: r.SubjectNorm, Subject: r.LatestSubject,
			MessageCount: r.MessageCount, UnreadCount: r.UnreadCount,
			LatestReceived: r.LatestReceived.Time, Senders: r.Senders,
		})
	}
	return listResponse(c, out, total, limit, offset)
}

// GetInboxGroup returns one subject group's messages, newest first. The group key
// (a normalized subject, which may contain spaces) is passed as ?key=. A group may
// span sources, so it is not source-filtered.
func (a *API) GetInboxGroup(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := a.queries.ListEmailsByGroup(c.Context(), db.ListEmailsByGroupParams{
		UserID: userID, SubjectNorm: c.Query("key"),
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": rows})
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
	}})
}
