package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/search/feed"
)

// profileVersion is the filter version a cursor binds: the profile's UpdatedAt, which
// also moves when excluded_companies does.
func profileVersion(p userprofile.Profile) int64 {
	if p.UpdatedAt == nil {
		return 0
	}
	return p.UpdatedAt.UnixMicro()
}

// pageCursor reads ?cursor=. Without one it starts a session: it reads (or creates) the
// source's watermark, fixes the snapshot below the drain high-water, and begins in the new
// phase when anything can be new. source is the resolved source key: profile or search:<id>.
// A bad, expired or foreign cursor, or one for another source, is a 400 and the client
// restarts from page 1.
func (h *feedHandlers) pageCursor(c *fiber.Ctx, userID int64, source string) (feed.Cursor, error) {
	raw := c.Query("cursor")
	if raw == "" {
		now := time.Now()
		// Rounded up to the next whole second: a save made earlier in this same second is
		// still before the cutoff, so a refetch right after an action leaves the job out.
		acted := now.Unix() + 1
		// The job snapshot is the last fully elapsed second: a job created later in the current
		// one may land after this read, and must still be new on the next visit.
		snap := now.Unix() - 1
		hw, ok, err := h.state.SearchDrainHighWater(c.Context())
		if err != nil {
			return feed.Cursor{}, err
		}
		// A job still in search_outbox is not in the index yet; staying strictly below its
		// second keeps it out of this session, so the next visit sees it as new.
		if ok && hw.Unix()-1 < snap {
			snap = hw.Unix() - 1
		}
		seen, err := h.state.GetOrInitFeedSeen(c.Context(), userID, source, time.Unix(snap, 0))
		if err != nil {
			return feed.Cursor{}, err
		}
		seenAt := min(seen.Unix(), snap) // a watermark ahead of this snapshot leaves nothing new
		phase := feed.PhaseRest
		if seenAt < snap {
			phase = feed.PhaseNew
		}
		return feed.Cursor{UserID: userID, Source: source, SnapshotAt: snap, SeenAt: seenAt, ActedBefore: acted, Phase: phase, ExpiresAt: now.Add(feed.CursorTTL).Unix()}, nil
	}
	cur, err := h.verifyCursor(raw, userID)
	// A cursor for another source (the choice changed) restarts the client.
	if err != nil || cur.Source != source || (cur.Phase != feed.PhaseNew && cur.Phase != feed.PhaseRest) {
		return feed.Cursor{}, fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	return cur, nil
}

func (h *feedHandlers) verifyCursor(raw string, userID int64) (feed.Cursor, error) {
	cur, err := h.cursors.Verify(raw, time.Now())
	if err != nil || cur.UserID != userID {
		return feed.Cursor{}, fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	return cur, nil
}

// nextCursor is the signed window after this page (in cur's phase), or "" on a short page,
// at the total, or past maxPageWindow.
func (h *feedHandlers) nextCursor(cur feed.Cursor, version int64, limit, got int, total int64) string {
	next := cur.Offset + got
	if got < limit || int64(next) >= total || next+limit > maxPageWindow {
		return ""
	}
	cur.Offset, cur.Version = next, version
	return h.cursors.Sign(cur)
}
