package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// toBookingResponse is one session as a party to it sees it. The meeting link is here and
// not on the public profile: it is a live room, and only the two people meeting in it
// have any business holding the address.
func toBookingResponse(b mentorship.Booking, now time.Time) apigen.MentorshipBooking {
	return apigen.MentorshipBooking{
		ID: b.ID.String(), MentorSlug: b.MentorSlug, Headline: b.MentorHeadline,
		StartsAt: b.StartsAt.UTC(), EndsAt: b.EndsAt.UTC(), Status: apigen.MentorshipBookingStatus(b.Status),
		Note: b.Note, MeetingURL: b.MeetingURL, Completed: b.Completed(now),
	}
}

// BookSession takes one of a mentor's offered hours. The END is not in the request: it is
// the mentor's session length, so a crafted payload cannot book four hours of somebody who
// offers one.
func (h *mentorshipHandlers) BookSession(ctx context.Context, req *apigen.MentorshipBookingBody, params apigen.BookSessionParams) (*apigen.BookSessionCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt.Value)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "starts_at must be an RFC 3339 timestamp")
	}

	booking, err := h.mentorship.Book(ctx, mentorship.BookingInput{
		MentorSlug:     params.Slug,
		SeekerUserID:   userID,
		StartsAt:       startsAt,
		SeekerTimezone: req.Timezone.Value,
		Note:           req.Note.Value,
		JobID:          req.JobID.Value,
	})
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.BookSessionCreated{Data: toBookingResponse(booking, time.Now())}, nil
}

// ListMySessions is the seeker's own list.
func (h *mentorshipHandlers) ListMySessions(ctx context.Context, params apigen.ListMySessionsParams) (*apigen.MentorshipSessionsPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	bookings, err := h.mentorship.MySessions(ctx, userID, lenientInt32(params.Limit.Value))
	if err != nil {
		return nil, mentorshipError(err)
	}
	return sessionsPage(seekerBookingList(bookings)), nil
}

// ListMentorBookings is the mentor's own list of who is coming. It carries each seeker's
// address — the mentor is meeting this person, so their identity is not a leak.
func (h *mentorshipHandlers) ListMentorBookings(ctx context.Context, params apigen.ListMentorBookingsParams) (*apigen.MentorshipSessionsPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	bookings, err := h.mentorship.MentorSessions(ctx, userID, lenientInt32(params.Limit.Value))
	if err != nil {
		return nil, mentorshipError(err)
	}
	return sessionsPage(mentorBookingList(bookings)), nil
}

func sessionsPage(sessions apigen.MentorshipSessions) *apigen.MentorshipSessionsPage {
	return &apigen.MentorshipSessionsPage{
		Data: sessions,
		Meta: apigen.MentorshipCountMeta{Count: int64(len(sessions.Upcoming) + len(sessions.Past))},
	}
}

// seekerBookingList and mentorBookingList differ by one field, and are two functions
// rather than one with a flag. The field is somebody's email address: a caller that has
// to pass `true` to withhold it is a caller that can pass `false` by mistake, and the
// mistake is silent.
func seekerBookingList(bookings []mentorship.Booking) apigen.MentorshipSessions {
	return splitBookings(bookings, false)
}

// mentorBookingList additionally names who is coming — the mentor is meeting this person,
// so their identity is not a leak.
func mentorBookingList(bookings []mentorship.Booking) apigen.MentorshipSessions {
	return splitBookings(bookings, true)
}

// splitBookings renders one party's sessions into the wire shape both session lists take:
// "each split into upcoming and past", per the spec. Split HERE rather than by two queries
// against two clocks, so the halves cannot disagree about which side of now a session sits
// on — a session starting between two reads would otherwise appear in both, or in neither.
// One clock for the whole list.
//
// Both slices are non-nil: a nil one marshals to `null`, and a client that gets null for
// "no past sessions" has to special-case it.
func splitBookings(bookings []mentorship.Booking, nameTheSeeker bool) apigen.MentorshipSessions {
	now := time.Now()
	out := apigen.MentorshipSessions{Upcoming: []apigen.MentorshipBooking{}, Past: []apigen.MentorshipBooking{}}
	for _, b := range bookings {
		row := toBookingResponse(b, now)
		if nameTheSeeker {
			row.SeekerEmail = optString(b.SeekerEmail)
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
func (h *mentorshipHandlers) GetSession(ctx context.Context, params apigen.GetSessionParams) (*apigen.GetSessionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := bookingIDParam(params.ID)
	if err != nil {
		return nil, err
	}

	booking, err := h.mentorship.Session(ctx, id, userID)
	if err != nil {
		return nil, mentorshipError(err)
	}

	row := toBookingResponse(booking, time.Now())
	// The mentor sees who is coming; the seeker does not need their own address back.
	if userID == booking.MentorUserID {
		row.SeekerEmail = optString(booking.SeekerEmail)
	}
	return &apigen.GetSessionOK{Data: row}, nil
}

// CancelSession ends a session. Either party may; anybody else gets the same 404 a
// session that does not exist gets. A cancellation with no body is a cancellation with no
// reason, which is allowed.
func (h *mentorshipHandlers) CancelSession(ctx context.Context, req apigen.OptMentorshipCancelBody, params apigen.CancelSessionParams) (*apigen.CancelSessionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := bookingIDParam(params.ID)
	if err != nil {
		return nil, err
	}

	booking, err := h.mentorship.Cancel(ctx, id, userID, req.Value.Reason.Value)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.CancelSessionOK{Data: toBookingResponse(booking, time.Now())}, nil
}

// ReviewSession records the seeker's rating. PUT, not POST: there is one review per
// session and a second submission replaces the first, which is what PUT means.
func (h *mentorshipHandlers) ReviewSession(ctx context.Context, req *apigen.MentorshipReviewBody, params apigen.ReviewSessionParams) (*apigen.ReviewSessionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := bookingIDParam(params.ID)
	if err != nil {
		return nil, err
	}

	review, err := h.mentorship.Review(ctx, id, userID, int(req.Rating.Value), req.Comment.Value)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.ReviewSessionOK{Data: apigen.MentorshipReview{
		BookingID: review.BookingID.String(),
		Rating:    int64(review.Rating),
		Comment:   review.Comment,
	}}, nil
}

// profileInput is a mentor profile as submitted or edited. The session figures are
// minutes and days — the units a person types — and are converted once, here.
func profileInput(userID int64, r *apigen.MentorshipProfileBody) mentorship.ProfileInput {
	return mentorship.ProfileInput{
		UserID:      userID,
		CompanySlug: r.CompanySlug.Value,
		Slug:        r.Slug.Value,
		DisplayName: r.Name.Value,
		Headline:    r.Headline.Value,
		Bio:         r.Bio.Value,
		Topics:      r.Topics.Value,
		Languages:   r.Languages.Value,
		Timezone:    r.Timezone.Value,
		Session: slots.SessionParams{
			Duration:      time.Duration(r.SessionMinutes.Value) * time.Minute,
			BufferBefore:  time.Duration(r.BufferBeforeMinutes.Value) * time.Minute,
			BufferAfter:   time.Duration(r.BufferAfterMinutes.Value) * time.Minute,
			MinimumNotice: time.Duration(r.NoticeMinutes.Value) * time.Minute,
			Horizon:       time.Duration(r.HorizonDays.Value) * 24 * time.Hour,
		},
		MeetingURL: r.MeetingURL.Value,
		// ShowPhoto is the mentor's own opt-in to publish their account's CV headshot. Off
		// by default; see mentorship.Profile.ShowPhoto.
		ShowPhoto: r.ShowPhoto.Value,
		// Optional; see mentorship.Profile.Seniority for what an empty value means and what
		// a non-empty one must be.
		Seniority: r.Seniority.Value,
	}
}

// GetMyMentorProfile is the owner's own profile, whatever its status — a pending one must
// still be visible to the person who submitted it.
func (h *mentorshipHandlers) GetMyMentorProfile(ctx context.Context) (*apigen.GetMyMentorProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, found, err := h.mentorship.MyProfile(ctx, userID)
	if err != nil {
		return nil, mentorshipError(err)
	}
	if !found {
		return &apigen.GetMyMentorProfileOK{Data: apigen.NilMentorshipOwnMentor{Null: true}}, nil
	}
	return &apigen.GetMyMentorProfileOK{Data: apigen.NewNilMentorshipOwnMentor(toOwnMentorResponse(profile))}, nil
}

func (h *mentorshipHandlers) SubmitMentorProfile(ctx context.Context, req *apigen.MentorshipProfileBody) (*apigen.SubmitMentorProfileCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in, err := h.withCalendarLink(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	profile, err := h.mentorship.SubmitProfile(ctx, in)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.SubmitMentorProfileCreated{Data: toOwnMentorResponse(profile)}, nil
}

func (h *mentorshipHandlers) UpdateMentorProfile(ctx context.Context, req *apigen.MentorshipProfileBody) (*apigen.UpdateMentorProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in, err := h.withCalendarLink(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	profile, err := h.mentorship.UpdateProfile(ctx, in)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.UpdateMentorProfileOK{Data: toOwnMentorResponse(profile)}, nil
}

// withCalendarLink resolves whether the caller holds a connected calendar.events grant
// and sets it on the input, so a connected mentor's meeting link is no longer required —
// the same test CreateMeetEvent's own gate applies at booking time.
//
// Skipped when the submitted link is already non-empty: validateMeetingURL only ever
// consults HasCalendarLink for an EMPTY link, so a mentor who filled the field in pays
// no extra round trip to answer a question that cannot change their outcome.
func (h *mentorshipHandlers) withCalendarLink(ctx context.Context, userID int64, req *apigen.MentorshipProfileBody) (mentorship.ProfileInput, error) {
	in := profileInput(userID, req)
	if strings.TrimSpace(in.MeetingURL) != "" {
		return in, nil
	}
	hasCalendar, err := h.mentorship.HasConnectedCalendar(ctx, userID)
	if err != nil {
		return mentorship.ProfileInput{}, err
	}
	in.HasCalendarLink = hasCalendar
	return in, nil
}

// PauseMentorProfile flips the mentor's own switch. It needs no moderator and must not
// disturb what one decided.
func (h *mentorshipHandlers) PauseMentorProfile(ctx context.Context, req *apigen.MentorshipPauseBody) (*apigen.PauseMentorProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.mentorship.SetPaused(ctx, userID, req.Paused.Value)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.PauseMentorProfileOK{Data: toOwnMentorResponse(profile)}, nil
}

// WithdrawMentorProfile removes the profile, cancelling and announcing every future
// session first.
func (h *mentorshipHandlers) WithdrawMentorProfile(ctx context.Context) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	return mentorshipError(h.mentorship.Withdraw(ctx, userID))
}

// ReactivateMentorProfile resubmits a withdrawn profile for moderation. It is refused,
// not silently accepted, for a profile that was never withdrawn.
func (h *mentorshipHandlers) ReactivateMentorProfile(ctx context.Context) (*apigen.ReactivateMentorProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.mentorship.Reactivate(ctx, userID)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.ReactivateMentorProfileOK{Data: toOwnMentorResponse(profile)}, nil
}

// ListPendingMentorProfiles is the moderation queue.
func (h *mentorshipHandlers) ListPendingMentorProfiles(ctx context.Context) (*apigen.ListPendingMentorProfilesOK, error) {
	pending, err := h.mentorship.PendingQueue(ctx)
	if err != nil {
		return nil, mentorshipError(err)
	}

	out := make([]apigen.MentorshipPendingMentor, 0, len(pending))
	for _, p := range pending {
		m := toModeratorMentorResponse(p.Profile)
		out = append(out, apigen.MentorshipPendingMentor{
			Slug: m.Slug, Name: m.Name, CompanySlug: m.CompanySlug, CompanyName: m.CompanyName,
			Headline: m.Headline, Bio: m.Bio, Topics: m.Topics, Languages: m.Languages,
			Timezone: m.Timezone, SessionMinutes: m.SessionMinutes,
			RatingCount: m.RatingCount, RatingAvg: m.RatingAvg, ShowPhoto: m.ShowPhoto,
			Seniority: m.Seniority, Status: m.Status, Paused: m.Paused, CreatedAt: m.CreatedAt,
			ID: p.ID,
			// Evidence for the human deciding: this account is already an approved
			// referrer for the same company. Never a gate.
			HasApprovedReferralOffer: p.HasApprovedReferralOffer,
		})
	}
	return &apigen.ListPendingMentorProfilesOK{Data: out, Meta: apigen.MentorshipCountMeta{Count: int64(len(out))}}, nil
}

// DecideMentorProfile approves or rejects a pending profile.
func (h *mentorshipHandlers) DecideMentorProfile(ctx context.Context, req *apigen.MentorshipDecideBody, params apigen.DecideMentorProfileParams) (*apigen.DecideMentorProfileOK, error) {
	moderatorID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profileID, err := strconv.Atoi(params.ID)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "profile not found")
	}

	profile, err := h.mentorship.Decide(ctx, int64(profileID), moderatorID, req.Status.Value)
	if err != nil {
		return nil, mentorshipError(err)
	}
	return &apigen.DecideMentorProfileOK{Data: toModeratorMentorResponse(profile)}, nil
}
