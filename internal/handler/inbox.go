package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// inboxGroup is one subject-grouped bucket in the inbox listing.
type inboxGroup struct {
	Key            string    `json:"key"`     // normalized subject (group key)
	Subject        string    `json:"subject"` // newest message's original subject
	MessageCount   int64     `json:"message_count"`
	LatestReceived time.Time `json:"latest_received"`
	Senders        []string  `json:"senders"`
}

// GetInbox returns the caller's ATS mail grouped by normalized subject, newest
// group first.
func (a *API) GetInbox(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	q := c.Query("q")
	limit, offset := pageParams(c) // default 20, clamped
	rows, err := a.queries.ListInboxGroups(c.Context(), db.ListInboxGroupsParams{
		UserID: userID, Q: q, Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := a.queries.CountInboxGroups(c.Context(), db.CountInboxGroupsParams{UserID: userID, Q: q})
	if err != nil {
		return err
	}
	out := make([]inboxGroup, 0, len(rows))
	for _, r := range rows {
		out = append(out, inboxGroup{
			Key: r.SubjectNorm, Subject: r.LatestSubject,
			MessageCount: r.MessageCount, LatestReceived: r.LatestReceived.Time,
			Senders: r.Senders,
		})
	}
	return listResponse(c, out, total, limit, offset)
}

// GetInboxGroup returns one subject group's messages, newest first. The group key
// (a normalized subject, which may contain spaces) is passed as ?key=.
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

// GetEmail returns one message body, scoped to the caller (404 for another user's).
func (a *API) GetEmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	email, err := a.queries.GetEmail(c.Context(), db.GetEmailParams{ID: int64(id), UserID: userID})
	if err != nil {
		return err // pgx.ErrNoRows → 404 via the central error handler
	}
	return c.JSON(fiber.Map{"data": email})
}
