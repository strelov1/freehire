package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// mentorshipHandlers serves the mentorship marketplace: the public directory and its
// slots, the seeker's bookings, the mentor's own cabinet, and the moderation queue.
type mentorshipHandlers struct {
	mentorship *mentorship.Service
	// photos is nil-safe (headshot.Store.Enabled() is false on a nil receiver), so an
	// unconfigured object store makes GetMentorPhoto "not found", not a server error.
	photos *headshot.Store
}

func newMentorshipHandlers(svc *mentorship.Service, photos *headshot.Store) *mentorshipHandlers {
	return &mentorshipHandlers{mentorship: svc, photos: photos}
}

func (h *mentorshipHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	h.registerPublic(api, mw, contract)

	// The seeker's side.
	api.Post("/mentors/:slug/bookings", mw.key, contract)
	api.Get("/me/mentorship/sessions", mw.key, contract)
	api.Get("/me/mentorship/sessions/:id", mw.key, contract)
	api.Post("/me/mentorship/sessions/:id/cancel", mw.key, contract)
	api.Put("/me/mentorship/sessions/:id/review", mw.key, contract)

	// The mentor's own cabinet: their profile, their schedule, their bookings.
	api.Get("/me/mentorship/profile", mw.key, contract)
	api.Post("/me/mentorship/profile", mw.key, contract)
	api.Put("/me/mentorship/profile", mw.key, contract)
	api.Delete("/me/mentorship/profile", mw.key, contract)
	api.Post("/me/mentorship/profile/pause", mw.key, contract)
	api.Post("/me/mentorship/profile/reactivate", mw.key, contract)
	api.Get("/me/mentorship/availability", mw.key, contract)
	api.Get("/me/mentorship/availability/calendar", mw.key, contract)
	api.Put("/me/mentorship/availability/weekly", mw.key, contract)
	api.Post("/me/mentorship/availability/overrides", mw.key, contract)
	api.Delete("/me/mentorship/availability/:id", mw.key, contract)
	api.Get("/me/mentorship/bookings", mw.key, contract)

	// Moderation, behind the same gate the referral queue uses.
	api.Get("/mentorship/profiles", mw.key, mw.moderator, contract)
	api.Get("/mentorship/profiles/:id/photo", mw.key, mw.moderator, h.GetPendingMentorPhoto)
	api.Post("/mentorship/profiles/:id/decide", mw.key, mw.moderator, contract)
}

// registerPublic mounts the anonymous mentor catalogue.
func (h *mentorshipHandlers) registerPublic(api fiber.Router, mw middleware, contract fiber.Handler) {

	api.Get("/mentors", contract)
	api.Get("/mentors/:slug", contract)
	api.Get("/mentors/:slug/slots", contract)
	api.Get("/mentors/:slug/photo", h.GetMentorPhoto)
}

// toMentorResponse renders a profile for the PUBLIC. The meeting link is deliberately
// absent: it is a live room, and publishing it lets anyone walk into someone else's session.
// A booked seeker gets it on their booking. user_id is internal and omitted; Name stays, so
// the directory lists people rather than headlines.
func toMentorResponse(p mentorship.Profile) apigen.MentorshipMentor {
	return apigen.MentorshipMentor{
		Slug: p.Slug, Name: p.DisplayName,
		CompanySlug: p.CompanySlug, CompanyName: p.CompanyName,
		Headline: p.Headline, Bio: p.Bio, Topics: p.Topics, Languages: p.Languages,
		Timezone: p.Timezone,
		// What a seeker is booking belongs on the card too: an hour and 20 minutes differ.
		SessionMinutes: int64(p.Session.Duration / time.Minute),
		RatingCount:    p.RatingCount, RatingAvg: p.RatingAvg,
		// ShowPhoto is the mentor's own opt-in to serve their stored CV headshot publicly (see
		// GetMentorPhoto); the card needs it to decide whether to render an avatar. Seniority
		// is the platform's closed vocabulary (vocab.SeniorityValues), absent when unset.
		ShowPhoto: p.ShowPhoto, Seniority: optString(p.Seniority),
	}
}

// toOwnMentorResponse is the profile as its OWNER sees it: with the moderation status,
// the pause switch and the meeting link they configured.
func toOwnMentorResponse(p mentorship.Profile) apigen.MentorshipOwnMentor {
	m := toModeratorMentorResponse(p)
	return apigen.MentorshipOwnMentor{
		Slug: m.Slug, Name: m.Name, CompanySlug: m.CompanySlug, CompanyName: m.CompanyName,
		Headline: m.Headline, Bio: m.Bio, Topics: m.Topics, Languages: m.Languages,
		Timezone: m.Timezone, SessionMinutes: m.SessionMinutes,
		RatingCount: m.RatingCount, RatingAvg: m.RatingAvg, ShowPhoto: m.ShowPhoto, Seniority: m.Seniority,
		Status: m.Status, Paused: m.Paused, CreatedAt: m.CreatedAt,
		MeetingURL: optString(p.MeetingURL),
		// The parameters the cabinet edits, for the OWNER alone, read back so a correction
		// re-submits what the mentor chose. A zero buffer is real, so these are always sent.
		BufferBeforeMinutes: int64(p.Session.BufferBefore / time.Minute),
		BufferAfterMinutes:  int64(p.Session.BufferAfter / time.Minute),
		NoticeMinutes:       int64(p.Session.MinimumNotice / time.Minute),
		HorizonDays:         int64(p.Session.Horizon / (24 * time.Hour)),
	}
}

// toModeratorMentorResponse is the profile as a MODERATOR sees it: everything the owner sees
// except the meeting link, which approving a mentor does not require. created_at (when the
// profile was submitted) appears only in the moderator and owner views.
func toModeratorMentorResponse(p mentorship.Profile) apigen.MentorshipModeratedMentor {
	m := toMentorResponse(p)
	return apigen.MentorshipModeratedMentor{
		Slug: m.Slug, Name: m.Name, CompanySlug: m.CompanySlug, CompanyName: m.CompanyName,
		Headline: m.Headline, Bio: m.Bio, Topics: m.Topics, Languages: m.Languages,
		Timezone: m.Timezone, SessionMinutes: m.SessionMinutes,
		RatingCount: m.RatingCount, RatingAvg: m.RatingAvg, ShowPhoto: m.ShowPhoto, Seniority: m.Seniority,
		Status: apigen.MentorshipProfileStatus(p.Status), Paused: p.Paused, CreatedAt: p.CreatedAt,
	}
}

// ListMentors serves the public directory.
func (h *mentorshipHandlers) ListMentors(ctx context.Context, params apigen.ListMentorsParams) (*apigen.MentorshipDirectoryPage, error) {
	query := contractQuery(ctx)
	filter := mentorship.DirectoryFilter{
		CompanySlug: params.Company.Value,
		Topic:       params.Topic.Value,
		Language:    params.Language.Value,
		Seniority:   params.Seniority.Value,
		Query:       params.Q.Value,
	}
	filter.NoReviewsOnly, _ = strconv.ParseBool(params.NoReviews.Value)
	if limit := lenientInt32(params.Limit.Value); limit > 0 {
		filter.Limit = limit
	}

	profiles, err := h.mentorship.Directory(ctx, filter)
	if err != nil {
		return nil, mentorshipError(err)
	}

	out := make([]apigen.MentorshipMentor, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, toMentorResponse(p))
	}

	// The dropped-filter rule: an endpoint whose answer WIDENS on a parameter it does not
	// understand names it. Refusing would break shared links carrying a retired facet;
	// silence let a mistyped filter pass for a search.
	return &apigen.MentorshipDirectoryPage{
		Data: out,
		Meta: apigen.MentorshipDirectoryMeta{Count: int64(len(out)), IgnoredParams: unknownMentorParams(query)},
	}, nil
}

// lenientInt32 reads a limit the way Fiber's QueryInt did: a value that does not read is 0,
// and one past int32 is 0 too rather than wrapping.
func lenientInt32(raw string) int32 {
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

// knownMentorParams is the directory's whole vocabulary. A filter added to DirectoryFilter
// and forgotten here is reported as ignored while applied; one removed there and left here
// is applied silently (the dangerous one). Change them together.
var knownMentorParams = map[string]bool{
	"company": true, "topic": true, "language": true, "limit": true,
	"seniority": true, "q": true, "no_reviews": true,
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

// GetMentor serves one public profile. An unpublished one is 404: the service reads through
// a query whose predicate makes it absent.
func (h *mentorshipHandlers) GetMentor(ctx context.Context, params apigen.GetMentorParams) (*apigen.GetMentorOK, error) {
	profile, err := h.mentorship.PublicProfile(ctx, params.Slug)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.GetMentorOK{Data: toMentorResponse(profile)}, nil
}

// errMentorPhotoNotFound is what every reason a public photo may not be served collapses to
// (not opted in, no stored headshot, storage unconfigured), so the response cannot leak
// whether an opted-out mentor has a CV photo.
var errMentorPhotoNotFound = errors.New("mentor photo not available")

// mentorPhoto resolves an already-public mentor's photo bytes from their opt-in and the
// account's stored headshot. It does not check publication: GetMentorPhoto reuses
// PublicProfile for that, the same predicate as GetMentor.
func mentorPhoto(ctx context.Context, profile mentorship.Profile, photos *headshot.Store) ([]byte, error) {
	if !profile.ShowPhoto {
		return nil, errMentorPhotoNotFound
	}
	data, err := photos.Get(ctx, profile.UserID)
	if err != nil {
		return nil, errMentorPhotoNotFound
	}
	return data, nil
}

// GetMentorPhoto serves a published mentor's account headshot, only when that mentor has
// opted in via show_photo. An unpublished profile answers 404 exactly as GetMentor does.
func (h *mentorshipHandlers) GetMentorPhoto(c *fiber.Ctx) error {
	profile, err := h.mentorship.PublicProfile(c.Context(), c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound)
	}
	data, err := mentorPhoto(c.Context(), profile, h.photos)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound)
	}
	c.Set(fiber.HeaderContentType, "image/jpeg")
	return c.Send(data)
}

// GetPendingMentorPhoto lets a moderator preview a pending mentor's opted-in photo, resolved
// by the row id off the queue and unconditional on status: ProfileForModeration has no
// publication predicate because the mw.moderator gate is the access control.
func (h *mentorshipHandlers) GetPendingMentorPhoto(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound)
	}
	profile, err := h.mentorship.ProfileForModeration(c.Context(), int64(id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound)
	}
	data, err := mentorPhoto(c.Context(), profile, h.photos)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound)
	}
	c.Set(fiber.HeaderContentType, "image/jpeg")
	return c.Send(data)
}

// slotResponse is one offerable hour: the absolute instant (what gets booked), the local
// wall clock (what a person reads) and the UTC offset, which alone tells apart the two slots
// sharing a label on the autumn daylight-saving transition.
type slotResponse struct {
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	LocalStart string    `json:"local_start"`
	LocalEnd   string    `json:"local_end"`
	UTCOffset  string    `json:"utc_offset"`
}

// GetMentorSlots serves the bookable hours in a window, expressed in the viewer's zone.
func (h *mentorshipHandlers) GetMentorSlots(ctx context.Context, params apigen.GetMentorSlotsParams) (*apigen.MentorshipSlotsPage, error) {
	from, to, err := slotWindowOf(params.From.Value, params.To.Value)
	if err != nil {
		return nil, err
	}

	// Through the cache: the one public endpoint here that computes per request, on a
	// crawler-heavy host.
	result, err := h.mentorship.CachedMentorSlots(ctx, params.Slug, from, to, params.Timezone.Value)
	if err != nil {
		return nil, mentorshipError(err)
	}

	out := make([]apigen.BuddyMockSlot, 0, len(result.Slots))
	for _, s := range slotResponses(result.Slots) {
		out = append(out, apigen.BuddyMockSlot{
			StartsAt: s.StartsAt, EndsAt: s.EndsAt, LocalStart: s.LocalStart, LocalEnd: s.LocalEnd, UtcOffset: s.UTCOffset,
		})
	}

	// The zone is echoed because it may differ from the one asked for (an unrecognised name
	// falls back to UTC), and the client could not otherwise tell a wrong time from a right one.
	return &apigen.MentorshipSlotsPage{
		Data: out,
		Meta: apigen.MentorshipTimezoneMeta{Timezone: result.Zone, Count: int64(len(out))},
	}, nil
}

// slotResponses renders engine slots for the wire: the instant, the wall clock and the
// offset, for the reasons slotResponse gives. Buddy mock slots use it too.
func slotResponses(ivs []slots.Interval) []slotResponse {
	out := make([]slotResponse, 0, len(ivs))
	for _, s := range ivs {
		_, offset := s.Start.Zone()
		out = append(out, slotResponse{
			StartsAt:   s.Start.UTC(),
			EndsAt:     s.End.UTC(),
			LocalStart: s.Start.Format(time.RFC3339),
			LocalEnd:   s.End.Format(time.RFC3339),
			UTCOffset:  offsetLabel(offset),
		})
	}
	return out
}

// maxSlotWindowDays bounds the WORK one request may ask for; the horizon already bounds
// what can be OFFERED. It matters on a crawler-heavy public endpoint.
const maxSlotWindowDays = 62

// slotWindowOf reads the requested window from the raw from and to values, defaulting
// to the next 30 days.
func slotWindowOf(rawFrom, rawTo string) (from, to time.Time, err error) {
	var fromAt, toAt *time.Time
	if rawFrom != "" {
		parsed, parseErr := time.Parse(time.RFC3339, rawFrom)
		if parseErr != nil {
			return from, to, fiber.NewError(fiber.StatusBadRequest, "from must be an RFC 3339 timestamp")
		}
		fromAt = &parsed
	}
	if rawTo != "" {
		parsed, parseErr := time.Parse(time.RFC3339, rawTo)
		if parseErr != nil {
			return from, to, fiber.NewError(fiber.StatusBadRequest, "to must be an RFC 3339 timestamp")
		}
		toAt = &parsed
	}
	from, to = slotWindowBounds(fromAt, toAt)
	return from, to, nil
}

// slotWindowBounds fills an unset end of the window (from now, to 30 days after from) and clamps
// it to maxSlotWindowDays, for the routes and the MCP tools alike.
func slotWindowBounds(fromAt, toAt *time.Time) (from, to time.Time) {
	from = time.Now().UTC()
	if fromAt != nil {
		from = *fromAt
	}
	to = from.AddDate(0, 0, 30)
	if toAt != nil {
		to = *toAt
	}
	if limit := from.AddDate(0, 0, maxSlotWindowDays); to.After(limit) {
		to = limit
	}
	return from, to
}

// offsetLabel renders a zone offset as ±HH:MM, the form a reader recognises.
func offsetLabel(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	return sign + time.Unix(0, 0).UTC().Add(time.Duration(seconds)*time.Second).Format("15:04")
}

// mentorshipError is the ONE place this feature's domain errors become HTTP statuses; each
// sentinel's mapping is recorded beside its declaration in the domain package.
func mentorshipError(err error) error {
	switch {
	case errors.Is(err, mentorship.ErrNotAuthenticated):
		return fiber.NewError(fiber.StatusUnauthorized, "sign in to book a session")
	case errors.Is(err, mentorship.ErrInvalidProfile),
		errors.Is(err, slots.ErrInvalidSessionParams),
		errors.Is(err, mentorship.ErrInvalidReview),
		errors.Is(err, mentorship.ErrCannotBookYourself),
		errors.Is(err, slots.ErrInvalidTimeOfDay),
		errors.Is(err, slots.ErrInvalidDate),
		errors.Is(err, slots.ErrInvalidWeekday),
		errors.Is(err, slots.ErrInvalidSpan):
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
	case errors.Is(err, mentorship.ErrProfileNotWithdrawn):
		return fiber.NewError(fiber.StatusConflict, "this profile is not withdrawn")
	case errors.Is(err, mentorship.ErrSlotUnavailable):
		// One status for every ordinary way a booking fails to land (taken, withdrawn, inside
		// the notice period, past the horizon, lost race), so a stranger cannot tell which.
		return fiber.NewError(fiber.StatusConflict, "that time is no longer available")
	case errors.Is(err, mentorship.ErrBookingNotCancellable):
		return fiber.NewError(fiber.StatusConflict, "this session can no longer be cancelled")
	case errors.Is(err, mentorship.ErrSessionNotCompleted):
		return fiber.NewError(fiber.StatusConflict, "this session has not taken place yet")
	case errors.Is(err, slots.ErrNoOwnerZone):
		// A stored zone that no longer resolves is a broken row, not a bad request.
		return fiber.NewError(fiber.StatusInternalServerError, "this mentor's schedule is misconfigured")
	default:
		return err
	}
}

// bookingIDParam reads the session id from the path. A malformed one is 404, not 400: it
// answers the same as an id that does not exist, and the distinction would only help probing.
func bookingIDParam(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return id, nil
}
