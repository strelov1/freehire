package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// registerMocks mounts the buddy mock-interview routes (buddy.MockService). Cookie-only,
// like every buddy route, so a leaked API key cannot book or read a member's mocks.
func (h *buddyHandlers) registerMocks(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/buddies/mocks", mw.cookie, contract)
	api.Get("/me/buddies/mocks/hosts", mw.cookie, contract)
	api.Get("/me/buddies/mocks/settings", mw.cookie, contract)
	api.Put("/me/buddies/mocks/settings", mw.cookie, contract)
	api.Get("/me/buddies/mocks/availability", mw.cookie, contract)
	api.Put("/me/buddies/mocks/availability/weekly", mw.cookie, contract)
	api.Post("/me/buddies/mocks/availability/overrides", mw.cookie, contract)
	api.Delete("/me/buddies/mocks/availability/:id", mw.cookie, contract)
	api.Post("/me/buddies/mocks/:id/cancel", mw.cookie, contract)
	api.Post("/me/buddies/mocks/:id/review", mw.cookie, contract)
	api.Get("/me/buddies/profiles/:handle/mock-slots", mw.cookie, contract)
	api.Post("/me/buddies/profiles/:handle/mocks", mw.cookie, contract)
	api.Get("/me/buddies/profiles/:handle/mock-reviews", mw.cookie, contract)
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
		// not a fiber.Error, so the report keeps the host and the zone; the caller gets RenderError's
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

func toBuddyMockHostResponses(res buddy.MockHosts) []buddyMockHostResponse {
	out := make([]buddyMockHostResponse, 0, len(res.Hosts))
	for _, host := range res.Hosts {
		slot := slotResponses([]slots.Interval{host.NextSlot})[0]
		out = append(out, buddyMockHostResponse{
			Handle: host.Handle, CardSummary: toBuddyCardSummary(host.Card), Topics: host.Topics,
			NextSlot:  buddyMockNextSlot{StartsAt: slot.StartsAt, LocalStart: slot.LocalStart, UTCOffset: slot.UTCOffset},
			RatingAvg: host.Summary.RatingAvg, RatingCount: host.Summary.RatingCount, Completed: host.Summary.Completed,
		})
	}
	return out
}

type buddyBookMockBody struct {
	StartsAt time.Time `json:"starts_at"`
	Topic    string    `json:"topic"`
	Timezone string    `json:"timezone"`
}

// The routes below implement the contract (typespec/buddies.tsp); the types above stay for
// the MCP tools, which the SDK validates against the Go type.

func buddyMockSettingsOut(s buddy.MockSettings) apigen.BuddyMockSettingsOut {
	return apigen.BuddyMockSettingsOut{Timezone: s.Timezone, MeetingURL: s.MeetingURL, Topics: buddyMockTopics(s.Topics), Paused: s.Paused}
}

// buddyMockTopics types the service's topic strings, which its own check keeps inside the vocabulary.
func buddyMockTopics(topics []string) []apigen.BuddyMockTopic {
	out := make([]apigen.BuddyMockTopic, len(topics))
	for i, t := range topics {
		out[i] = apigen.BuddyMockTopic(t)
	}
	return out
}

// BuddyMockSettingsGet is the caller's own settings, or null before the first save.
func (h *buddyHandlers) BuddyMockSettingsGet(ctx context.Context) (*apigen.BuddyMockSettingsGetOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	s, ok, err := h.mocks.Settings(ctx, userID)
	if err != nil {
		return nil, mockError(err)
	}
	if !ok {
		return &apigen.BuddyMockSettingsGetOK{Data: apigen.NilBuddyMockSettingsOut{Null: true}}, nil
	}
	return &apigen.BuddyMockSettingsGetOK{Data: apigen.NewNilBuddyMockSettingsOut(buddyMockSettingsOut(s))}, nil
}

// BuddyMockSettingsSave stores the caller's settings whole.
func (h *buddyHandlers) BuddyMockSettingsSave(ctx context.Context, req *apigen.BuddyMockSettings) (*apigen.BuddyMockSettingsSaveOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	s, err := h.mocks.SaveSettings(ctx, userID, buddy.MockSettings{
		Timezone: req.Timezone.Value, MeetingURL: req.MeetingURL.Value, Topics: req.Topics.Value, Paused: req.Paused.Value,
	})
	if err != nil {
		return nil, mockError(err)
	}
	return &apigen.BuddyMockSettingsSaveOK{Data: buddyMockSettingsOut(s)}, nil
}

func availabilityRuleOut(rule slots.Rule) apigen.BuddyMockRuleOut {
	r := toAvailabilityResponse(rule)
	out := apigen.BuddyMockRuleOut{ID: r.ID, Start: r.Start, End: r.End, Closure: r.Closure}
	if r.Weekday != nil {
		out.Weekday = apigen.NewNilInt64(int64(*r.Weekday))
	} else {
		out.Weekday.SetToNull()
	}
	out.Date = nilString(r.Date)
	return out
}

// ruleRequestIn is a contract schedule row as the mentor schedule's request type reads it.
func ruleRequestIn(r apigen.BuddyMockRule) availabilityRuleRequest {
	return availabilityRuleRequest{
		Weekday: intPtrOf(r.Weekday.Get), Date: ptrOf(r.Date.Get), Start: r.Start.Value, End: r.End.Value,
	}
}

// BuddyMockAvailabilityGet is the caller's whole schedule, in the mentor schedule's wire shape.
func (h *buddyHandlers) BuddyMockAvailabilityGet(ctx context.Context) (*apigen.BuddyMockAvailabilityGetOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := h.mocks.Availability(ctx, userID)
	if err != nil {
		return nil, mockError(err)
	}
	out := make([]apigen.BuddyMockRuleOut, 0, len(rules))
	for _, rule := range rules {
		out = append(out, availabilityRuleOut(rule))
	}
	return &apigen.BuddyMockAvailabilityGetOK{Data: out}, nil
}

// BuddyMockWeeklyReplace swaps the whole recurring week, as the mentor schedule's PUT does.
func (h *buddyHandlers) BuddyMockWeeklyReplace(ctx context.Context, req *apigen.BuddyMockWeekly) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	rules := make([]slots.Rule, 0, len(req.Rules))
	for _, raw := range req.Rules {
		in := ruleRequestIn(raw)
		if in.Date != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity,
				"the weekly schedule takes weekday rules; use the overrides endpoint for a date")
		}
		rule, err := in.toRule()
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	return mockError(h.mocks.ReplaceWeekly(ctx, userID, rules))
}

// BuddyMockOverrideAdd adds one dated row and answers it with its id, so it can be removed.
func (h *buddyHandlers) BuddyMockOverrideAdd(ctx context.Context, req *apigen.BuddyMockRule) (*apigen.BuddyMockOverrideAddCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := ruleRequestIn(*req)
	if in.Date == nil {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "an override needs a date")
	}
	rule, err := in.toRule()
	if err != nil {
		return nil, err
	}
	stored, err := h.mocks.AddOverride(ctx, userID, rule)
	if err != nil {
		return nil, mockError(err)
	}
	return &apigen.BuddyMockOverrideAddCreated{Data: availabilityRuleOut(stored)}, nil
}

// BuddyMockRuleDelete removes one row of the caller's own schedule.
func (h *buddyHandlers) BuddyMockRuleDelete(ctx context.Context, params apigen.BuddyMockRuleDeleteParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	return mockError(h.mocks.DeleteRule(ctx, userID, id))
}

func buddyMockReviewOut(r *buddy.MockReview) apigen.NilBuddyMockReview {
	if r == nil {
		return apigen.NilBuddyMockReview{Null: true}
	}
	return apigen.NewNilBuddyMockReview(apigen.BuddyMockReview{Rating: int64(r.Rating), Comment: r.Comment})
}

func buddyMockOut(e buddy.MockEntry) apigen.BuddyMock {
	return apigen.BuddyMock{
		ID: e.ID.String(), Role: apigen.BuddyMockRole(e.Role), StartsAt: e.StartsAt.UTC(), EndsAt: e.EndsAt.UTC(),
		Topic: apigen.BuddyMockTopic(e.Topic), Status: apigen.BuddyMockStatus(e.Status), CanCancel: e.CanCancel, CanReview: e.CanReview,
		Counterpart: nilString(nilIfEmptyPtr(e.Counterpart)), MeetingURL: nilString(nilIfEmptyPtr(e.MeetingURL)),
		MyReview: buddyMockReviewOut(e.MyReview), TheirReview: buddyMockReviewOut(e.TheirReview),
	}
}

func buddyMocksOut(entries []buddy.MockEntry) []apigen.BuddyMock {
	out := make([]apigen.BuddyMock, 0, len(entries))
	for _, e := range entries {
		out = append(out, buddyMockOut(e))
	}
	return out
}

func nilIfEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// BuddyMockList is the caller's own mocks, upcoming and past.
func (h *buddyHandlers) BuddyMockList(ctx context.Context) (*apigen.BuddyMockListOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	list, err := h.mocks.List(ctx, userID)
	if err != nil {
		return nil, mockError(err)
	}
	return &apigen.BuddyMockListOK{Data: apigen.BuddyMockList{
		Upcoming: buddyMocksOut(list.Upcoming), Past: buddyMocksOut(list.Past),
	}}, nil
}

// BuddyMockSlots lists a member's open slots in the viewer's zone, with the topics they offer.
// The meeting link is never part of it.
func (h *buddyHandlers) BuddyMockSlots(ctx context.Context, params apigen.BuddyMockSlotsParams) (*apigen.BuddyMockSlotsPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	from, to, err := slotWindowOf(params.From.Value, params.To.Value)
	if err != nil {
		return nil, err
	}
	res, err := h.mocks.OpenSlots(ctx, userID, params.Handle, from, to, params.Timezone.Value)
	if err != nil {
		return nil, mockError(err)
	}
	out := make([]apigen.BuddyMockSlot, 0, len(res.Slots))
	for _, s := range slotResponses(res.Slots) {
		out = append(out, apigen.BuddyMockSlot{
			StartsAt: s.StartsAt, EndsAt: s.EndsAt, LocalStart: s.LocalStart, LocalEnd: s.LocalEnd, UtcOffset: s.UTCOffset,
		})
	}
	return &apigen.BuddyMockSlotsPage{
		Data: out,
		Meta: apigen.BuddyMockSlotsMeta{Timezone: res.Zone, Count: int64(len(out)), Topics: buddyMockTopics(res.Topics)},
	}, nil
}

// BuddyMockHosts lists the members the caller could book a mock with now, soonest first.
func (h *buddyHandlers) BuddyMockHosts(ctx context.Context, params apigen.BuddyMockHostsParams) (*apigen.BuddyMockHostsPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	res, err := h.mocks.Hosts(ctx, userID, params.Topic.Value, params.Timezone.Value)
	if err != nil {
		return nil, mockError(err)
	}
	out := make([]apigen.BuddyMockHost, 0, len(res.Hosts))
	for _, host := range res.Hosts {
		slot := slotResponses([]slots.Interval{host.NextSlot})[0]
		out = append(out, apigen.BuddyMockHost{
			Handle: host.Handle, CardSummary: buddyCardSummaryOut(host.Card), Topics: buddyMockTopics(host.Topics),
			NextSlot:  apigen.BuddyMockNextSlot{StartsAt: slot.StartsAt, LocalStart: slot.LocalStart, UtcOffset: slot.UTCOffset},
			RatingAvg: host.Summary.RatingAvg, RatingCount: int64(host.Summary.RatingCount), Completed: int64(host.Summary.Completed),
		})
	}
	return &apigen.BuddyMockHostsPage{Data: out, Meta: apigen.BuddyMockHostsMeta{Timezone: res.Zone, Count: int64(len(out))}}, nil
}

// BuddyMockBook takes one of a member's slots for the caller, as guest.
func (h *buddyHandlers) BuddyMockBook(ctx context.Context, req *apigen.BuddyMockBook, params apigen.BuddyMockBookParams) (*apigen.BuddyMockBookCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if req.StartsAt.Value.IsZero() || req.Topic.Value == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "starts_at and topic are required")
	}
	m, err := h.mocks.Book(ctx, buddy.BookInput{
		HostHandle: params.Handle, GuestUserID: userID, StartsAt: req.StartsAt.Value,
		Topic: req.Topic.Value, GuestZone: req.Timezone.Value,
	})
	if err != nil {
		return nil, mockError(err)
	}
	return &apigen.BuddyMockBookCreated{Data: buddyMockOut(m.EntryFor(userID, time.Now()))}, nil
}

// mockIDOf reads the mock id from the path. A malformed one is 404: an id that cannot
// exist and one that does not are the same answer.
func mockIDOf(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "mock interview not found")
	}
	return id, nil
}

// BuddyMockCancel cancels one of the caller's mocks before it starts.
func (h *buddyHandlers) BuddyMockCancel(ctx context.Context, params apigen.BuddyMockCancelParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := mockIDOf(params.ID)
	if err != nil {
		return err
	}
	return mockError(h.mocks.Cancel(ctx, userID, id))
}

// BuddyMockReview rates the other participant of one of the caller's mocks.
func (h *buddyHandlers) BuddyMockReview(ctx context.Context, req *apigen.BuddyMockReviewBody, params apigen.BuddyMockReviewParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := mockIDOf(params.ID)
	if err != nil {
		return err
	}
	return mockError(h.mocks.Review(ctx, userID, id, int(req.Rating.Value), req.Comment.Value))
}

// BuddyMockReviews is what a member page shows about that member's mocks.
func (h *buddyHandlers) BuddyMockReviews(ctx context.Context, params apigen.BuddyMockReviewsParams) (*apigen.BuddyMockReviewsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	sum, err := h.mocks.MemberSummary(ctx, userID, params.Handle)
	if err != nil {
		return nil, mockError(err)
	}
	out := apigen.BuddyMockSummary{
		Completed: int64(sum.Completed), RatingCount: int64(sum.RatingCount), RatingAvg: sum.RatingAvg,
		Recent: make([]apigen.BuddyRevealedReview, 0, len(sum.Recent)),
	}
	for _, r := range sum.Recent {
		out.Recent = append(out.Recent, apigen.BuddyRevealedReview{Rating: int64(r.Rating), Comment: r.Comment, Topic: apigen.BuddyMockTopic(r.Topic), Day: r.Day})
	}
	return &apigen.BuddyMockReviewsOK{Data: out}, nil
}
