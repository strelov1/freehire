package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/subscription"
	"github.com/strelov1/freehire/internal/search/savedsearch"
	"github.com/strelov1/freehire/internal/search/search"
)

func (h *assistantHandlers) listSubscriptionsTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "list_subscriptions",
		Title:  "List job alerts",
		Effect: assistant.EffectReads,
		Description: "List the person's job alerts with every setting: id, name, a readable filter summary AND the " +
			"search as the structured object the alert tools take (query, filters, " +
			"salary_min/max; `other` holds website-only filters an update keeps), whether it is paused, and " +
			"each delivery channel with its active flag, pending matches and last delivery. delivery_schedule " +
			"is the account-wide digest timing every alert shares. Effect: none, read-only.",
		Schema: assistant.EmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			all, err := h.alerts(ctx, userID)
			if err != nil {
				return nil, err
			}
			schedule, err := h.deliverySchedule(ctx, userID, origin)
			if err != nil {
				return nil, err
			}
			if all == nil {
				all = []alert{}
			}
			return map[string]any{"alerts": all, "delivery_schedule": schedule}, nil
		}),
	}
}

func (h *assistantHandlers) updateSubscriptionTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:   "update_subscription",
		Title:  "Change a job alert",
		Effect: assistant.EffectOverwrites,
		Description: "Change one of the person's job alerts by its id. Every field is optional and only the ones " +
			"given change: name; the search (query, filters, salary_min, salary_max — each replaces that part, " +
			"an empty filters object clears the facet filters, a salary bound of 0 clears it; website-only " +
			"filters in `other` are kept); channels (the full set to deliver on — added ones must be connected, " +
			"missing ones stop); paused (true stops every channel, false resumes them). The new search must " +
			"still narrow the catalogue, and unknown values are dropped and named in ignored_params. " +
			alertExample + ". Digest timing is account-wide and not changed here. " +
			"Effect: overwrites the alert's name, search, channels or paused state; a removed channel's record " +
			"of matches is deleted with it.",
		Schema: map[string]any{
			"type": "object",
			"properties": alertSearchSchema(map[string]any{
				"id":   map[string]any{"type": "integer", "description": "The alert id."},
				"name": map[string]any{"type": "string", "description": "A new name (1-100 characters)."},
				"channels": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": notify.Channels},
					"description": "The complete set of channels to deliver on.",
				},
				"paused": map[string]any{"type": "boolean", "description": "true pauses every channel, false resumes them."},
			}),
			"required": []string{"id"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				alertSearchArgs
				ID       int64    `json:"id"`
				Name     *string  `json:"name"`
				Channels []string `json:"channels"`
				Paused   *bool    `json:"paused"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			current, err := h.ownAlert(ctx, userID, in.ID)
			if err != nil {
				return nil, err
			}

			// Everything is validated before the first write, so a refused change leaves the alert as it was.
			var newQuery *string
			var ignored []string
			if in.given() {
				vals, dropped, err := applyAlertSearch(savedsearch.ParseQuery(current.query), in.alertSearchArgs)
				if err != nil {
					return nil, err
				}
				ignored = dropped
				if !search.Narrows(vals) {
					return nil, fiber.NewError(fiber.StatusBadRequest, withIgnored("nothing was changed: "+unfilteredRefusal, ignored))
				}
				if q := savedsearch.EncodeQuery(vals); !savedsearch.SameSelection(q, current.query) {
					newQuery = &q
				}
			}
			var add, remove []alertChannel
			if raw := in.Channels; raw != nil {
				want := slices.Compact(slices.Sorted(slices.Values(nonEmptyStrings(raw))))
				if len(want) == 0 {
					return nil, fiber.NewError(fiber.StatusBadRequest, "nothing was changed: channels cannot be empty — to stop the alert use unsubscribe, to hold it use paused")
				}
				for _, ch := range want {
					if !slices.ContainsFunc(current.Channels, func(c alertChannel) bool { return c.Channel == ch }) {
						add = append(add, alertChannel{Channel: ch})
					}
				}
				for _, c := range current.Channels {
					if !slices.Contains(want, c.Channel) {
						remove = append(remove, c)
					}
				}
				if len(add) > 0 {
					names := make([]string, len(add))
					for i, c := range add {
						names[i] = c.Channel
					}
					if _, err := h.requireDeliverable(ctx, userID, names, origin); err != nil {
						return nil, err
					}
				}
			}

			if in.Name != nil || newQuery != nil {
				if _, err := h.savedSearches().Update(ctx, userID, current.ID, in.Name, newQuery); err != nil {
					name := ""
					if in.Name != nil {
						name = *in.Name
					}
					return nil, alertSavedSearchError(err, name)
				}
			}
			subs := h.subscriptions()
			for _, c := range add {
				if _, err := subs.Create(ctx, userID, current.ID, c.Channel); err != nil && !errors.Is(err, subscription.ErrDuplicate) {
					return nil, alertSavedSearchError(err, "")
				}
			}
			for _, c := range remove {
				if err := subs.Delete(ctx, userID, c.SubscriptionID); err != nil && !errors.Is(err, subscription.ErrNotFound) {
					return nil, err
				}
			}
			// paused applies to the channels the alert has AFTER the change, so a channel
			// added in the same call is held with the rest. Without paused, a channel added
			// to a paused alert joins it paused: the alert stays one state, not half of each.
			if in.Paused != nil || (current.Paused && len(add) > 0) {
				paused := current.Paused
				if in.Paused != nil {
					paused = *in.Paused
				}
				after, err := h.ownAlert(ctx, userID, current.ID)
				if err != nil {
					return nil, err
				}
				for _, c := range after.Channels {
					if c.Active == !paused {
						continue
					}
					if _, err := subs.SetActive(ctx, userID, c.SubscriptionID, !paused); err != nil {
						return nil, err
					}
				}
			}
			updated, err := h.ownAlert(ctx, userID, current.ID)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"alert": updated}
			if len(ignored) > 0 {
				out["ignored_params"] = ignored
			}
			return out, nil
		},
	}
}

func (h *assistantHandlers) unsubscribeTool() assistant.Tool {
	return assistant.Tool{
		Name:   "unsubscribe",
		Title:  "Stop a job alert",
		Effect: assistant.EffectOverwrites,
		Description: "Stop one job alert by its id, on every channel or on the one named. " +
			"Effect: deletes the alert's delivery on those channels and their record of matches; the saved " +
			"search itself stays on the account as a bookmarked search.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "integer", "description": "The alert id."},
				"channel": map[string]any{"type": "string", "enum": notify.Channels, "description": "Stop only this channel. Omit for all."},
			},
			"required": []string{"id"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID      int64  `json:"id"`
				Channel string `json:"channel"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			current, err := h.ownAlert(ctx, userID, in.ID)
			if err != nil {
				return nil, err
			}
			var stopped []string
			subs := h.subscriptions()
			for _, c := range current.Channels {
				if in.Channel != "" && c.Channel != in.Channel {
					continue
				}
				if err := subs.Delete(ctx, userID, c.SubscriptionID); err != nil && !errors.Is(err, subscription.ErrNotFound) {
					return nil, err
				}
				stopped = append(stopped, c.Channel)
			}
			if len(stopped) == 0 {
				return nil, fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("alert %d is not delivered on %s", in.ID, in.Channel))
			}
			return map[string]any{"id": in.ID, "stopped_channels": stopped}, nil
		},
	}
}

type subscriptionUpdate struct {
	Title     string     `json:"title"`
	Company   string     `json:"company"`
	Location  string     `json:"location,omitempty"`
	URL       string     `json:"url"`
	PostedAt  *time.Time `json:"posted_at,omitempty"`
	MatchedAt *time.Time `json:"matched_at,omitempty"`
	Delivered bool       `json:"delivered"`
	Alerts    []string   `json:"alerts"`
}
