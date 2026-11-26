package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/availability"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// availabilityHandlers serve the user's one availability calendar, the same rows the
// mentorship and buddy schedule routes read and write.
type availabilityHandlers struct {
	store *availability.Store
	plans interface {
		RequirePaid(ctx context.Context, userID int64) (plan.Tier, error)
	}
}

func (h *availabilityHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/availability", mw.key, contract)
	api.Put("/me/availability/weekly", mw.key, contract)
	api.Post("/me/availability/overrides", mw.key, contract)
	api.Put("/me/availability/booking", mw.key, contract)
	api.Delete("/me/availability/:id", mw.key, contract)
}

func (h *availabilityHandlers) GetAvailability(ctx context.Context) (*apigen.GetAvailabilityOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	sc, err := h.store.Load(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := apigen.AvailabilityOut{
		Rules:    make([]apigen.BuddyMockRuleOut, 0, len(sc.Rules)),
		Timezone: apigen.NilString{Null: true},
		Saved:    sc.Settings != nil || len(sc.Rules) > 0,
		Booking: apigen.AvailabilityBookingOut{
			DurationMin: int64(availability.DefaultBookingDuration / time.Minute),
			NoticeHours: int64(availability.DefaultBookingNotice / time.Hour),
		},
	}
	for _, rule := range sc.Rules {
		out.Rules = append(out.Rules, availabilityRuleOut(rule))
	}
	if sc.Settings != nil {
		out.Timezone = apigen.NewNilString(sc.Zone)
		out.Booking = bookingSettingsOut(*sc.Settings)
	}
	return &apigen.GetAvailabilityOK{Data: out}, nil
}

// setZone stores the optional zone the hours are stated in; 422 when it is not an IANA place.
func (h *availabilityHandlers) setZone(ctx context.Context, userID int64, zone apigen.OptString) error {
	if !zone.Set {
		return nil
	}
	if !availability.ValidZone(zone.Value) {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "timezone must be an IANA zone name")
	}
	return h.store.SetZone(ctx, userID, zone.Value)
}

func bookingSettingsOut(s availability.Settings) apigen.AvailabilityBookingOut {
	return apigen.AvailabilityBookingOut{
		DurationMin: int64(s.Duration / time.Minute), NoticeHours: int64(s.Notice / time.Hour),
	}
}

func (h *availabilityHandlers) ReplaceAvailabilityWeekly(ctx context.Context, req *apigen.AvailabilityWeeklyBody) error {
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
		rule, err := raw.toRule()
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	if err := h.setZone(ctx, userID, req.Timezone); err != nil {
		return err
	}
	return h.store.ReplaceWeekly(ctx, userID, rules)
}

func (h *availabilityHandlers) AddAvailabilityDated(ctx context.Context, body *apigen.AvailabilityRuleBody) (*apigen.AddAvailabilityDatedCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	req := ruleRequestIn(apigen.BuddyMockRule{Weekday: body.Weekday, Date: body.Date, Start: body.Start, End: body.End})
	if req.Date == nil {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "an override needs a date")
	}
	rule, err := req.toRule()
	if err != nil {
		return nil, err
	}
	if err := h.setZone(ctx, userID, body.Timezone); err != nil {
		return nil, err
	}
	rule, err = h.store.Add(ctx, userID, rule)
	if err != nil {
		return nil, err
	}
	return &apigen.AddAvailabilityDatedCreated{Data: availabilityRuleOut(rule)}, nil
}

func (h *availabilityHandlers) DeleteAvailabilityEntry(ctx context.Context, params apigen.DeleteAvailabilityEntryParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(params.ID, 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "rule not found")
	}
	ok, err := h.store.Delete(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "rule not found")
	}
	return nil
}

func (h *availabilityHandlers) SaveAvailabilityBooking(ctx context.Context, req *apigen.AvailabilityBookingBody) (*apigen.SaveAvailabilityBookingOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.plans.RequirePaid(ctx, userID); err != nil {
		if errors.Is(err, plan.ErrPaymentRequired) {
			return nil, fiber.NewError(fiber.StatusPaymentRequired, "Booking settings need Pro or Ultra")
		}
		return nil, err
	}
	if !availability.ValidZone(req.Timezone) {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "timezone must be an IANA zone name")
	}
	// Bounds first, so a huge int cannot overflow the Duration before it is checked.
	if req.NoticeHours < 0 || req.NoticeHours > int64(availability.MaxBookingNotice/time.Hour) || req.DurationMin < 0 || req.DurationMin > 60 {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, availability.ErrInvalidSettings.Error())
	}
	st := availability.Settings{
		Duration: time.Duration(req.DurationMin) * time.Minute,
		Notice:   time.Duration(req.NoticeHours) * time.Hour,
	}
	if err := h.store.SaveSettings(ctx, userID, req.Timezone, st); err != nil {
		if errors.Is(err, availability.ErrInvalidSettings) {
			return nil, fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		return nil, err
	}
	return &apigen.SaveAvailabilityBookingOK{Data: bookingSettingsOut(st)}, nil
}
