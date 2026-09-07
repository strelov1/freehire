package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/engage/mentorship"
)

// mentorshipHandlers serves the mentorship marketplace: the public directory and its
// slots, the seeker's bookings, the mentor's own cabinet, and the moderation queue.
type mentorshipHandlers struct {
	mentorship *mentorship.Service
}

func newMentorshipHandlers(svc *mentorship.Service) *mentorshipHandlers {
	return &mentorshipHandlers{mentorship: svc}
}

// slotReadsPerMinute is this endpoint's own budget, separate from the shared public-read
// one.
//
// Slots are COMPUTED per request, on a host where most traffic is crawlers — so this is
// the one public endpoint here that costs real work per hit, and it must not be able to
// exhaust the allowance the rest of the site reads on. The figure is deliberately below
// publicReadsPerMinute: a human comparing a mentor's September and October needs a handful
// of requests a minute, and anything past that is a robot walking the calendar.
const slotReadsPerMinute = 20

func (h *mentorshipHandlers) register(api fiber.Router, mw middleware) {
	h.registerPublic(api, mw)

	// The seeker's side.
	api.Post("/mentors/:slug/bookings", mw.key, h.BookSession)
	api.Get("/me/mentorship/sessions", mw.key, h.ListMySessions)
	api.Post("/me/mentorship/sessions/:id/cancel", mw.key, h.CancelSession)
	api.Put("/me/mentorship/sessions/:id/review", mw.key, h.ReviewSession)

	// The mentor's own cabinet: their profile, their schedule, their bookings.
	api.Get("/me/mentorship/profile", mw.key, h.GetMyMentorProfile)
	api.Post("/me/mentorship/profile", mw.key, h.SubmitMentorProfile)
	api.Put("/me/mentorship/profile", mw.key, h.UpdateMentorProfile)
	api.Delete("/me/mentorship/profile", mw.key, h.WithdrawMentorProfile)
	api.Post("/me/mentorship/profile/pause", mw.key, h.PauseMentorProfile)
	api.Get("/me/mentorship/availability", mw.key, h.GetMyAvailability)
	api.Put("/me/mentorship/availability/weekly", mw.key, h.ReplaceWeeklyAvailability)
	api.Post("/me/mentorship/availability/overrides", mw.key, h.AddAvailabilityOverride)
	api.Delete("/me/mentorship/availability/:id", mw.key, h.DeleteAvailabilityRule)
	api.Get("/me/mentorship/bookings", mw.key, h.ListMentorBookings)

	// Moderation, behind the same gate the referral queue uses.
	api.Get("/mentorship/profiles", mw.key, mw.moderator, h.ListPendingMentorProfiles)
	api.Post("/mentorship/profiles/:id/decide", mw.key, mw.moderator, h.DecideMentorProfile)
}

// registerPublic mounts the anonymous surface, separately from the rest.
//
// Split out rather than mixed in, because the public-read-limiter guard drives every GET a
// register mounts and requires the limiter to be the FIRST handler on each chain. That is
// the right rule and this feature's cabinet routes cannot satisfy it — they sit behind
// auth, which must run first. Keeping the two in one register would have meant either
// weakening the guard or throttling the cabinet by IP, and both are worse than a second
// method.
//
// Anyone may browse mentors and see when they are free. Booking is not public, and that
// split is the product: a directory behind a login sells nothing.
func (h *mentorshipHandlers) registerPublic(api fiber.Router, mw middleware) {
	readLimit := publicReadLimiter(mw.throttler)
	slotLimit := ratelimit.Middleware(mw.throttler,
		ratelimit.KeyByIP("mentorslots"), slotReadsPerMinute, time.Minute)

	api.Get("/mentors", readLimit, h.ListMentors)
	api.Get("/mentors/:slug", readLimit, h.GetMentor)
	api.Get("/mentors/:slug/slots", slotLimit, h.GetMentorSlots)
}

// mentorResponse is the public shape of a mentor. user_id is not in it: it is ownership,
// internal, and nothing public needs it.
type mentorResponse struct {
	Slug        string   `json:"slug"`
	CompanySlug string   `json:"company_slug"`
	CompanyName string   `json:"company_name"`
	Headline    string   `json:"headline"`
	Bio         string   `json:"bio"`
	Topics      []string `json:"topics"`
	Languages   []string `json:"languages"`
	Timezone    string   `json:"timezone"`
	// SessionMinutes is what a seeker is booking, so it belongs on the card as well as the
	// profile: an hour and twenty minutes are different offers.
	SessionMinutes int     `json:"session_minutes"`
	RatingCount    int64   `json:"rating_count"`
	RatingAvg      float64 `json:"rating_avg"`
	Status         string  `json:"status,omitempty"`
	Paused         bool    `json:"paused,omitempty"`
	MeetingURL     string  `json:"meeting_url,omitempty"`
}

// toMentorResponse renders a profile for the PUBLIC. The meeting link is deliberately
// absent: it is a live room, and publishing it lets anybody who reads the directory walk
// into somebody else's session. A booked seeker gets it on their booking.
func toMentorResponse(p mentorship.Profile) mentorResponse {
	return mentorResponse{
		Slug: p.Slug, CompanySlug: p.CompanySlug, CompanyName: p.CompanyName,
		Headline: p.Headline, Bio: p.Bio, Topics: p.Topics, Languages: p.Languages,
		Timezone:       p.Timezone,
		SessionMinutes: int(p.Session.Duration / time.Minute),
		RatingCount:    p.RatingCount, RatingAvg: p.RatingAvg,
	}
}

// toOwnMentorResponse is the same profile as its OWNER sees it: with the moderation
// status, the pause switch and the meeting link they configured.
func toOwnMentorResponse(p mentorship.Profile) mentorResponse {
	out := toMentorResponse(p)
	out.Status = p.Status
	out.Paused = p.Paused
	out.MeetingURL = p.MeetingURL
	return out
}

// ListMentors serves the public directory.
func (h *mentorshipHandlers) ListMentors(c *fiber.Ctx) error {
	query := queryValues(c)
	filter := mentorship.DirectoryFilter{
		CompanySlug: query.Get("company"),
		Topic:       query.Get("topic"),
		Language:    query.Get("language"),
	}
	if limit := c.QueryInt("limit"); limit > 0 {
		filter.Limit = int32(limit)
	}

	profiles, err := h.mentorship.Directory(c.Context(), filter)
	if err != nil {
		return mentorshipError(err)
	}

	out := make([]mentorResponse, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, toMentorResponse(p))
	}

	meta := fiber.Map{"count": len(out)}
	// The dropped-filter rule: an endpoint whose answer WIDENS when it does not understand
	// a parameter says which ones it did not read. Refusing them instead would break a
	// shared link carrying a retired facet; staying silent is what let a mistyped filter
	// pass for a search.
	if ignored := unknownMentorParams(query); len(ignored) > 0 {
		meta["ignored_params"] = ignored
	}
	return c.JSON(fiber.Map{"data": out, "meta": meta})
}

// knownMentorParams is the directory's whole vocabulary. A filter added to
// DirectoryFilter and forgotten here is reported as ignored while in fact being applied,
// which is the confusing direction; one removed there and left here is applied silently,
// which is the dangerous one. They change together.
var knownMentorParams = map[string]bool{
	"company": true, "topic": true, "language": true, "limit": true,
}

func unknownMentorParams(query map[string][]string) []string {
	var out []string
	for key := range query {
		if !knownMentorParams[key] {
			out = append(out, key)
		}
	}
	return out
}

// GetMentor serves one public profile. An unpublished one answers 404 — the service reads
// through a query whose predicate makes it absent rather than filtered.
func (h *mentorshipHandlers) GetMentor(c *fiber.Ctx) error {
	profile, err := h.mentorship.PublicProfile(c.Context(), c.Params("slug"))
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": toMentorResponse(profile)})
}

// slotResponse is one offerable hour.
//
// It carries the absolute instant AND the local wall clock AND the UTC offset. All three,
// deliberately: the instant is what gets booked, the wall clock is what a person reads,
// and the offset is the only thing that distinguishes the two slots that share a label on
// the autumn daylight-saving transition — see the mentor-availability spec.
type slotResponse struct {
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	LocalStart string    `json:"local_start"`
	LocalEnd   string    `json:"local_end"`
	UTCOffset  string    `json:"utc_offset"`
}

// GetMentorSlots serves the bookable hours in a window, expressed in the viewer's zone.
func (h *mentorshipHandlers) GetMentorSlots(c *fiber.Ctx) error {
	from, to, err := slotWindow(c)
	if err != nil {
		return err
	}

	// Through the cache: this is the one public endpoint here that computes per request,
	// and it faces a crawler-heavy host.
	result, err := h.mentorship.CachedMentorSlots(c.Context(), c.Params("slug"),
		from, to, c.Query("timezone"))
	if err != nil {
		return mentorshipError(err)
	}

	out := make([]slotResponse, 0, len(result.Slots))
	for _, s := range result.Slots {
		_, offset := s.Start.Zone()
		out = append(out, slotResponse{
			StartsAt:   s.Start.UTC(),
			EndsAt:     s.End.UTC(),
			LocalStart: s.Start.Format(time.RFC3339),
			LocalEnd:   s.End.Format(time.RFC3339),
			UTCOffset:  offsetLabel(offset),
		})
	}

	// The zone is echoed because it may NOT be the one that was asked for: an
	// unrecognised name falls back to UTC, and a client that does not know which zone it
	// was answered in cannot tell a correct time from a wrong one.
	return c.JSON(fiber.Map{
		"data": out,
		"meta": fiber.Map{"timezone": result.Zone, "count": len(out)},
	})
}

// maxSlotWindowDays bounds how much a single request may ask to have computed. The
// horizon already bounds what can be OFFERED; this bounds the WORK, which is a different
// question on a crawler-heavy public endpoint.
const maxSlotWindowDays = 62

// slotWindow reads the requested window, defaulting to the next 30 days.
func slotWindow(c *fiber.Ctx) (from, to time.Time, err error) {
	now := time.Now().UTC()
	from, to = now, now.AddDate(0, 0, 30)

	if raw := c.Query("from"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return from, to, fiber.NewError(fiber.StatusBadRequest, "from must be an RFC 3339 timestamp")
		}
		from = parsed
	}
	if raw := c.Query("to"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return from, to, fiber.NewError(fiber.StatusBadRequest, "to must be an RFC 3339 timestamp")
		}
		to = parsed
	}
	if limit := from.AddDate(0, 0, maxSlotWindowDays); to.After(limit) {
		to = limit
	}
	return from, to, nil
}

// offsetLabel renders a zone offset as ±HH:MM, the form a reader recognises.
func offsetLabel(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	return sign + time.Unix(0, 0).UTC().Add(time.Duration(seconds)*time.Second).Format("15:04")
}

// mentorshipError is the ONE place this feature's domain errors become HTTP statuses.
// Each sentinel's mapping is recorded beside its declaration in the domain package; this
// switch is where those comments become true.
func mentorshipError(err error) error {
	switch {
	case errors.Is(err, mentorship.ErrNotAuthenticated):
		return fiber.NewError(fiber.StatusUnauthorized, "sign in to book a session")
	case errors.Is(err, mentorship.ErrInvalidProfile),
		errors.Is(err, mentorship.ErrInvalidSessionParams),
		errors.Is(err, mentorship.ErrInvalidReview),
		errors.Is(err, mentorship.ErrCannotBookYourself),
		errors.Is(err, mentorship.ErrInvalidTimeOfDay),
		errors.Is(err, mentorship.ErrInvalidDate),
		errors.Is(err, mentorship.ErrInvalidWeekday),
		errors.Is(err, mentorship.ErrInvalidSpan):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, mentorship.ErrProfileNotFound):
		return fiber.NewError(fiber.StatusNotFound, "mentor not found")
	case errors.Is(err, mentorship.ErrBookingNotFound):
		return fiber.NewError(fiber.StatusNotFound, "session not found")
	case errors.Is(err, mentorship.ErrCompanyNotFound):
		return fiber.NewError(fiber.StatusNotFound, "we don't have that company")
	case errors.Is(err, mentorship.ErrAlreadyAMentor):
		return fiber.NewError(fiber.StatusConflict, "you already have a mentor profile")
	case errors.Is(err, mentorship.ErrSlugTaken):
		return fiber.NewError(fiber.StatusConflict, "that profile address is taken")
	case errors.Is(err, mentorship.ErrProfileNotPending):
		return fiber.NewError(fiber.StatusConflict, "this profile is not pending")
	case errors.Is(err, mentorship.ErrSlotUnavailable):
		// One status for every ordinary way a booking fails to land — taken, withdrawn,
		// inside the notice period, past the horizon, or a lost race. They are one event
		// to the seeker, and splitting them would tell a stranger which.
		return fiber.NewError(fiber.StatusConflict, "that time is no longer available")
	case errors.Is(err, mentorship.ErrBookingNotCancellable):
		return fiber.NewError(fiber.StatusConflict, "this session can no longer be cancelled")
	case errors.Is(err, mentorship.ErrSessionNotCompleted):
		return fiber.NewError(fiber.StatusConflict, "this session has not taken place yet")
	case errors.Is(err, mentorship.ErrNoMentorZone):
		// A stored zone that no longer resolves is a broken row, not a bad request.
		return fiber.NewError(fiber.StatusInternalServerError, "this mentor's schedule is misconfigured")
	default:
		return err
	}
}

// bookingIDParam reads the session id from the path. A malformed one is 404 rather than
// 400: an id that cannot exist and one that does not are the same answer, and the
// distinction would only help somebody probing.
func bookingIDParam(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return id, nil
}
