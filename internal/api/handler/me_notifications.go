package handler

import (
	"context"
	"encoding/json"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// notificationOut is the public shape of one user_notifications row. No internal job id
// is exposed — only the public slug, same rule DigestJob already follows. jobs passes
// the stored JSON straight through (empty for every kind but a multi-job digest, and
// then left out of the answer).
func notificationOut(id int64, kind, title, body string, publicSlug pgtype.Text, jobs json.RawMessage, createdAt, readAt pgtype.Timestamptz) apigen.Notification {
	out := apigen.Notification{
		ID: id, Kind: kind, Title: title, Body: body,
		PublicSlug: nilString(nil),
		Jobs:       jx.Raw(jobs),
		CreatedAt:  nilDateTime(pgconv.TimePtr(createdAt)),
		ReadAt:     nilDateTime(pgconv.TimePtr(readAt)),
	}
	if publicSlug.Valid {
		out.PublicSlug = nilString(&publicSlug.String)
	}
	return out
}

// ListNotifications returns the caller's own notification-center entries,
// newest first, standard offset/limit paging. meta.unread_count rides in the
// same response as the page, computed by one query alongside the total so the
// two numbers always describe the same underlying set (see CountUserNotifications).
// Cookie-only.
func (h *authHandlers) ListNotifications(ctx context.Context, _ apigen.ListNotificationsParams) (*apigen.NotificationPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := contractPageBounded(ctx, defaultLimit, maxLimit)

	rows, err := h.queries.ListUserNotifications(ctx, db.ListUserNotificationsParams{
		UserID: userID, Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	counts, err := h.queries.CountUserNotifications(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]apigen.Notification, len(rows))
	for i, r := range rows {
		out[i] = notificationOut(r.ID, r.Kind, r.Title, r.Body, r.PublicSlug, r.Jobs, r.CreatedAt, r.ReadAt)
	}
	return &apigen.NotificationPage{
		Data: out,
		Meta: apigen.NotificationListMeta{
			Total: counts.Total, UnreadCount: counts.Unread, Limit: int64(limit), Offset: int64(offset),
		},
	}, nil
}

// GetNotification returns one of the caller's own notifications, including
// its jobs snapshot when it has one — the read a direct visit or bookmark of
// /my/notifications/[id]/jobs needs, since the list page alone only ever
// serves whatever offset the caller last paged to. Owner-scoped: another
// user's id — or a nonexistent one — 404s rather than revealing which.
// Cookie-only.
func (h *authHandlers) GetNotification(ctx context.Context, params apigen.GetNotificationParams) (*apigen.GetNotificationOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}

	r, err := h.queries.GetNotification(ctx, db.GetNotificationParams{ID: id, UserID: userID})
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404 via the central error handler
	}
	return &apigen.GetNotificationOK{Data: notificationOut(r.ID, r.Kind, r.Title, r.Body, r.PublicSlug, r.Jobs, r.CreatedAt, r.ReadAt)}, nil
}

// MarkNotificationRead marks one of the caller's own notifications read.
// Idempotent (an already-read notification is left unchanged, not an error);
// owner-scoped, so another user's id — or a nonexistent one — 404s rather
// than revealing which is which. Cookie-only.
func (h *authHandlers) MarkNotificationRead(ctx context.Context, params apigen.MarkNotificationReadParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}

	affected, err := h.queries.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		ID: id, UserID: userID,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "notification not found")
	}
	return nil
}

// MarkAllNotificationsRead marks every currently-unread notification of the
// caller's read in one call, and reports how many it marked. Cookie-only.
func (h *authHandlers) MarkAllNotificationsRead(ctx context.Context) (*apigen.MarkAllNotificationsReadOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	marked, err := h.queries.MarkAllNotificationsRead(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.MarkAllNotificationsReadOK{Data: apigen.NotificationsMarked{Marked: marked}}, nil
}
