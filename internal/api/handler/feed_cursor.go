package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/search/feed"
)

const feedSourceProfile = "profile"

// profileVersion is the filter version a cursor binds: the profile's UpdatedAt, which
// also moves when excluded_companies does.
func profileVersion(p userprofile.Profile) int64 {
	if p.UpdatedAt == nil {
		return 0
	}
	return p.UpdatedAt.UnixMicro()
}

// pageCursor reads ?cursor=. Without one it starts a session: snapshot now, offset 0.
// A bad, expired or foreign cursor is a 400 and the client restarts from page 1.
func (h *feedHandlers) pageCursor(c *fiber.Ctx, userID int64) (feed.Cursor, error) {
	raw := c.Query("cursor")
	if raw == "" {
		now := time.Now()
		// Rounded up to the next whole second: a save made earlier in this same second is
		// still before the snapshot, so a refetch right after an action leaves the job out.
		return feed.Cursor{UserID: userID, Source: feedSourceProfile, SnapshotAt: now.Unix() + 1, ExpiresAt: now.Add(feed.CursorTTL).Unix()}, nil
	}
	cur, err := h.cursors.Verify(raw, time.Now())
	if err != nil || cur.UserID != userID || cur.Source != feedSourceProfile {
		return feed.Cursor{}, fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	return cur, nil
}

// nextCursor is the signed window after this page, or "" on a short page, at the total,
// or past maxPageWindow.
func (h *feedHandlers) nextCursor(cur feed.Cursor, version int64, limit, got int, total int64) string {
	next := cur.Offset + got
	if got < limit || int64(next) >= total || next+limit > maxPageWindow {
		return ""
	}
	cur.Offset, cur.Version = next, version
	return h.cursors.Sign(cur)
}
