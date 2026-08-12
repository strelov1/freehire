package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// notificationResponse is the public shape of one user_notifications row. No
// internal job id is exposed — only the public slug, same rule DigestJob
// already follows. public_slug/created_at/read_at use pgtype's own JSON
// marshaling (null when not Valid), the same convention pushTokenResponse
// already uses.
type notificationResponse struct {
	ID         int64              `json:"id"`
	Kind       string             `json:"kind"`
	Title      string             `json:"title"`
	Body       string             `json:"body"`
	PublicSlug pgtype.Text        `json:"public_slug"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
	ReadAt     pgtype.Timestamptz `json:"read_at"`
}

func toNotificationResponse(r db.ListUserNotificationsRow) notificationResponse {
	return notificationResponse{
		ID:         r.ID,
		Kind:       r.Kind,
		Title:      r.Title,
		Body:       r.Body,
		PublicSlug: r.PublicSlug,
		CreatedAt:  r.CreatedAt,
		ReadAt:     r.ReadAt,
	}
}

// GetNotifications returns the caller's own notification-center entries,
// newest first, standard offset/limit paging. meta.unread_count rides in the
// same response as the page, computed by one query alongside the total so the
// two numbers always describe the same underlying set (see CountUserNotifications).
// Cookie-only.
func (h *authHandlers) GetNotifications(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, offset := pageParamsBounded(c, defaultLimit, maxLimit)

	rows, err := h.queries.ListUserNotifications(c.Context(), db.ListUserNotificationsParams{
		UserID: userID, Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	counts, err := h.queries.CountUserNotifications(c.Context(), userID)
	if err != nil {
		return err
	}

	out := make([]notificationResponse, len(rows))
	for i, r := range rows {
		out[i] = toNotificationResponse(r)
	}
	return c.JSON(fiber.Map{
		"data": out,
		"meta": fiber.Map{
			"total":        counts.Total,
			"unread_count": counts.Unread,
			"limit":        limit,
			"offset":       offset,
		},
	})
}

// MarkNotificationRead marks one of the caller's own notifications read.
// Idempotent (an already-read notification is left unchanged, not an error);
// owner-scoped, so another user's id — or a nonexistent one — 404s rather
// than revealing which is which. Cookie-only.
func (h *authHandlers) MarkNotificationRead(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	affected, err := h.queries.MarkNotificationRead(c.Context(), db.MarkNotificationReadParams{
		ID: id, UserID: userID,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "notification not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// MarkAllNotificationsRead marks every currently-unread notification of the
// caller's read in one call, and reports how many it marked. Cookie-only.
func (h *authHandlers) MarkAllNotificationsRead(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	marked, err := h.queries.MarkAllNotificationsRead(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"marked": marked}})
}
