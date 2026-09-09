package mentorship

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// validInput is a profile submission that should always be accepted, so each test can
// vary the one field it is about.
func validInput() ProfileInput {
	return ProfileInput{
		UserID:      7,
		CompanySlug: "acme",
		Slug:        "jane-doe",
		DisplayName: "Jane Doe",
		Headline:    "Senior Backend Engineer",
		Bio:         "Ten years of Go and Postgres.",
		Topics:      []string{"career", "system-design"},
		Languages:   []string{"en", "de"},
		Timezone:    "Europe/Berlin",
		Session: SessionParams{
			Duration:      time.Hour,
			MinimumNotice: 2 * time.Hour,
			Horizon:       30 * 24 * time.Hour,
		},
		MeetingURL: "https://meet.example.test/jane",
	}
}

func newTestService(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	return New(repo, Config{Now: func() time.Time {
		return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)
	}})
}

func TestSubmitProfileStartsPending(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)

	got, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("status = %q, want %q", got.Status, StatusPending)
	}
	if got.Paused {
		t.Error("a fresh profile is paused")
	}
}

// The moderation rule stated as a test, because it is the one an eager future change is
// most likely to "improve": an approved referral offer is evidence for a human, never a
// gate that approves anything.
func TestAnApprovedReferralOfferDoesNotApproveAProfile(t *testing.T) {
	repo := newFakeRepo()
	repo.approvedReferralOffers[referralKey{userID: 7, company: "acme"}] = true
	svc := newTestService(t, repo)

	got, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("status = %q, want %q — a referral offer is evidence, not approval", got.Status, StatusPending)
	}

	queue, err := svc.PendingQueue(context.Background())
	if err != nil {
		t.Fatalf("PendingQueue: %v", err)
	}
	if len(queue) != 1 || !queue[0].HasApprovedReferralOffer {
		t.Error("the moderation queue does not surface the corroborating referral offer")
	}
}

func TestSubmitProfileRefusesWhatCannotYieldASchedule(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ProfileInput)
		want   error
	}{
		{"no company", func(in *ProfileInput) { in.CompanySlug = "" }, ErrInvalidProfile},
		// A profile without a name is the anonymous referral offer with extra steps, and
		// this marketplace's whole premise is that a mentor is chosen.
		{"no name", func(in *ProfileInput) { in.DisplayName = "" }, ErrInvalidProfile},
		{"a name of only spaces", func(in *ProfileInput) { in.DisplayName = "   " }, ErrInvalidProfile},
		{"no headline", func(in *ProfileInput) { in.Headline = "" }, ErrInvalidProfile},
		{"a headline of only spaces", func(in *ProfileInput) { in.Headline = "   " }, ErrInvalidProfile},
		{"no topics", func(in *ProfileInput) { in.Topics = nil }, ErrInvalidProfile},
		{"no languages", func(in *ProfileInput) { in.Languages = nil }, ErrInvalidProfile},
		// A zone that does not resolve is not a cosmetic error: it is the ONLY thing that
		// gives the mentor's stored hours a meaning, so a profile carrying one has no
		// schedule at all.
		{"an unresolvable timezone", func(in *ProfileInput) { in.Timezone = "Mars/Olympus_Mons" }, ErrInvalidProfile},
		{"no timezone", func(in *ProfileInput) { in.Timezone = "" }, ErrInvalidProfile},
		// "Local" resolves in Go and means the SERVER's clock. A mentor whose hours are
		// interpreted in the server's zone is a mentor whose hours are wrong.
		{"a timezone of Local", func(in *ProfileInput) { in.Timezone = "Local" }, ErrInvalidProfile},
		{"a session that yields nothing", func(in *ProfileInput) { in.Session.Duration = 0 }, ErrInvalidSessionParams},
		{"no meeting link", func(in *ProfileInput) { in.MeetingURL = "" }, ErrInvalidProfile},
		{"a meeting link that is not a URL", func(in *ProfileInput) { in.MeetingURL = "not a url" }, ErrInvalidProfile},
		{"a meeting link that is not http", func(in *ProfileInput) { in.MeetingURL = "javascript:alert(1)" }, ErrInvalidProfile},
		// A non-empty link is validated identically whether or not the mentor has a
		// calendar — holding one is no excuse for a link that isn't a URL.
		{"an invalid meeting link even with a connected calendar", func(in *ProfileInput) {
			in.MeetingURL = "not a url"
			in.HasCalendarLink = true
		}, ErrInvalidProfile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)

			if _, err := newTestService(t, newFakeRepo()).SubmitProfile(context.Background(), in); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// A mentor with a connected calendar.events grant needs no static meeting link at all: a
// real one is minted per booking instead. A non-empty link is still validated normally
// either way — see the table test above.
func TestSubmitProfileAcceptsNoMeetingLinkWithAConnectedCalendar(t *testing.T) {
	in := validInput()
	in.MeetingURL = ""
	in.HasCalendarLink = true

	if _, err := newTestService(t, newFakeRepo()).SubmitProfile(context.Background(), in); err != nil {
		t.Errorf("SubmitProfile: %v, want no error — a connected calendar makes the static link optional", err)
	}
}

// The slug is in the URL, so its shape is the same one usernames carry — and it is
// COPIED rather than referenced, so a later username change cannot 404 every link
// somebody has already shared. An empty slug is NOT in this list: it is no longer a
// refusal, it is a request to derive one — see TestSubmitProfileWithoutASlugDerivesOneFromTheName.
func TestSubmitProfileRefusesASlugThatCannotBeAURL(t *testing.T) {
	for _, slug := range []string{"no", "Jane-Doe", "jane doe", "jane_doe", "-jane", "jane-", strings.Repeat("a", 31)} {
		t.Run("slug "+slug, func(t *testing.T) {
			in := validInput()
			in.Slug = slug

			if _, err := newTestService(t, newFakeRepo()).SubmitProfile(context.Background(), in); !errors.Is(err, ErrInvalidProfile) {
				t.Errorf("slug %q: error = %v, want ErrInvalidProfile", slug, err)
			}
		})
	}
}

func TestSubmitProfileReportsWhatTheDatabaseRefuses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setUp func(*fakeRepo)
		want  error
	}{
		{"a second profile for one account", func(r *fakeRepo) { r.createErr = ErrAlreadyAMentor }, ErrAlreadyAMentor},
		{"a slug somebody already has", func(r *fakeRepo) { r.createErr = ErrSlugTaken }, ErrSlugTaken},
		{"a company the catalogue does not carry", func(r *fakeRepo) { r.createErr = ErrCompanyNotFound }, ErrCompanyNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			tc.setUp(repo)

			if _, err := newTestService(t, repo).SubmitProfile(context.Background(), validInput()); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// An empty URL slug is no longer a refusal: the mentor left it to the system, and the
// system derives one from the display name — the field already required on this exact
// form, so nothing new needs to be typed.
func TestSubmitProfileWithoutASlugDerivesOneFromTheName(t *testing.T) {
	repo := newFakeRepo()
	in := validInput()
	in.Slug = ""

	got, err := newTestService(t, repo).SubmitProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if got.Slug != "jane-doe" {
		t.Errorf("slug = %q, want %q", got.Slug, "jane-doe")
	}
}

// A slug the system derived can collide with one another mentor already holds — the
// derivation only looks at the name, not the catalogue. The system SHALL resolve that
// itself with the smallest free numbered variant rather than refusing the submission.
func TestSubmitProfileWithoutASlugRetriesOnCollision(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.CreateProfile(context.Background(), ProfileInput{UserID: 1, Slug: "jane-doe"}); err != nil {
		t.Fatalf("seed CreateProfile: %v", err)
	}

	in := validInput()
	in.UserID = 7
	in.Slug = ""

	got, err := newTestService(t, repo).SubmitProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if got.Slug != "jane-doe-2" {
		t.Errorf("slug = %q, want %q", got.Slug, "jane-doe-2")
	}
}

// A display name with no latin letters or digits — an all-Cyrillic name, say — sanitizes
// to nothing. The system SHALL fall back to a fixed base rather than refuse the
// submission, exactly as username.Sanitize already falls back for an account username.
func TestSubmitProfileWithoutASlugFallsBackWhenTheNameSanitizesToNothing(t *testing.T) {
	repo := newFakeRepo()
	in := validInput()
	in.Slug = ""
	in.DisplayName = "Иван Стрелов"

	got, err := newTestService(t, repo).SubmitProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if got.Slug != "user" {
		t.Errorf("slug = %q, want %q", got.Slug, "user")
	}
}

// An explicitly supplied slug that is already taken is refused outright — never
// silently substituted with a suffixed variant. Substituting one would leave the
// mentor with a URL that differs from what they typed and no indication that happened.
func TestSubmitProfileWithAnExplicitTakenSlugIsRefusedWithoutRetrying(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.CreateProfile(context.Background(), ProfileInput{UserID: 1, Slug: "jane-doe"}); err != nil {
		t.Fatalf("seed CreateProfile: %v", err)
	}
	repo.createCalls = 0 // the seed call above doesn't count

	in := validInput()
	in.UserID = 7
	in.Slug = "jane-doe"

	_, err := newTestService(t, repo).SubmitProfile(context.Background(), in)
	if !errors.Is(err, ErrSlugTaken) {
		t.Errorf("error = %v, want ErrSlugTaken", err)
	}
	if repo.createCalls != 1 {
		t.Errorf("CreateProfile called %d times, want exactly 1 — an explicit slug must never be silently retried", repo.createCalls)
	}
}

func TestModerationRecordsWhoDecidedAndWhen(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	approved, err := svc.Decide(context.Background(), submitted.ID, 99, StatusApproved)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if approved.Status != StatusApproved {
		t.Errorf("status = %q, want approved", approved.Status)
	}

	t.Run("a second decision is refused", func(t *testing.T) {
		if _, err := svc.Decide(context.Background(), submitted.ID, 99, StatusRejected); !errors.Is(err, ErrProfileNotPending) {
			t.Errorf("error = %v, want ErrProfileNotPending", err)
		}
	})

	t.Run("only approved and rejected are decisions", func(t *testing.T) {
		for _, status := range []string{StatusPending, "paused", "", "APPROVED"} {
			if _, err := svc.Decide(context.Background(), submitted.ID, 99, status); !errors.Is(err, ErrInvalidProfile) {
				t.Errorf("status %q: error = %v, want ErrInvalidProfile", status, err)
			}
		}
	})
}

// Pausing is the mentor's own switch and needs no moderator; it must not disturb what the
// moderator decided, or resuming would mean re-entering the queue.
func TestPausingAndResumingNeedsNoModerator(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if _, err := svc.Decide(context.Background(), submitted.ID, 99, StatusApproved); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	paused, err := svc.SetPaused(context.Background(), validInput().UserID, true)
	if err != nil {
		t.Fatalf("SetPaused(true): %v", err)
	}
	if !paused.Paused || paused.Status != StatusApproved {
		t.Errorf("paused=%v status=%q, want paused with the moderator's decision intact",
			paused.Paused, paused.Status)
	}

	resumed, err := svc.SetPaused(context.Background(), validInput().UserID, false)
	if err != nil {
		t.Fatalf("SetPaused(false): %v", err)
	}
	if resumed.Paused || resumed.Status != StatusApproved {
		t.Errorf("paused=%v status=%q, want unpaused and still approved", resumed.Paused, resumed.Status)
	}
}

func TestOnlyTheOwnerMayEditOrPause(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	const stranger = 4242

	if _, err := svc.SetPaused(context.Background(), stranger, true); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("SetPaused by a stranger: error = %v, want ErrProfileNotFound", err)
	}

	in := validInput()
	in.UserID = stranger
	if _, err := svc.UpdateProfile(context.Background(), in); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("UpdateProfile by a stranger: error = %v, want ErrProfileNotFound", err)
	}
}

// Editing must not send an approved mentor back to the queue: rewording a headline is not
// a new claim about where somebody works.
func TestEditingDoesNotResetModeration(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if _, err := svc.Decide(context.Background(), submitted.ID, 99, StatusApproved); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	in := validInput()
	in.Headline = "Staff Backend Engineer"
	updated, err := svc.UpdateProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Status != StatusApproved {
		t.Errorf("status = %q after an edit, want approved", updated.Status)
	}
	if updated.Headline != "Staff Backend Engineer" {
		t.Errorf("headline = %q, want the new one", updated.Headline)
	}
}

// The slug is stable by design: the URL somebody shared must keep working.
func TestEditingCannotChangeTheSlugOrTheCompany(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	in := validInput()
	in.Slug = "someone-else"
	in.CompanySlug = "othercorp"
	updated, err := svc.UpdateProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Slug != "jane-doe" {
		t.Errorf("slug = %q, want the original — a shared link must not 404", updated.Slug)
	}
	if updated.CompanySlug != "acme" {
		t.Errorf("company = %q, want the original — changing employer is a new profile, "+
			"and a new moderation decision", updated.CompanySlug)
	}
}

func TestWithdrawalCancelsFutureBookingsAndNotifiesEachSeeker(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	svc := New(repo, Config{
		Notifier: notifier,
		Now:      func() time.Time { return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC) },
	})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{
		{ID: uuid.New(), SeekerUserID: 11},
		{ID: uuid.New(), SeekerUserID: 12},
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if len(notifier.cancelled) != 2 {
		t.Errorf("notified %d seekers, want 2 — a session cancelled without telling anybody "+
			"is worse than one not cancelled", len(notifier.cancelled))
	}
	if !repo.withdrawn {
		t.Error("the profile was not withdrawn")
	}
	if repo.cancelledBeforeDelete != 2 {
		t.Errorf("%d bookings were cancelled before the delete, want 2 — the ON DELETE "+
			"CASCADE would otherwise take them and nobody would ever be told",
			repo.cancelledBeforeDelete)
	}
}

// A withdrawing mentor's cancelled bookings must not leave stray Meet events live on
// their own calendar — the same cleanup a single Cancel() already does, extended to the
// bulk path Withdraw uses.
func TestWithdrawalDeletesCalendarEventsOfCancelledBookings(t *testing.T) {
	repo := newFakeRepo()
	linker := &fakeCalendarLinker{}
	svc := New(repo, Config{
		Notifier:       &fakeNotifier{},
		CalendarLinker: linker,
		Now:            func() time.Time { return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC) },
	})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{
		{ID: uuid.New(), SeekerUserID: 11, MentorUserID: submitted.UserID, GoogleEventID: "evt-1"},
		// No calendar event to clean up — the ordinary static-link case.
		{ID: uuid.New(), SeekerUserID: 12, MentorUserID: submitted.UserID},
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if linker.deletedEventID != "evt-1" {
		t.Errorf("DeleteMeetEvent called with %q, want evt-1", linker.deletedEventID)
	}
}

// A delivery failure must not leave a mentor unable to withdraw. The cancellations have
// already committed; refusing here would strand them.
func TestWithdrawalSurvivesAFailedNotification(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{err: errors.New("smtp is down")}
	svc := New(repo, Config{Notifier: notifier, Now: time.Now})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{{ID: uuid.New(), SeekerUserID: 11}}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Errorf("Withdraw: %v — a delivery failure must not block a withdrawal", err)
	}
	if !repo.withdrawn {
		t.Error("the profile was not withdrawn after a failed notification")
	}
}

func TestWithdrawalByAStrangerRemovesNothing(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if err := svc.Withdraw(context.Background(), 4242); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
	if repo.withdrawn {
		t.Error("a stranger withdrew somebody else's profile")
	}
}

// Withdrawal must not touch the mentor's own pause switch — status and pause are
// independent decisions, and folding them together is what made a withdrawn profile
// show up in the UI labelled "paused".
func TestWithdrawalDoesNotPauseTheProfile(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	profile, _, err := svc.MyProfile(context.Background(), validInput().UserID)
	if err != nil {
		t.Fatalf("MyProfile: %v", err)
	}
	if profile.Status != StatusWithdrawn {
		t.Errorf("status = %q, want %q", profile.Status, StatusWithdrawn)
	}
	if profile.Paused {
		t.Error("withdrawing paused the profile — status and pause must stay independent")
	}
}

// A second withdrawal changes nothing, per the spec — it must succeed, not 404 as
// though the profile had vanished.
func TestASecondWithdrawalSucceeds(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("first Withdraw: %v", err)
	}
	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Errorf("second Withdraw: %v, want success — a repeat withdrawal is a no-op, not a failure", err)
	}
}

// Reactivating a withdrawn profile sends it back into the ordinary moderation queue —
// exactly the pending state a first submission starts in, with no special treatment for
// having been a mentor before.
func TestReactivateReturnsAWithdrawnProfileToPending(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if _, err := svc.Decide(context.Background(), submitted.ID, 99, StatusApproved); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	reactivated, err := svc.Reactivate(context.Background(), validInput().UserID)
	if err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if reactivated.Status != StatusPending {
		t.Errorf("status = %q, want %q", reactivated.Status, StatusPending)
	}
	if reactivated.Paused {
		t.Error("a reactivated profile is paused")
	}
}

// Resubmitting is not a way to force a pending or approved profile back to review out
// of turn — only a withdrawn one may be reactivated.
func TestReactivateRefusesAProfileThatWasNeverWithdrawn(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if _, err := svc.Reactivate(context.Background(), validInput().UserID); !errors.Is(err, ErrProfileNotWithdrawn) {
		t.Errorf("error = %v, want ErrProfileNotWithdrawn", err)
	}
}

func TestReactivateWithNoProfileIsRefused(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)

	if _, err := svc.Reactivate(context.Background(), 4242); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
}

func TestReactivateByAStrangerIsRefused(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if _, err := svc.Reactivate(context.Background(), 4242); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
}

// A moderator reads a profile by row id regardless of its status — the queue lists
// pending rows, but the lookup itself carries no status guard: the caller (an id from a
// moderator's own queue read) already establishes the context.
func TestProfileForModerationReadsAnyStatus(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	got, err := svc.ProfileForModeration(context.Background(), submitted.ID)
	if err != nil {
		t.Fatalf("ProfileForModeration: %v", err)
	}
	if got.ID != submitted.ID || got.Status != StatusPending {
		t.Errorf("got id=%d status=%q, want id=%d status=%q", got.ID, got.Status, submitted.ID, StatusPending)
	}
}

func TestProfileForModerationRefusesAnUnknownID(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)

	if _, err := svc.ProfileForModeration(context.Background(), 999999); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
}

// ShowPhoto defaults off and round-trips through both create and update, independent
// of every other field — a mentor's own opt-in, not a byproduct of anything else they
// submit.
func TestShowPhotoDefaultsOffAndRoundTripsThroughCreateAndUpdate(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)

	created, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if created.ShowPhoto {
		t.Error("show_photo = true on a fresh profile, want false (off by default)")
	}

	in := validInput()
	in.ShowPhoto = true
	updated, err := svc.UpdateProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if !updated.ShowPhoto {
		t.Error("show_photo = false after opting in, want true")
	}

	in.ShowPhoto = false
	updated, err = svc.UpdateProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.ShowPhoto {
		t.Error("show_photo = true after opting back out, want false")
	}
}

// A profile with no object storage and no notifier configured must still work: the
// feature ships before every channel exists, exactly as the referral pings do.
func TestAServiceWithNoNotifierStillWorks(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo, Config{Now: time.Now})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{{ID: uuid.New(), SeekerUserID: 11}}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Errorf("Withdraw with no notifier: %v", err)
	}
}
