package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/mentorship"
)

// availabilityRuleRequest is one row of a mentor's schedule, in whichever of the two
// shapes it holds. Exactly one of weekday and date is set — the domain type has no
// constructor that takes both, and the schema has a CHECK saying the same thing.
type availabilityRuleRequest struct {
	// Weekday is 0 (Sunday) through 6. Nil means this is a dated override.
	Weekday *int `json:"weekday"`
	// Date is YYYY-MM-DD. Nil means this is a weekly rule.
	Date *string `json:"date"`
	// Start and End are HH:MM. An END of "24:00" is how a mentor is available until
	// midnight without the row spilling onto the next date.
	Start string `json:"start"`
	End   string `json:"end"`
}

type availabilityRuleResponse struct {
	Weekday *int    `json:"weekday"`
	Date    *string `json:"date"`
	Start   string  `json:"start"`
	End     string  `json:"end"`
	// Closure marks the trick: a dated row with an equal start and end closes its date,
	// beating any other override on it. Without this flag a client would render "10:00 to
	// 10:00" and nobody would know what it meant.
	Closure bool `json:"closure"`
}

// toRule turns a request into the domain type, which is where the shape is enforced.
func (r availabilityRuleRequest) toRule() (mentorship.Rule, error) {
	start, err := parseClock(r.Start)
	if err != nil {
		return mentorship.Rule{}, err
	}
	end, err := parseClock(r.End)
	if err != nil {
		return mentorship.Rule{}, err
	}

	if r.Date != nil {
		parsed, parseErr := time.Parse(time.DateOnly, *r.Date)
		if parseErr != nil {
			return mentorship.Rule{}, fiber.NewError(fiber.StatusUnprocessableEntity,
				"date must be YYYY-MM-DD")
		}
		date, dateErr := mentorship.NewDate(parsed.Year(), parsed.Month(), parsed.Day())
		if dateErr != nil {
			return mentorship.Rule{}, mentorshipError(dateErr)
		}
		rule, ruleErr := mentorship.NewDatedRule(date, start, end)
		if ruleErr != nil {
			return mentorship.Rule{}, mentorshipError(ruleErr)
		}
		return rule, nil
	}

	if r.Weekday == nil {
		return mentorship.Rule{}, fiber.NewError(fiber.StatusUnprocessableEntity,
			"a rule needs either a weekday or a date")
	}
	rule, err := mentorship.NewWeeklyRule(time.Weekday(*r.Weekday), start, end)
	if err != nil {
		return mentorship.Rule{}, mentorshipError(err)
	}
	return rule, nil
}

func toAvailabilityResponse(rule mentorship.Rule) availabilityRuleResponse {
	out := availabilityRuleResponse{
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

// parseClock reads an HH:MM wall-clock time.
//
// Parsed by hand rather than with time.Parse("15:04", …), which cannot read "24:00" — and
// 24:00 is the whole point of TimeOfDay's upper bound: it is how a mentor says "until
// midnight" without the row spilling onto the next date. Strict about the shape, because
// fmt.Sscanf would happily accept "18:00nonsense".
func parseClock(raw string) (mentorship.TimeOfDay, error) {
	invalid := fiber.NewError(fiber.StatusUnprocessableEntity, "times must be HH:MM")

	hourPart, minutePart, ok := strings.Cut(raw, ":")
	if !ok || len(hourPart) != 2 || len(minutePart) != 2 {
		return mentorship.TimeOfDay{}, invalid
	}
	hour, err := strconv.Atoi(hourPart)
	if err != nil {
		return mentorship.TimeOfDay{}, invalid
	}
	minute, err := strconv.Atoi(minutePart)
	if err != nil {
		return mentorship.TimeOfDay{}, invalid
	}

	got, err := mentorship.NewTimeOfDay(hour, minute)
	if err != nil {
		return mentorship.TimeOfDay{}, mentorshipError(err)
	}
	return got, nil
}

// GetMyAvailability is the mentor's whole schedule, both shapes together — an override
// only means anything beside the weekly rules it replaces.
func (h *mentorshipHandlers) GetMyAvailability(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rules, err := h.mentorship.MyAvailability(c.Context(), userID)
	if err != nil {
		return mentorshipError(err)
	}

	out := make([]availabilityRuleResponse, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toAvailabilityResponse(rule))
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"count": len(out)}})
}

type weeklyAvailabilityRequest struct {
	Rules []availabilityRuleRequest `json:"rules"`
}

// ReplaceWeeklyAvailability swaps the whole recurring week. PUT and whole-week, not
// per-row: a schedule is a shape a mentor reasons about all at once, and the replacement
// is one transaction — a half-applied edit is a mentor bookable at hours they just removed.
// Dated overrides are untouched.
func (h *mentorshipHandlers) ReplaceWeeklyAvailability(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req weeklyAvailabilityRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	rules := make([]mentorship.Rule, 0, len(req.Rules))
	for _, raw := range req.Rules {
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

	if err := h.mentorship.ReplaceWeeklySchedule(c.Context(), userID, rules); err != nil {
		return mentorshipError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// AddAvailabilityOverride adds one dated row: a narrowed day, an extra day, or — with an
// equal start and end — a closure.
func (h *mentorshipHandlers) AddAvailabilityOverride(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req availabilityRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.Date == nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "an override needs a date")
	}
	rule, err := req.toRule()
	if err != nil {
		return err
	}

	if err := h.mentorship.AddOverride(c.Context(), userID, rule); err != nil {
		return mentorshipError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toAvailabilityResponse(rule)})
}

// DeleteAvailabilityRule removes one row of the caller's own schedule.
func (h *mentorshipHandlers) DeleteAvailabilityRule(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	ruleID, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "rule not found")
	}
	if err := h.mentorship.DeleteAvailabilityRule(c.Context(), userID, int64(ruleID)); err != nil {
		return mentorshipError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
