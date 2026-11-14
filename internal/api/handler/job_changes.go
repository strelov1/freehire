package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	defaultChangesLimit = 100
	maxChangesLimit     = 100
	// maxChangesLookback bounds `since`: the log keeps growing, and a mirror that is further
	// behind than this should rebuild from search instead of replaying history.
	maxChangesLookback = 30 * 24 * time.Hour
)

// changesCursor is a position in the feed: the (xid, id) of the last change served. The
// handler never reads it as anything but "after this", and a client never reads it at all.
// The log is never pruned, so a cursor never expires.
type changesCursor struct{ xid, id int64 }

func (c changesCursor) encode() string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("v1:%d:%d", c.xid, c.id)))
}

func decodeChangesCursor(s string) (changesCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return changesCursor{}, errBadChangesCursor
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return changesCursor{}, errBadChangesCursor
	}
	xid, err1 := strconv.ParseInt(parts[1], 10, 64)
	id, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || xid < 0 || id < 0 {
		return changesCursor{}, errBadChangesCursor
	}
	return changesCursor{xid: xid, id: id}, nil
}

var errBadChangesCursor = fiber.NewError(fiber.StatusBadRequest, "invalid cursor")

// ListJobChanges serves the public change feed: what opened, closed and reopened, oldest
// first, after a keyset position. The log and its ordering rules are in migration 0263 and
// queries/job_changes.sql. Public like the other job reads; no tier gating.
func (h *jobsHandlers) ListJobChanges(ctx context.Context, params apigen.ListJobChangesParams) (*apigen.JobChangesPage, error) {
	limit, _ := contractPageBounded(ctx, defaultChangesLimit, maxChangesLimit)

	// Validation first: a refused call costs no database work.
	var (
		pos      changesCursor
		hasPos   bool
		since    time.Time
		cursorIn = strings.TrimSpace(params.Cursor.Or(""))
		sinceIn  = strings.TrimSpace(params.Since.Or(""))
	)
	switch {
	case cursorIn != "":
		var err error
		if pos, err = decodeChangesCursor(cursorIn); err != nil {
			return nil, err
		}
		hasPos = true
	case sinceIn != "":
		var err error
		if since, err = time.Parse(time.RFC3339, sinceIn); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "since must be an RFC 3339 time")
		}
		if since.Before(time.Now().Add(-maxChangesLookback)) {
			return nil, fiber.NewError(fiber.StatusBadRequest, "since is more than 30 days back")
		}
	default:
		return nil, fiber.NewError(fiber.StatusBadRequest, "since or cursor is required")
	}

	meta := apigen.JobChangesMeta{Limit: int64(limit)}
	if avail, err := h.queries.JobChangesAvailableSince(ctx); err != nil {
		return nil, err
	} else if avail.Valid {
		meta.AvailableSince = apigen.NewOptDateTime(avail.Time)
	}

	if !hasPos {
		xid, err := h.queries.JobChangesStartXID(ctx, pgtype.Timestamptz{Time: since, Valid: true})
		if err != nil {
			return nil, err
		}
		pos = changesCursor{xid: xid}
	}

	// One extra row says whether more is waiting without a count.
	rows, err := h.queries.ListJobChanges(ctx, db.ListJobChangesParams{
		AfterXid: pos.xid, AfterID: pos.id, RowLimit: int32(limit) + 1,
	})
	if err != nil {
		return nil, err
	}
	if meta.HasMore = len(rows) > limit; meta.HasMore {
		rows = rows[:limit]
	}

	data := make([]apigen.JobChange, len(rows))
	for i, r := range rows {
		data[i] = apigen.JobChange{
			PublicSlug:   r.PublicSlug,
			Seq:          r.ID,
			Kind:         apigen.JobChangeKind(r.Kind),
			At:           r.At.Time,
			ClosedReason: optString(r.Reason),
		}
	}
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		pos = changesCursor{xid: last.Xid, id: last.ID}
	}
	// Always: even an empty first page from `since` has a position, so a client can store it
	// and resume after a quiet spell without `since`'s 30-day cap forcing a resync.
	meta.NextCursor = pos.encode()
	return &apigen.JobChangesPage{Data: data, Meta: meta}, nil
}
