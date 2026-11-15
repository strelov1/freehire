package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// availabilityRuleRequest is one row of a mentor's schedule, in either of two shapes.
// Exactly one of weekday and date is set (the domain type and a schema CHECK enforce it).
type availabilityRuleRequest struct {
	// Weekday is 0 (Sunday) through 6. Nil means this is a dated override.
	Weekday *int `json:"weekday"`
	// Date is YYYY-MM-DD. Nil means this is a weekly rule.
	Date *string `json:"date"`
	// Start and End are HH:MM. An END of "24:00" means available until midnight without the
	// row spilling onto the next date.
	Start string `json:"start"`
	End   string `json:"end"`
}

type availabilityRuleResponse struct {
	// ID is what the delete route needs; a rule rendered without one can never be removed.
	ID      int64   `json:"id"`
	Weekday *int    `json:"weekday"`
	Date    *string `json:"date"`
	Start   string  `json:"start"`
	End     string  `json:"end"`
	// Closure marks a dated row with equal start and end, which closes its date and beats any
	// other override; otherwise a client would render "10:00 to 10:00" with no meaning.
	Closure bool `json:"closure"`
}

// toRule turns a request into the domain type, which is where the shape is enforced.
func (r availabilityRuleRequest) toRule() (slots.Rule, error) {
	start, err := parseClock(r.Start)
	if err != nil {
		return slots.Rule{}, err
	}
	end, err := parseClock(r.End)
	if err != nil {
		return slots.Rule{}, err
	}

	if r.Date != nil {
		parsed, parseErr := time.Parse(time.DateOnly, *r.Date)
		if parseErr != nil {
			return slots.Rule{}, fiber.NewError(fiber.StatusUnprocessableEntity,
				"date must be YYYY-MM-DD")
		}
		date, dateErr := slots.NewDate(parsed.Year(), parsed.Month(), parsed.Day())
		if dateErr != nil {
			return slots.Rule{}, mentorshipError(dateErr)
		}
		rule, ruleErr := slots.NewDatedRule(date, start, end)
		if ruleErr != nil {
			return slots.Rule{}, mentorshipError(ruleErr)
		}
		return rule, nil
	}

	if r.Weekday == nil {
		return slots.Rule{}, fiber.NewError(fiber.StatusUnprocessableEntity,
			"a rule needs either a weekday or a date")
	}
	rule, err := slots.NewWeeklyRule(time.Weekday(*r.Weekday), start, end)
	if err != nil {
		return slots.Rule{}, mentorshipError(err)
	}
	return rule, nil
}

func toAvailabilityResponse(rule slots.Rule) availabilityRuleResponse {
	out := availabilityRuleResponse{
		ID:      rule.ID(),
		Start:   rule.Start().String(),
		End:     rule.End().String(),
		Closure: rule.IsClosure(),
	}
	if rule.IsDated() {
		date := rule.Date().String()
		out.Date = &date
		return out
	}
	weekday := int(rule.Weekday())
	out.Weekday = &weekday
	return out
}

// parseClock reads an HH:MM wall-clock time. Parsed by hand because time.Parse("15:04")
// cannot read "24:00", TimeOfDay's upper bound ("until midnight"); strict about the shape
// because fmt.Sscanf would accept "18:00nonsense".
func parseClock(raw string) (slots.TimeOfDay, error) {
	invalid := fiber.NewError(fiber.StatusUnprocessableEntity, "times must be HH:MM")

	hourPart, minutePart, ok := strings.Cut(raw, ":")
	if !ok || len(hourPart) != 2 || len(minutePart) != 2 {
		return slots.TimeOfDay{}, invalid
	}
	hour, err := strconv.Atoi(hourPart)
	if err != nil {
		return slots.TimeOfDay{}, invalid
	}
	minute, err := strconv.Atoi(minutePart)
	if err != nil {
		return slots.TimeOfDay{}, invalid
	}

	got, err := slots.NewTimeOfDay(hour, minute)
	if err != nil {
		return slots.TimeOfDay{}, mentorshipError(err)
	}
	return got, nil
}

type weeklyAvailabilityRequest struct {
	Rules []availabilityRuleRequest `json:"rules"`
}

// GetMyAvailability is the mentor's whole schedule, both shapes together: an override only
// means something beside the weekly rules it replaces.
func (h *mentorshipHandlers) GetMyAvailability(ctx context.Context) (*apigen.MentorshipRulesPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := h.mentorship.MyAvailability(ctx, userID)
	if err != nil {
		return nil, mentorshipError(err)
	}

	out := make([]apigen.BuddyMockRuleOut, 0, len(rules))
	for _, rule := range rules {
		out = append(out, availabilityRuleOut(rule))
	}
	return &apigen.MentorshipRulesPage{Data: out, Meta: apigen.MentorshipCountMeta{Count: int64(len(out))}}, nil
}

// ReplaceWeeklyAvailability swaps the whole recurring week in one transaction (PUT, not
// per-row), so a half-applied edit never leaves a mentor bookable at removed hours. Dated
// overrides are untouched.
func (h *mentorshipHandlers) ReplaceWeeklyAvailability(ctx context.Context, req *apigen.BuddyMockWeekly) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	rules := make([]slots.Rule, 0, len(req.Rules))
	for _, item := range req.Rules {
		raw := ruleRequestIn(item)
		if raw.Date != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity,
				"the weekly schedule takes weekday rules; use the overrides endpoint for a date")
		}
		rule, ruleErr := raw.toRule()
		if ruleErr != nil {
			return ruleErr
		}
		rules = append(rules, rule)
	}

	return mentorshipError(h.mentorship.ReplaceWeeklySchedule(ctx, userID, rules))
}

// AddAvailabilityOverride adds one dated row: a narrowed day, an extra day, or (with equal
// start and end) a closure.
func (h *mentorshipHandlers) AddAvailabilityOverride(ctx context.Context, body *apigen.BuddyMockRule) (*apigen.AddAvailabilityOverrideCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	req := ruleRequestIn(*body)
	if req.Date == nil {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "an override needs a date")
	}
	rule, err := req.toRule()
	if err != nil {
		return nil, err
	}

	if err := h.mentorship.AddOverride(ctx, userID, rule); err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.AddAvailabilityOverrideCreated{Data: availabilityRuleOut(rule)}, nil
}

// GetMyCalendar is the owner's resolved calendar for one month (booked, busy, free and
// closed time), computed like the public slot engine decides what a seeker may book, but
// authenticated and in the mentor's own timezone. Each interval's status is "booked",
// "busy", "free" or "closed".
func (h *mentorshipHandlers) GetMyCalendar(ctx context.Context, params apigen.GetMyCalendarParams) (*apigen.MentorshipCalendarPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	year, month, err := calendarMonth(params.Month.Value)
	if err != nil {
		return nil, err
	}

	result, err := h.mentorship.MyCalendar(ctx, userID, year, month)
	if err != nil {
		return nil, mentorshipError(err)
	}

	out := make([]apigen.MentorshipCalendarInterval, 0, len(result.Intervals))
	for _, iv := range result.Intervals {
		out = append(out, apigen.MentorshipCalendarInterval{
			StartsAt: iv.Start.UTC(),
			EndsAt:   iv.End.UTC(),
			Status:   apigen.MentorshipCalendarStatus(iv.Status),
		})
	}
	return &apigen.MentorshipCalendarPage{
		Data: out,
		Meta: apigen.MentorshipTimezoneMeta{Timezone: result.Zone, Count: int64(len(out))},
	}, nil
}

// calendarMonth parses "?month=YYYY-MM", defaulting to the current UTC month. UTC is an
// approximation of the mentor's own month, since resolving it needs the profile read this
// runs before; a day's error at a month edge is cosmetic and an explicit month fixes it.
func calendarMonth(raw string) (year int, month time.Month, err error) {
	if raw == "" {
		now := time.Now().UTC()
		return now.Year(), now.Month(), nil
	}
	parsed, parseErr := time.Parse("2006-01", raw)
	if parseErr != nil {
		return 0, 0, fiber.NewError(fiber.StatusUnprocessableEntity, "month must be YYYY-MM")
	}
	return parsed.Year(), parsed.Month(), nil
}

// DeleteAvailabilityRule removes one row of the caller's own schedule.
func (h *mentorshipHandlers) DeleteAvailabilityRule(ctx context.Context, params apigen.DeleteAvailabilityRuleParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	ruleID, err := strconv.Atoi(params.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "rule not found")
	}
	return mentorshipError(h.mentorship.DeleteAvailabilityRule(ctx, userID, int64(ruleID)))
}
