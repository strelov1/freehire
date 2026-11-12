package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// The interview-buddy and mock-interview tools of the signed-in MCP server: an agent
// connected for a person can join the network, find a host, book or cancel a mock, and keep
// the person's own hosting schedule. Each tool calls the same service method as the cookie
// route beside it (buddies.go, buddy_mocks.go) and maps errors through the same buddyError /
// mockError, so the two doors cannot disagree. Served to OAuth grants only (mcpAccountTools):
// every buddy route is cookie-only, so a leaked API key must not book or read mocks here
// either.

// mcpTools builds the buddy tools. Building touches no field, so a nil receiver is safe
// until a tool runs.
func (h *buddyHandlers) mcpTools() []assistant.Tool {
	return []assistant.Tool{
		h.statusTool(), h.joinTool(), h.leaveTool(),
		h.mockHostsTool(), h.mockSlotsTool(), h.mockBookTool(), h.mockListTool(), h.mockCancelTool(),
		h.mockScheduleGetTool(), h.mockSettingsSetTool(), h.mockScheduleSetWeeklyTool(),
		h.mockScheduleAddOverrideTool(), h.mockScheduleDeleteTool(),
	}
}

// mockToolError is mockError for a tool. The route turns a broken host schedule into a
// generic 500 and reports the wrapped detail; a tool hands its error text to the agent
// verbatim, and that detail names the host's account id, which a member never sees.
func mockToolError(ctx context.Context, err error) error {
	if errors.Is(err, slots.ErrNoOwnerZone) {
		observability.RequestLogger(ctx).ErrorContext(ctx, "buddy mock: host schedule misconfigured", "error", err)
		return fiber.NewError(fiber.StatusInternalServerError, "this member's schedule is unavailable right now")
	}
	return mockError(err)
}

var timezoneProperty = map[string]any{"type": "string", "description": "IANA time zone the times are shown in, e.g. Europe/Berlin. An unknown or missing zone falls back to UTC; the result names the zone it used."}

var clockProperties = map[string]any{
	"start": map[string]any{"type": "string", "description": "Start of the window, HH:MM in the person's mock time zone."},
	"end":   map[string]any{"type": "string", "description": "End of the window, HH:MM; 24:00 means until midnight."},
}

func (h *buddyHandlers) status(ctx context.Context, userID int64) (buddyStatusResponse, error) {
	st, err := h.buddy.Status(ctx, userID)
	if err != nil {
		return buddyStatusResponse{}, err
	}
	return buddyStatusResponse{Member: st.Member, Handle: st.Handle, CardEmpty: st.EmptyCard}, nil
}

func (h *buddyHandlers) statusTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_status", Title: "Interview buddy membership", Effect: assistant.EffectReads,
		Description: "Whether the person is a member of freehire's interview buddy network, and their " +
			"anonymous handle there. Members are known to each other only by handle. Membership is needed " +
			"for every mock-interview action.",
		Schema: assistant.ClosedEmptySchema,
		// No Touch, unlike GET /me/buddies: a read-only tool an agent may poll must not
		// rewrite the member's ranking keys on every call.
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			return h.status(ctx, userID)
		}),
	}
}

func (h *buddyHandlers) joinTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_join", Title: "Join the interview buddy network", Effect: assistant.EffectAdds,
		Description: "Join freehire's interview buddy network under a generated anonymous handle, or rejoin " +
			"under the old one. Members can book and host 60-minute mock interviews with each other. " +
			"Returns the membership status.",
		Schema: assistant.ClosedEmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			if _, err := h.buddy.Join(ctx, userID); err != nil {
				return nil, err
			}
			return h.status(ctx, userID)
		}),
	}
}

func (h *buddyHandlers) leaveTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_leave", Title: "Leave the interview buddy network", Effect: assistant.EffectOverwrites,
		Description: "Leave the interview buddy network. Closes the person's buddy conversations and cancels " +
			"every upcoming mock interview they host or attend; the other participants are told. Rejoining " +
			"later restores the same handle.",
		Schema: assistant.ClosedEmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			if err := h.buddy.Leave(ctx, userID); err != nil {
				return nil, buddyError(err)
			}
			// The leave cancelled the member's future mocks in its own statement; this tells
			// the other participants, as the HTTP route does.
			h.mocks.AnnounceCancellations(ctx, userID)
			return map[string]any{"left": true}, nil
		}),
	}
}

func (h *buddyHandlers) mockHostsTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_hosts", Title: "Find mock interview hosts", Effect: assistant.EffectReads,
		Description: "Members of the buddy network the person could book a 60-minute mock interview with now, " +
			"soonest available first: each host's handle, a coarse card (category, seniority, country), the " +
			"topics they offer, their next open slot and their review rating. Never includes a meeting link.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic":    map[string]any{"type": "string", "enum": buddy.MockTopics, "description": "Only hosts offering this topic."},
				"timezone": timezoneProperty,
			},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Topic    string `json:"topic"`
				Timezone string `json:"timezone"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			res, err := h.mocks.Hosts(ctx, userID, in.Topic, in.Timezone)
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			return map[string]any{"hosts": toBuddyMockHostResponses(res), "timezone": res.Zone}, nil
		},
	}
}

func (h *buddyHandlers) mockSlotsTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_slots", Title: "List a host's open mock slots", Effect: assistant.EffectReads,
		Description: "The open 60-minute mock-interview slots of one host, by handle, and the topics that host " +
			"offers. Each slot's starts_at is the exact RFC 3339 instant to book. Covers the next 30 days " +
			"unless from/to say otherwise, at most 62 days. Never includes a meeting link.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"handle":   map[string]any{"type": "string", "description": "The host's buddy handle."},
				"timezone": timezoneProperty,
				"from":     map[string]any{"type": "string", "description": "Window start, RFC 3339. Default now."},
				"to":       map[string]any{"type": "string", "description": "Window end, RFC 3339. Default 30 days after from."},
			},
			"required":             []string{"handle"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Handle   string     `json:"handle"`
				Timezone string     `json:"timezone"`
				From     *time.Time `json:"from"`
				To       *time.Time `json:"to"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Handle == "" {
				return nil, fiber.NewError(fiber.StatusBadRequest, "handle is required")
			}
			from, to := slotWindowBounds(in.From, in.To)
			res, err := h.mocks.OpenSlots(ctx, userID, in.Handle, from, to, in.Timezone)
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			return map[string]any{"slots": slotResponses(res.Slots), "timezone": res.Zone, "topics": res.Topics}, nil
		},
	}
}

func (h *buddyHandlers) mockBookTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_book", Title: "Book a mock interview", Effect: assistant.EffectAdds,
		Description: "Book a 60-minute mock interview with a host, the person attending as guest. starts_at " +
			"must be one of the host's open slots exactly (RFC 3339) and topic one of the topics that host " +
			"offers. Returns the booked mock with the counterpart's handle and the meeting link. A guest may " +
			"hold at most 3 upcoming mocks.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"handle":    map[string]any{"type": "string", "description": "The host's buddy handle."},
				"starts_at": map[string]any{"type": "string", "description": "The slot's start, RFC 3339, exactly as listed."},
				"topic":     map[string]any{"type": "string", "description": "One of the host's topics."},
				"timezone":  timezoneProperty,
			},
			"required":             []string{"handle", "starts_at", "topic"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Handle string `json:"handle"`
				buddyBookMockBody
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Handle == "" {
				return nil, fiber.NewError(fiber.StatusBadRequest, "handle is required")
			}
			if in.StartsAt.IsZero() || in.Topic == "" {
				return nil, fiber.NewError(fiber.StatusBadRequest, "starts_at and topic are required")
			}
			m, err := h.mocks.Book(ctx, buddy.BookInput{
				HostHandle: in.Handle, GuestUserID: userID, StartsAt: in.StartsAt,
				Topic: in.Topic, GuestZone: in.Timezone,
			})
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			return toBuddyMockResponse(m.EntryFor(userID, time.Now())), nil
		},
	}
}

func (h *buddyHandlers) mockListTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_list", Title: "List my mock interviews", Effect: assistant.EffectReads,
		Description: "The person's own mock interviews, upcoming and past, as host or guest: id, role, the " +
			"counterpart's handle, times, topic, status, the meeting link while confirmed, whether it can " +
			"still be cancelled, and the reviews.",
		Schema: assistant.ClosedEmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			list, err := h.mocks.List(ctx, userID)
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			return map[string]any{
				"upcoming": toBuddyMockResponses(list.Upcoming),
				"past":     toBuddyMockResponses(list.Past),
			}, nil
		}),
	}
}

func (h *buddyHandlers) mockCancelTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_cancel", Title: "Cancel a mock interview", Effect: assistant.EffectOverwrites,
		Description: "Cancel one of the person's upcoming mock interviews, by id, before it starts. The other " +
			"participant is told.",
		Schema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"id": map[string]any{"type": "string", "description": "The mock interview's id."}},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				ID string `json:"id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			// As mockIDParam: an id that cannot exist and one that does not are the same answer.
			id, err := uuid.Parse(in.ID)
			if err != nil {
				return nil, fiber.NewError(fiber.StatusNotFound, "mock interview not found")
			}
			if err := h.mocks.Cancel(ctx, userID, id); err != nil {
				return nil, mockToolError(ctx, err)
			}
			return map[string]any{"cancelled": true}, nil
		},
	}
}

func toAvailabilityResponses(rules []slots.Rule) []availabilityRuleResponse {
	out := make([]availabilityRuleResponse, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toAvailabilityResponse(rule))
	}
	return out
}

func (h *buddyHandlers) mockScheduleGetTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_schedule_get", Title: "Show my mock hosting schedule", Effect: assistant.EffectReads,
		Description: "The person's own mock-interview hosting setup: settings (time zone, meeting link, " +
			"topics offered, paused) or null before they were first saved, and the schedule rules — weekly " +
			"rules by weekday (0 = Sunday) and dated overrides, each with the id needed to delete it.",
		Schema: assistant.ClosedEmptySchema,
		Run: assistant.NoArgs(func(ctx context.Context, userID int64) (any, error) {
			s, ok, err := h.mocks.Settings(ctx, userID)
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			rules, err := h.mocks.Availability(ctx, userID)
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			var settings *buddyMockSettingsBody
			if ok {
				body := toBuddyMockSettingsBody(s)
				settings = &body
			}
			return map[string]any{"settings": settings, "rules": toAvailabilityResponses(rules)}, nil
		}),
	}
}

func (h *buddyHandlers) mockSettingsSetTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_settings_set", Title: "Save my mock hosting settings", Effect: assistant.EffectOverwrites,
		Description: "Replace the person's mock-interview hosting settings as a whole: the time zone their " +
			"schedule is in, the meeting link guests receive, the topics they offer, and whether hosting is " +
			"paused. Returns the saved settings.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timezone":    map[string]any{"type": "string", "description": "IANA time zone the schedule's HH:MM times are in."},
				"meeting_url": map[string]any{"type": "string", "description": "The https link guests join the call at."},
				"topics":      map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": buddy.MockTopics}, "description": "Topics the person offers to interview on, at least one."},
				"paused":      map[string]any{"type": "boolean", "description": "True stops new bookings; existing mocks stay."},
			},
			"required":             []string{"timezone", "meeting_url", "topics", "paused"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			// The arguments are not validated against the schema, and a missing paused would
			// decode as false: a settings edit would silently resume a paused host.
			var in struct {
				buddyMockSettingsBody
				Paused *bool `json:"paused"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Paused == nil {
				return nil, fiber.NewError(fiber.StatusBadRequest, "paused is required")
			}
			s, err := h.mocks.SaveSettings(ctx, userID, buddy.MockSettings{
				Timezone: in.Timezone, MeetingURL: in.MeetingURL, Topics: in.Topics, Paused: *in.Paused,
			})
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			return toBuddyMockSettingsBody(s), nil
		},
	}
}

func (h *buddyHandlers) mockScheduleSetWeeklyTool() assistant.Tool {
	ruleProperties := map[string]any{
		"weekday": map[string]any{"type": "integer", "minimum": 0, "maximum": 6, "description": "0 = Sunday through 6 = Saturday."},
	}
	for k, v := range clockProperties {
		ruleProperties[k] = v
	}
	return assistant.Tool{
		Name: "buddy_mock_schedule_set_weekly", Title: "Replace my weekly mock schedule", Effect: assistant.EffectOverwrites,
		Description: "Replace the person's whole recurring weekly hosting schedule with these weekday rules " +
			"(an empty list clears it). Times are HH:MM in their settings' time zone. Dated overrides are " +
			"kept and are not accepted here.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rules": map[string]any{"type": "array", "items": map[string]any{
					"type": "object", "properties": ruleProperties, "required": []string{"weekday", "start", "end"},
				}},
			},
			"required":             []string{"rules"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in weeklyAvailabilityRequest
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			// A missing rules would decode as an empty week and wipe the schedule.
			if in.Rules == nil {
				return nil, fiber.NewError(fiber.StatusBadRequest, "rules is required; pass an empty list to clear the weekly schedule")
			}
			rules := make([]slots.Rule, 0, len(in.Rules))
			for _, r := range in.Rules {
				if r.Date != nil {
					return nil, fiber.NewError(fiber.StatusUnprocessableEntity,
						"the weekly schedule takes weekday rules; add a dated override for a date")
				}
				rule, err := r.toRule()
				if err != nil {
					return nil, err
				}
				rules = append(rules, rule)
			}
			if err := h.mocks.ReplaceWeekly(ctx, userID, rules); err != nil {
				return nil, mockToolError(ctx, err)
			}
			return map[string]any{"saved": true, "rules": len(rules)}, nil
		},
	}
}

func (h *buddyHandlers) mockScheduleAddOverrideTool() assistant.Tool {
	props := map[string]any{"date": map[string]any{"type": "string", "description": "The date, YYYY-MM-DD."}}
	for k, v := range clockProperties {
		props[k] = v
	}
	return assistant.Tool{
		Name: "buddy_mock_schedule_add_override", Title: "Add a dated mock schedule override", Effect: assistant.EffectAdds,
		Description: "Add one dated override to the person's hosting schedule: on that date they host " +
			"between start and end (HH:MM) instead of their weekly hours. An equal start and end closes the " +
			"date. Returns the stored rule with its id.",
		Schema: map[string]any{
			"type": "object", "properties": props,
			"required":             []string{"date", "start", "end"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in availabilityRuleRequest
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Date == nil {
				return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "an override needs a date")
			}
			rule, err := in.toRule()
			if err != nil {
				return nil, err
			}
			stored, err := h.mocks.AddOverride(ctx, userID, rule)
			if err != nil {
				return nil, mockToolError(ctx, err)
			}
			return toAvailabilityResponse(stored), nil
		},
	}
}

func (h *buddyHandlers) mockScheduleDeleteTool() assistant.Tool {
	return assistant.Tool{
		Name: "buddy_mock_schedule_delete", Title: "Delete a mock schedule rule", Effect: assistant.EffectOverwrites,
		Description: "Delete one rule (weekly or dated override) of the person's hosting schedule, by its id.",
		Schema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"rule_id": map[string]any{"type": "integer", "description": "The rule's id."}},
			"required":             []string{"rule_id"},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				RuleID int64 `json:"rule_id"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.RuleID <= 0 {
				return nil, fiber.NewError(fiber.StatusBadRequest, "rule_id is required")
			}
			if err := h.mocks.DeleteRule(ctx, userID, in.RuleID); err != nil {
				return nil, mockToolError(ctx, err)
			}
			return map[string]any{"deleted": true}, nil
		},
	}
}
