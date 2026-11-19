package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// bookingLinkHandlers serves the candidate's booking link: turning it on (the owner, by
// cookie) and the public pair a recruiter uses with no account. The public answers never
// name the candidate, their address, the application or other bookings.
type bookingLinkHandlers struct {
	links  *bookinglink.Service
	origin string
}

func newBookingLinkHandlers(links *bookinglink.Service, frontendOrigin string) *bookingLinkHandlers {
	return &bookingLinkHandlers{links: links, origin: strings.TrimRight(frontendOrigin, "/")}
}

func (h *bookingLinkHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Post("/me/applications/:id/booking-link", mw.cookie, contract)
	// Anonymous; the gateway rate-limits /api/v1/booking/* per address.
	api.Get("/booking/:token/slots", contract)
	api.Post("/booking/:token/bookings", refuseEncodedBodyMW, contract)
}

// refuseEncodedBodyMW is refuseEncodedBody as route middleware, for an anonymous contract
// route whose body the generated server would otherwise decompress unbounded.
func refuseEncodedBodyMW(c *fiber.Ctx) error {
	if err := refuseEncodedBody(c); err != nil {
		return err
	}
	return c.Next()
}

// bookingURL is the public page for a token.
func (h *bookingLinkHandlers) bookingURL(token string) string {
	return h.origin + "/booking/" + token
}

func (h *bookingLinkHandlers) EnableBookingLink(ctx context.Context, req *apigen.BookingLinkEnableBody, params apigen.EnableBookingLinkParams) (*apigen.EnableBookingLinkCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	token, err := h.links.Enable(ctx, userID, params.ID, req.Timezone)
	if err != nil {
		return nil, bookingLinkError(err)
	}
	return &apigen.EnableBookingLinkCreated{Data: apigen.BookingLinkOut{URL: h.bookingURL(token)}}, nil
}

func (h *bookingLinkHandlers) GetBookingLinkSlots(ctx context.Context, params apigen.GetBookingLinkSlotsParams) (*apigen.MentorshipSlotsPage, error) {
	result, err := h.links.Slots(ctx, params.Token, params.Tz.Value)
	if err != nil {
		return nil, bookingLinkError(err)
	}
	out := make([]apigen.BuddyMockSlot, 0, len(result.Slots))
	for _, s := range slotResponses(result.Slots) {
		out = append(out, apigen.BuddyMockSlot{
			StartsAt: s.StartsAt, EndsAt: s.EndsAt, LocalStart: s.LocalStart, LocalEnd: s.LocalEnd, UtcOffset: s.UTCOffset,
		})
	}
	return &apigen.MentorshipSlotsPage{
		Data: out,
		Meta: apigen.MentorshipTimezoneMeta{Timezone: result.Zone, Count: int64(len(out))},
	}, nil
}

func (h *bookingLinkHandlers) BookBookingLinkSlot(ctx context.Context, req *apigen.BookingLinkBookBody, params apigen.BookBookingLinkSlotParams) (*apigen.BookBookingLinkSlotCreated, error) {
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "starts_at must be an RFC 3339 timestamp")
	}
	b, err := h.links.Book(ctx, bookinglink.BookInput{
		Token: params.Token, StartsAt: startsAt, Name: req.Name, Email: req.Email,
	})
	if err != nil {
		return nil, bookingLinkError(err)
	}
	return &apigen.BookBookingLinkSlotCreated{Data: apigen.BookingLinkBooking{StartsAt: b.StartsAt, EndsAt: b.EndsAt}}, nil
}

// bookingLinkError maps the bookinglink sentinels onto HTTP.
func bookingLinkError(err error) error {
	switch {
	case errors.Is(err, plan.ErrPaymentRequired):
		return fiber.NewError(fiber.StatusPaymentRequired, "A booking link needs Pro or Ultra")
	case errors.Is(err, bookinglink.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, bookinglink.ErrSlotUnavailable):
		return &codedError{status: fiber.StatusConflict, code: "slot_unavailable", message: "this time is no longer available"}
	case errors.Is(err, bookinglink.ErrInvalidBooking):
		return fiber.NewError(fiber.StatusBadRequest, "a name and a valid email are required")
	case errors.Is(err, bookinglink.ErrInvalidZone):
		return fiber.NewError(fiber.StatusBadRequest, "timezone must be an IANA zone name")
	case errors.Is(err, slots.ErrNoOwnerZone):
		return fiber.NewError(fiber.StatusInternalServerError, "this booking link is misconfigured")
	default:
		return err
	}
}
