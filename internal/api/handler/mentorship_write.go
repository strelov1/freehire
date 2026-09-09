package handler

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/mentorship"
)

// bookingRequest is what a seeker submits. The END is not in it: it is the mentor's
// session length, so a crafted payload cannot book four hours of somebody who offers one.
type bookingRequest struct {
	StartsAt string `json:"starts_at"`
	Timezone string `json:"timezone"`
	Note     string `json:"note"`
	JobID    int64  `json:"job_id"`
}

// bookingResponse is one session as a party to it sees it. The meeting link is here and
// not on the public profile: it is a live room, and only the two people meeting in it
// have any business holding the address.
type bookingResponse struct {
	ID          string    `json:"id"`
	MentorSlug  string    `json:"mentor_slug"`
	Headline    string    `json:"headline"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Status      string    `json:"status"`
	Note        string    `json:"note"`
	MeetingURL  string    `json:"meeting_url"`
	SeekerEmail string    `json:"seeker_email,omitempty"`
	Completed   bool      `json:"completed"`
}

func toBookingResponse(b mentorship.Booking, now time.Time) bookingResponse {
	return bookingResponse{
		ID: b.ID.String(), MentorSlug: b.MentorSlug, Headline: b.MentorHeadline,
		StartsAt: b.StartsAt.UTC(), EndsAt: b.EndsAt.UTC(), Status: b.Status,
		Note: b.Note, MeetingURL: b.MeetingURL, Completed: b.Completed(now),
	}
}

// BookSession takes one of a mentor's offered hours.
func (h *mentorshipHandlers) BookSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req bookingRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "starts_at must be an RFC 3339 timestamp")
	}

	booking, err := h.mentorship.Book(c.Context(), mentorship.BookingInput{
		MentorSlug:     c.Params("slug"),
		SeekerUserID:   userID,
		StartsAt:       startsAt,
		SeekerTimezone: req.Timezone,
		Note:           req.Note,
		JobID:          req.JobID,
	})
	if err != nil {
		return mentorshipError(err)
	}
	return c.Status(fiber.StatusCreated).
		JSON(fiber.Map{"data": toBookingResponse(booking, time.Now())})
}

// ListMySessions is the seeker's own list.
func (h *mentorshipHandlers) ListMySessions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	bookings, err := h.mentorship.MySessions(c.Context(), userID, int32(c.QueryInt("limit")))
	if err != nil {
		return mentorshipError(err)
	}
	sessions := seekerBookingList(bookings)
	return c.JSON(fiber.Map{"data": sessions, "meta": fiber.Map{"count": sessions.total()}})
}

// ListMentorBookings is the mentor's own list of who is coming. It carries each seeker's
// address — the mentor is meeting this person, so their identity is not a leak.
func (h *mentorshipHandlers) ListMentorBookings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	bookings, err := h.mentorship.MentorSessions(c.Context(), userID, int32(c.QueryInt("limit")))
	if err != nil {
		return mentorshipError(err)
	}
	sessions := mentorBookingList(bookings)
	return c.JSON(fiber.Map{"data": sessions, "meta": fiber.Map{"count": sessions.total()}})
}

// splitSessions is the wire shape both session lists take: "each split into upcoming and
// past", per the spec. Split HERE rather than by two queries against two clocks, so the
// halves cannot disagree about which side of now a session sits on — a session starting
// between two reads would otherwise appear in both, or in neither.
//
// Both slices are non-nil: a nil one marshals to `null`, and a client that gets null for
// "no past sessions" has to special-case it.
type splitSessions struct {
	Upcoming []bookingResponse `json:"upcoming"`
	Past     []bookingResponse `json:"past"`
}

func (s splitSessions) total() int { return len(s.Upcoming) + len(s.Past) }

// seekerBookingList and mentorBookingList differ by one field, and are two functions
// rather than one with a flag. The field is somebody's email address: a caller that has
// to pass `true` to withhold it is a caller that can pass `false` by mistake, and the
// mistake is silent.
func seekerBookingList(bookings []mentorship.Booking) splitSessions {
	return splitBookings(bookings, false)
}

// mentorBookingList additionally names who is coming — the mentor is meeting this person,
// so their identity is not a leak.
func mentorBookingList(bookings []mentorship.Booking) splitSessions {
	return splitBookings(bookings, true)
}

// splitBookings renders one party's sessions into the two halves, against ONE clock for
// the whole list.
func splitBookings(bookings []mentorship.Booking, nameTheSeeker bool) splitSessions {
	now := time.Now()
	out := splitSessions{Upcoming: []bookingResponse{}, Past: []bookingResponse{}}
	for _, b := range bookings {
		row := toBookingResponse(b, now)
		if nameTheSeeker {
			row.SeekerEmail = b.SeekerEmail
		}
		// "Upcoming" is still-to-happen AND still on: a cancelled session is behind you
		// whatever its clock says, because nobody is going to it.
		if b.Status == mentorship.BookingConfirmed && b.StartsAt.After(now) {
			out.Upcoming = append(out.Upcoming, row)
			continue
		}
		out.Past = append(out.Past, row)
	}
	return out
}

// GetSession reads one session. Either party may; anybody else gets the 404 a session
// that does not exist gets, so the route cannot be used to confirm one is real.
func (h *mentorshipHandlers) GetSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := bookingIDParam(c)
	if err != nil {
		return err
	}

	booking, err := h.mentorship.Session(c.Context(), id, userID)
	if err != nil {
		return mentorshipError(err)
	}

	row := toBookingResponse(booking, time.Now())
	// The mentor sees who is coming; the seeker does not need their own address back.
	if userID == booking.MentorUserID {
		row.SeekerEmail = booking.SeekerEmail
	}
	return c.JSON(fiber.Map{"data": row})
}

type cancelRequest struct {
	Reason string `json:"reason"`
}

// CancelSession ends a session. Either party may; anybody else gets the same 404 a
// session that does not exist gets.
func (h *mentorshipHandlers) CancelSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := bookingIDParam(c)
	if err != nil {
		return err
	}
	var req cancelRequest
	// A cancellation with no body is a cancellation with no reason, which is allowed.
	_ = c.BodyParser(&req)

	booking, err := h.mentorship.Cancel(c.Context(), id, userID, req.Reason)
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": toBookingResponse(booking, time.Now())})
}

type reviewRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// ReviewSession records the seeker's rating. PUT, not POST: there is one review per
// session and a second submission replaces the first, which is what PUT means.
func (h *mentorshipHandlers) ReviewSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := bookingIDParam(c)
	if err != nil {
		return err
	}
	var req reviewRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	review, err := h.mentorship.Review(c.Context(), id, userID, req.Rating, req.Comment)
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"booking_id": review.BookingID.String(),
		"rating":     review.Rating,
		"comment":    review.Comment,
	}})
}

// profileRequest is a mentor profile as submitted or edited. The session figures are
// minutes and days — the units a person types — and are converted once, here.
type profileRequest struct {
	CompanySlug         string   `json:"company_slug"`
	Slug                string   `json:"slug"`
	Name                string   `json:"name"`
	Headline            string   `json:"headline"`
	Bio                 string   `json:"bio"`
	Topics              []string `json:"topics"`
	Languages           []string `json:"languages"`
	Timezone            string   `json:"timezone"`
	SessionMinutes      int      `json:"session_minutes"`
	BufferBeforeMinutes int      `json:"buffer_before_minutes"`
	BufferAfterMinutes  int      `json:"buffer_after_minutes"`
	NoticeMinutes       int      `json:"notice_minutes"`
	HorizonDays         int      `json:"horizon_days"`
	MeetingURL          string   `json:"meeting_url"`
	// ShowPhoto is the mentor's own opt-in to publish their account's CV headshot. Off
	// by default; see mentorship.Profile.ShowPhoto.
	ShowPhoto bool `json:"show_photo"`
}

func (r profileRequest) toInput(userID int64) mentorship.ProfileInput {
	return mentorship.ProfileInput{
		UserID:      userID,
		CompanySlug: r.CompanySlug,
		Slug:        r.Slug,
		DisplayName: r.Name,
		Headline:    r.Headline,
		Bio:         r.Bio,
		Topics:      r.Topics,
		Languages:   r.Languages,
		Timezone:    r.Timezone,
		Session: mentorship.SessionParams{
			Duration:      time.Duration(r.SessionMinutes) * time.Minute,
			BufferBefore:  time.Duration(r.BufferBeforeMinutes) * time.Minute,
			BufferAfter:   time.Duration(r.BufferAfterMinutes) * time.Minute,
			MinimumNotice: time.Duration(r.NoticeMinutes) * time.Minute,
			Horizon:       time.Duration(r.HorizonDays) * 24 * time.Hour,
		},
		MeetingURL: r.MeetingURL,
		ShowPhoto:  r.ShowPhoto,
	}
}

// GetMyMentorProfile is the owner's own profile, whatever its status — a pending one must
// still be visible to the person who submitted it.
func (h *mentorshipHandlers) GetMyMentorProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	profile, found, err := h.mentorship.MyProfile(c.Context(), userID)
	if err != nil {
		return mentorshipError(err)
	}
	if !found {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": toOwnMentorResponse(profile)})
}

func (h *mentorshipHandlers) SubmitMentorProfile(c *fiber.Ctx) error {
	userID, req, err := mentorProfileBody(c)
	if err != nil {
		return err
	}
	in, err := h.withCalendarLink(c, userID, req)
	if err != nil {
		return err
	}
	profile, err := h.mentorship.SubmitProfile(c.Context(), in)
	if err != nil {
		return mentorshipError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toOwnMentorResponse(profile)})
}

func (h *mentorshipHandlers) UpdateMentorProfile(c *fiber.Ctx) error {
	userID, req, err := mentorProfileBody(c)
	if err != nil {
		return err
	}
	in, err := h.withCalendarLink(c, userID, req)
	if err != nil {
		return err
	}
	profile, err := h.mentorship.UpdateProfile(c.Context(), in)
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": toOwnMentorResponse(profile)})
}

// withCalendarLink resolves whether the caller holds a connected calendar.events grant
// and sets it on the input, so a connected mentor's meeting link is no longer required —
// the same test CreateMeetEvent's own gate applies at booking time.
//
// Skipped when the submitted link is already non-empty: validateMeetingURL only ever
// consults HasCalendarLink for an EMPTY link, so a mentor who filled the field in pays
// no extra round trip to answer a question that cannot change their outcome.
func (h *mentorshipHandlers) withCalendarLink(c *fiber.Ctx, userID int64, req profileRequest) (mentorship.ProfileInput, error) {
	in := req.toInput(userID)
	if strings.TrimSpace(in.MeetingURL) != "" {
		return in, nil
	}
	hasCalendar, err := h.mentorship.HasConnectedCalendar(c.Context(), userID)
	if err != nil {
		return mentorship.ProfileInput{}, err
	}
	in.HasCalendarLink = hasCalendar
	return in, nil
}

func mentorProfileBody(c *fiber.Ctx) (int64, profileRequest, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return 0, profileRequest{}, err
	}
	var req profileRequest
	if err := c.BodyParser(&req); err != nil {
		return 0, profileRequest{}, fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return userID, req, nil
}

type pauseRequest struct {
	Paused bool `json:"paused"`
}

// PauseMentorProfile flips the mentor's own switch. It needs no moderator and must not
// disturb what one decided.
func (h *mentorshipHandlers) PauseMentorProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req pauseRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	profile, err := h.mentorship.SetPaused(c.Context(), userID, req.Paused)
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": toOwnMentorResponse(profile)})
}

// WithdrawMentorProfile removes the profile, cancelling and announcing every future
// session first.
func (h *mentorshipHandlers) WithdrawMentorProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.mentorship.Withdraw(c.Context(), userID); err != nil {
		return mentorshipError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ReactivateMentorProfile resubmits a withdrawn profile for moderation. It is refused,
// not silently accepted, for a profile that was never withdrawn.
func (h *mentorshipHandlers) ReactivateMentorProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	profile, err := h.mentorship.Reactivate(c.Context(), userID)
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": toOwnMentorResponse(profile)})
}

// ListPendingMentorProfiles is the moderation queue.
func (h *mentorshipHandlers) ListPendingMentorProfiles(c *fiber.Ctx) error {
	pending, err := h.mentorship.PendingQueue(c.Context())
	if err != nil {
		return mentorshipError(err)
	}

	type queueRow struct {
		mentorResponse
		ID int64 `json:"id"`
		// HasApprovedReferralOffer is evidence for the human deciding — this account is
		// already an approved referrer for the same company — and never a gate.
		HasApprovedReferralOffer bool `json:"has_approved_referral_offer"`
	}

	out := make([]queueRow, 0, len(pending))
	for _, p := range pending {
		out = append(out, queueRow{
			mentorResponse:           toModeratorMentorResponse(p.Profile),
			ID:                       p.ID,
			HasApprovedReferralOffer: p.HasApprovedReferralOffer,
		})
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"count": len(out)}})
}

type decideRequest struct {
	Status string `json:"status"`
}

// DecideMentorProfile approves or rejects a pending profile.
func (h *mentorshipHandlers) DecideMentorProfile(c *fiber.Ctx) error {
	moderatorID, err := requireUserID(c)
	if err != nil {
		return err
	}
	profileID, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "profile not found")
	}
	var req decideRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	profile, err := h.mentorship.Decide(c.Context(), int64(profileID), moderatorID, req.Status)
	if err != nil {
		return mentorshipError(err)
	}
	return c.JSON(fiber.Map{"data": toModeratorMentorResponse(profile)})
}
