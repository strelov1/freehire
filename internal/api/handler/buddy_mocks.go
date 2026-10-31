package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// registerMocks mounts the buddy mock-interview routes (buddy.MockService). Cookie-only,
// like every buddy route, so a leaked API key cannot book or read a member's mocks.
func (h *buddyHandlers) registerMocks(api fiber.Router, mw middleware) {
	api.Get("/me/buddies/mocks", mw.cookie, h.ListMocks)
	api.Get("/me/buddies/mocks/hosts", mw.cookie, h.MockHosts)
	api.Get("/me/buddies/mocks/settings", mw.cookie, h.GetMockSettings)
	api.Put("/me/buddies/mocks/settings", mw.cookie, h.SaveMockSettings)
	api.Get("/me/buddies/mocks/availability", mw.cookie, h.GetMockAvailability)
	api.Put("/me/buddies/mocks/availability/weekly", mw.cookie, h.ReplaceMockWeekly)
	api.Post("/me/buddies/mocks/availability/overrides", mw.cookie, h.AddMockOverride)
	api.Delete("/me/buddies/mocks/availability/:id", mw.cookie, h.DeleteMockRule)
	api.Post("/me/buddies/mocks/:id/cancel", mw.cookie, h.CancelMock)
	api.Post("/me/buddies/mocks/:id/review", mw.cookie, h.ReviewMock)
	api.Get("/me/buddies/profiles/:handle/mock-slots", mw.cookie, h.MockSlots)
	api.Post("/me/buddies/profiles/:handle/mocks", mw.cookie, h.BookMock)
	api.Get("/me/buddies/profiles/:handle/mock-reviews", mw.cookie, h.MockReviews)
}

// mockError renders a mock sentinel as the status its declaration documents, and anything
// else as buddyError does.
func mockError(err error) error {
	switch {
	case errors.Is(err, buddy.ErrInvalidMockSettings), errors.Is(err, buddy.ErrInvalidTopic),
		errors.Is(err, buddy.ErrInvalidReview), errors.Is(err, buddy.ErrMockNotCancellable),
		errors.Is(err, buddy.ErrUnknownTopic),
		errors.Is(err, slots.ErrInvalidTimeOfDay), errors.Is(err, slots.ErrInvalidDate),
		errors.Is(err, slots.ErrInvalidWeekday), errors.Is(err, slots.ErrInvalidSpan):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, buddy.ErrSlotUnavailable):
		return fiber.NewError(fiber.StatusConflict, "that time is no longer available")
	case errors.Is(err, buddy.ErrAlreadyReviewed):
		return fiber.NewError(fiber.StatusConflict, "you already reviewed this mock interview")
	case errors.Is(err, buddy.ErrGuestMockLimit):
		return fiber.NewError(fiber.StatusTooManyRequests, "you already have 3 upcoming mock interviews as a guest")
	case errors.Is(err, buddy.ErrMockNotFound):
		return fiber.NewError(fiber.StatusNotFound, "mock interview not found")
	case errors.Is(err, slots.ErrNoOwnerZone):
		// A stored zone that no longer resolves is a broken row, not a bad request. Wrapped,
		// not a fiber.Error, so Sentry keeps the host and the zone; the caller gets RenderError's
		// generic 500.
		return fmt.Errorf("buddy mock: this member's schedule is misconfigured: %w", err)
	}
	return buddyError(err)
}

type buddyMockSettingsBody struct {
	Timezone   string   `json:"timezone"`
	MeetingURL string   `json:"meeting_url"`
	Topics     []string `json:"topics"`
	Paused     bool     `json:"paused"`
}

func toBuddyMockSettingsBody(s buddy.MockSettings) buddyMockSettingsBody {
	return buddyMockSettingsBody{Timezone: s.Timezone, MeetingURL: s.MeetingURL, Topics: s.Topics, Paused: s.Paused}
}

// GetMockSettings is the caller's own settings, or null before the first save.
func (h *buddyHandlers) GetMockSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	s, ok, err := h.mocks.Settings(c.Context(), userID)
	if err != nil {
		return mockError(err)
	}
	if !ok {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": toBuddyMockSettingsBody(s)})
}

// SaveMockSettings stores the caller's settings whole.
func (h *buddyHandlers) SaveMockSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in buddyMockSettingsBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	s, err := h.mocks.SaveSettings(c.Context(), userID, buddy.MockSettings{
		Timezone: in.Timezone, MeetingURL: in.MeetingURL, Topics: in.Topics, Paused: in.Paused,
	})
	if err != nil {
		return mockError(err)
	}
	return c.JSON(fiber.Map{"data": toBuddyMockSettingsBody(s)})
}

// GetMockAvailability is the caller's whole schedule, in the mentor schedule's wire shape.
func (h *buddyHandlers) GetMockAvailability(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rules, err := h.mocks.Availability(c.Context(), userID)
	if err != nil {
		return mockError(err)
	}
	out := make([]availabilityRuleResponse, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toAvailabilityResponse(rule))
	}
	return c.JSON(fiber.Map{"data": out})
}

// ReplaceMockWeekly swaps the whole recurring week, as the mentor schedule's PUT does.
func (h *buddyHandlers) ReplaceMockWeekly(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req weeklyAvailabilityRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	rules := make([]slots.Rule, 0, len(req.Rules))
	for _, raw := range req.Rules {
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
	if err := h.mocks.ReplaceWeekly(c.Context(), userID, rules); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// AddMockOverride adds one dated row and answers it with its id, so it can be removed.
func (h *buddyHandlers) AddMockOverride(c *fiber.Ctx) error {
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
	stored, err := h.mocks.AddOverride(c.Context(), userID, rule)
	if err != nil {
		return mockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toAvailabilityResponse(stored)})
}

// DeleteMockRule removes one row of the caller's own schedule.
func (h *buddyHandlers) DeleteMockRule(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.mocks.DeleteRule(c.Context(), userID, id); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyMockReviewResponse struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// buddyMockResponse is one mock as its participant sees it. It names the other side by
// handle only (null once they left or a block stands between them), and carries no
// account id and no address.
type buddyMockResponse struct {
	ID          string                   `json:"id"`
	Role        string                   `json:"role"`
	Counterpart *string                  `json:"counterpart"`
	StartsAt    time.Time                `json:"starts_at"`
	EndsAt      time.Time                `json:"ends_at"`
	Topic       string                   `json:"topic"`
	Status      string                   `json:"status"`
	MeetingURL  *string                  `json:"meeting_url"`
	CanCancel   bool                     `json:"can_cancel"`
	CanReview   bool                     `json:"can_review"`
	MyReview    *buddyMockReviewResponse `json:"my_review"`
	TheirReview *buddyMockReviewResponse `json:"their_review"`
}

func toBuddyMockResponse(e buddy.MockEntry) buddyMockResponse {
	out := buddyMockResponse{
		ID: e.ID.String(), Role: e.Role, StartsAt: e.StartsAt.UTC(), EndsAt: e.EndsAt.UTC(),
		Topic: e.Topic, Status: e.Status, CanCancel: e.CanCancel, CanReview: e.CanReview,
	}
	if e.Counterpart != "" {
		out.Counterpart = &e.Counterpart
	}
	if e.MeetingURL != "" {
		out.MeetingURL = &e.MeetingURL
	}
	if e.MyReview != nil {
		out.MyReview = &buddyMockReviewResponse{Rating: e.MyReview.Rating, Comment: e.MyReview.Comment}
	}
	if e.TheirReview != nil {
		out.TheirReview = &buddyMockReviewResponse{Rating: e.TheirReview.Rating, Comment: e.TheirReview.Comment}
	}
	return out
}

func toBuddyMockResponses(entries []buddy.MockEntry) []buddyMockResponse {
	out := make([]buddyMockResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toBuddyMockResponse(e))
	}
	return out
}

// ListMocks is the caller's own mocks, upcoming and past.
func (h *buddyHandlers) ListMocks(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	list, err := h.mocks.List(c.Context(), userID)
	if err != nil {
		return mockError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"upcoming": toBuddyMockResponses(list.Upcoming),
		"past":     toBuddyMockResponses(list.Past),
	}})
}

// MockSlots lists a member's open slots in the viewer's zone, with the topics they offer.
// The meeting link is never part of it.
func (h *buddyHandlers) MockSlots(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	from, to, err := slotWindow(c)
	if err != nil {
		return err
	}
	res, err := h.mocks.OpenSlots(c.Context(), userID, c.Params("handle"), from, to, c.Query("timezone"))
	if err != nil {
		return mockError(err)
	}
	out := slotResponses(res.Slots)
	return c.JSON(fiber.Map{
		"data": out,
		"meta": fiber.Map{"timezone": res.Zone, "count": len(out), "topics": res.Topics},
	})
}

// buddyMockHostResponse is one member who runs mocks, as the hosts listing shows them:
// the handle and card summary every buddy list shows, the topics they offer, their
// soonest open slot in the viewer's zone, and the member page's review aggregate. No
// meeting link, no account id.
type buddyMockHostResponse struct {
	Handle      string                   `json:"handle"`
	CardSummary buddyCardSummaryResponse `json:"card_summary"`
	Topics      []string                 `json:"topics"`
	NextSlot    buddyMockNextSlot        `json:"next_slot"`
	RatingAvg   float64                  `json:"rating_avg"`
	RatingCount int                      `json:"rating_count"`
	Completed   int                      `json:"completed"`
}

type buddyMockNextSlot struct {
	StartsAt   time.Time `json:"starts_at"`
	LocalStart string    `json:"local_start"`
	UTCOffset  string    `json:"utc_offset"`
}

// MockHosts lists the members the caller could book a mock with now, soonest first.
func (h *buddyHandlers) MockHosts(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	res, err := h.mocks.Hosts(c.Context(), userID, c.Query("topic"), c.Query("timezone"))
	if err != nil {
		return mockError(err)
	}
	out := make([]buddyMockHostResponse, 0, len(res.Hosts))
	for _, host := range res.Hosts {
		slot := slotResponses([]slots.Interval{host.NextSlot})[0]
		out = append(out, buddyMockHostResponse{
			Handle: host.Handle, CardSummary: toBuddyCardSummary(host.Card), Topics: host.Topics,
			NextSlot:  buddyMockNextSlot{StartsAt: slot.StartsAt, LocalStart: slot.LocalStart, UTCOffset: slot.UTCOffset},
			RatingAvg: host.Summary.RatingAvg, RatingCount: host.Summary.RatingCount, Completed: host.Summary.Completed,
		})
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"timezone": res.Zone, "count": len(out)}})
}

type buddyBookMockBody struct {
	StartsAt time.Time `json:"starts_at"`
	Topic    string    `json:"topic"`
	Timezone string    `json:"timezone"`
}

// BookMock takes one of a member's slots for the caller, as guest.
func (h *buddyHandlers) BookMock(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in buddyBookMockBody
	if err := c.BodyParser(&in); err != nil || in.StartsAt.IsZero() || in.Topic == "" {
		return fiber.NewError(fiber.StatusBadRequest, "starts_at and topic are required")
	}
	m, err := h.mocks.Book(c.Context(), buddy.BookInput{
		HostHandle: c.Params("handle"), GuestUserID: userID, StartsAt: in.StartsAt,
		Topic: in.Topic, GuestZone: in.Timezone,
	})
	if err != nil {
		return mockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toBuddyMockResponse(m.EntryFor(userID, time.Now()))})
}

// mockIDParam reads the mock id from the path. A malformed one is 404: an id that cannot
// exist and one that does not are the same answer.
func mockIDParam(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "mock interview not found")
	}
	return id, nil
}

// CancelMock cancels one of the caller's mocks before it starts.
func (h *buddyHandlers) CancelMock(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := mockIDParam(c)
	if err != nil {
		return err
	}
	if err := h.mocks.Cancel(c.Context(), userID, id); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyMockReviewBody struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// ReviewMock rates the other participant of one of the caller's mocks.
func (h *buddyHandlers) ReviewMock(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := mockIDParam(c)
	if err != nil {
		return err
	}
	var in buddyMockReviewBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if err := h.mocks.Review(c.Context(), userID, id, in.Rating, in.Comment); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyRevealedReviewResponse struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
	Topic   string `json:"topic"`
	Day     string `json:"day"`
}

type buddyMockSummaryResponse struct {
	Completed   int                           `json:"completed"`
	RatingCount int                           `json:"rating_count"`
	RatingAvg   float64                       `json:"rating_avg"`
	Recent      []buddyRevealedReviewResponse `json:"recent"`
}

// MockReviews is what a member page shows about that member's mocks.
func (h *buddyHandlers) MockReviews(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	sum, err := h.mocks.MemberSummary(c.Context(), userID, c.Params("handle"))
	if err != nil {
		return mockError(err)
	}
	out := buddyMockSummaryResponse{
		Completed: sum.Completed, RatingCount: sum.RatingCount, RatingAvg: sum.RatingAvg,
		Recent: make([]buddyRevealedReviewResponse, 0, len(sum.Recent)),
	}
	for _, r := range sum.Recent {
		out.Recent = append(out.Recent, buddyRevealedReviewResponse{Rating: r.Rating, Comment: r.Comment, Topic: r.Topic, Day: r.Day})
	}
	return c.JSON(fiber.Map{"data": out})
}
