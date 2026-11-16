package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

func (h *assistantHandlers) subscriptionUpdatesTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "subscription_updates",
		Title:  "Read what job alerts matched",
		Effect: assistant.EffectReads,
		Description: "List the new postings the person's job alerts have matched — the same matches their alerts " +
			"are built from, newest first, still-open postings only. Narrow to one alert by id, and to a window " +
			"by since (RFC 3339) or days (default 7, max 30). An alert only matches postings that appeared after " +
			"it was created, and matching runs every few minutes, so a new one starts empty. delivered says " +
			"whether an alert already carried the posting. Effect: none, read-only.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "integer", "description": "One alert's id; omit for all."},
				"since": map[string]any{"type": "string", "description": "Only matches recorded at or after this RFC 3339 time."},
				"days":  map[string]any{"type": "integer", "description": fmt.Sprintf("Look back this many days (default %d, max %d). Ignored when since is given.", subscriptionUpdatesDefaultDays, subscriptionUpdatesMaxDays)},
				"limit": map[string]any{"type": "integer", "description": fmt.Sprintf("How many postings (default %d, max %d).", subscriptionUpdatesDefaultMax, subscriptionUpdatesMax)},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID    int64  `json:"id"`
				Since string `json:"since"`
				Days  int    `json:"days"`
				Limit int    `json:"limit"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			since, err := updatesSince(time.Now(), in.Since, in.Days)
			if err != nil {
				return nil, err
			}
			limit := in.Limit
			if limit <= 0 {
				limit = subscriptionUpdatesDefaultMax
			}
			limit = min(limit, subscriptionUpdatesMax)

			var savedID pgtype.Int8
			if in.ID != 0 {
				// Said out loud rather than answered with an empty list: the SQL is owner-scoped
				// either way, but "nothing new" for an id that is not this account's is a lie.
				if _, err := h.ownAlert(ctx, userID, in.ID); err != nil {
					return nil, err
				}
				savedID = pgtype.Int8{Int64: in.ID, Valid: true}
			}

			// Read several rows per posting asked for: one alert on two channels records each
			// posting twice, and the duplicates fold below. A full read means there may be more.
			rowLimit := limit*4 + 1
			rows, err := h.queries.ListSubscriptionUpdates(ctx, db.ListSubscriptionUpdatesParams{
				UserID: userID, SavedSearchID: savedID, Since: pgconv.Timestamptz(&since), RowLimit: int32(rowLimit),
			})
			if err != nil {
				return nil, err
			}
			out, truncated := foldUpdates(rows, origin, limit)
			truncated = truncated || len(rows) == rowLimit
			return map[string]any{"since": since.UTC().Format(time.RFC3339), "postings": out, "truncated": truncated}, nil
		},
	}
}

// updatesSince resolves the look-back window: an explicit since wins, otherwise days
// (bounded) back from now.
func updatesSince(now time.Time, since string, days int) (time.Time, error) {
	if since = strings.TrimSpace(since); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return time.Time{}, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("since must be an RFC 3339 time such as 2026-10-20T00:00:00Z: %v", err))
		}
		return t, nil
	}
	if days <= 0 {
		days = subscriptionUpdatesDefaultDays
	}
	days = min(days, subscriptionUpdatesMaxDays)
	return now.AddDate(0, 0, -days), nil
}

// foldUpdates turns ledger rows into one entry per posting, newest match first, naming
// every alert that matched it. A posting is delivered once ANY of its matches was.
func foldUpdates(rows []db.ListSubscriptionUpdatesRow, origin string, limit int) ([]subscriptionUpdate, bool) {
	out := []subscriptionUpdate{}
	index := map[string]int{}
	truncated := false
	for _, r := range rows {
		if i, seen := index[r.PublicSlug]; seen {
			if !slices.Contains(out[i].Alerts, r.SavedSearchName) {
				out[i].Alerts = append(out[i].Alerts, r.SavedSearchName)
			}
			out[i].Delivered = out[i].Delivered || r.NotifiedAt.Valid
			continue
		}
		if len(out) == limit {
			truncated = true
			break
		}
		index[r.PublicSlug] = len(out)
		out = append(out, subscriptionUpdate{
			Title: r.Title, Company: r.Company, Location: r.Location,
			URL:      jobURL(origin, r.PublicSlug),
			PostedAt: pgconv.TimePtr(r.PostedAt), MatchedAt: pgconv.TimePtr(r.MatchedAt),
			Delivered: r.NotifiedAt.Valid, Alerts: []string{r.SavedSearchName},
		})
	}
	return out, truncated
}
