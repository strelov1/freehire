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
	// photos is nil-safe (headshot.Store.Enabled() reports false on a nil receiver), so
	// an unconfigured object store degrades GetMentorPhoto to "not found" rather than a
	// server error.
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
// absent: it is a live room, and publishing it lets anybody who reads the directory walk
// into somebody else's session. A booked seeker gets it on their booking.
//
// user_id is not in it: it is ownership, internal, and nothing public needs it. Name is
// what makes this a person rather than a role — a directory of headlines is the anonymous
// referral pool with extra steps.
func toMentorResponse(p mentorship.Profile) apigen.MentorshipMentor {
	return apigen.MentorshipMentor{
		Slug: p.Slug, Name: p.DisplayName,
		CompanySlug: p.CompanySlug, CompanyName: p.CompanyName,
		Headline: p.Headline, Bio: p.Bio, Topics: p.Topics, Languages: p.Languages,
		Timezone: p.Timezone,
		// What a seeker is booking, so it belongs on the card as well as the profile: an
		// hour and twenty minutes are different offers.
		SessionMinutes: int64(p.Session.Duration / time.Minute),
		RatingCount:    p.RatingCount, RatingAvg: p.RatingAvg,
		// ShowPhoto is the mentor's own opt-in to serve their account's stored CV headshot
		// publicly (see GetMentorPhoto). Present in every view: the directory card needs it
		// to decide whether to render an avatar at all. Seniority is the platform's own
		// closed vocabulary (vocab.SeniorityValues), absent when the mentor left it unset.
		ShowPhoto: p.ShowPhoto, Seniority: optString(p.Seniority),
	}
}

// toOwnMentorResponse is the profile as its OWNER sees it: with the moderation status,
// the pause switch and the meeting link they configured.
func toOwnMentorResponse(p mentorship.Profile) apigen.MentorshipMentor {
	out := toModeratorMentorResponse(p)
	out.MeetingURL = optString(p.MeetingURL)
	// The parameters the cabinet edits, for the OWNER alone. Read back so a mentor
	// correcting one field re-submits what they actually chose rather than this
	// endpoint's defaults. Optional rather than zero-omitted: a zero buffer is a real
	// setting.
	out.BufferBeforeMinutes = apigen.NewOptInt64(int64(p.Session.BufferBefore / time.Minute))
	out.BufferAfterMinutes = apigen.NewOptInt64(int64(p.Session.BufferAfter / time.Minute))
	out.NoticeMinutes = apigen.NewOptInt64(int64(p.Session.MinimumNotice / time.Minute))
	out.HorizonDays = apigen.NewOptInt64(int64(p.Session.Horizon / (24 * time.Hour)))
	return out
}

// toModeratorMentorResponse is the profile as a MODERATOR sees it: everything the owner
// sees except the meeting link.
//
// The link is a live room, and deciding whether somebody may mentor does not require the
// address of the room they meet in. A moderator is trusted, which is exactly why this is
// worth being deliberate about: the reason they hold no link is that they have no use for
// one, not that they are suspected of anything.
//
// created_at is when the profile was submitted — moderator and owner views only. A
// visitor deciding whether to book has no use for it.
func toModeratorMentorResponse(p mentorship.Profile) apigen.MentorshipMentor {
	out := toMentorResponse(p)
	out.Status = optString(p.Status)
	out.Paused = apigen.OptBool{Value: true, Set: p.Paused}
	out.CreatedAt = apigen.NewOptDateTime(p.CreatedAt)
	return out
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

	// The dropped-filter rule: an endpoint whose answer WIDENS when it does not understand
	// a parameter says which ones it did not read. Refusing them instead would break a
	// shared link carrying a retired facet; staying silent is what let a mistyped filter
	// pass for a search.
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

// knownMentorParams is the directory's whole vocabulary. A filter added to
// DirectoryFilter and forgotten here is reported as ignored while in fact being applied,
// which is the confusing direction; one removed there and left here is applied silently,
// which is the dangerous one. They change together.
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

// GetMentor serves one public profile. An unpublished one answers 404 — the service reads
// through a query whose predicate makes it absent rather than filtered.
func (h *mentorshipHandlers) GetMentor(ctx context.Context, params apigen.GetMentorParams) (*apigen.GetMentorOK, error) {
	profile, err := h.mentorship.PublicProfile(ctx, params.Slug)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.GetMentorOK{Data: toMentorResponse(profile)}, nil
}

// errMentorPhotoNotFound is what every reason a mentor's public photo may not be served
// collapses to: not opted in, no stored headshot, or storage unconfigured. They are
// deliberately indistinguishable from outside — a response that told them apart would
// leak whether an opted-out mentor has a CV photo at all, the narrower version of the
// privacy concern the opt-in itself exists to address.
var errMentorPhotoNotFound = errors.New("mentor photo not available")

// mentorPhoto resolves an already-public mentor's photo bytes, given their opt-in and
// the account's stored headshot. It does not itself resolve the profile or check
// publication status — GetMentorPhoto reuses PublicProfile for that, the same
// resolution and predicate GetMentor uses, so the two routes cannot drift on what
// "publicly readable" means.
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

// GetPendingMentorPhoto lets a moderator preview a still-pending mentor's opted-in
// photo — the same bytes GetMentorPhoto would serve once approved, but resolved by the
// row id a moderator already reads off the queue rather than by slug, and unconditional
// on status: ProfileForModeration carries no publication predicate, because the
// mw.moderator gate in front of this route is the access control, not a second copy of
// the public one.
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
func (h *mentorshipHandlers) GetMentorSlots(ctx context.Context, params apigen.GetMentorSlotsParams) (*apigen.MentorshipSlotsPage, error) {
	from, to, err := slotWindowOf(params.From.Value, params.To.Value)
	if err != nil {
		return nil, err
	}

	// Through the cache: this is the one public endpoint here that computes per request,
	// and it faces a crawler-heavy host.
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

	// The zone is echoed because it may NOT be the one that was asked for: an
	// unrecognised name falls back to UTC, and a client that does not know which zone it
	// was answered in cannot tell a correct time from a wrong one.
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

// maxSlotWindowDays bounds how much a single request may ask to have computed. The
// horizon already bounds what can be OFFERED; this bounds the WORK, which is a different
// question on a crawler-heavy public endpoint.
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

// mentorshipError is the ONE place this feature's domain errors become HTTP statuses.
// Each sentinel's mapping is recorded beside its declaration in the domain package; this
// switch is where those comments become true.
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
		// One status for every ordinary way a booking fails to land — taken, withdrawn,
		// inside the notice period, past the horizon, or a lost race. They are one event
		// to the seeker, and splitting them would tell a stranger which.
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

// bookingIDParam reads the session id from the path. A malformed one is 404 rather than
// 400: an id that cannot exist and one that does not are the same answer, and the
// distinction would only help somebody probing.
func bookingIDParam(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return id, nil
}
