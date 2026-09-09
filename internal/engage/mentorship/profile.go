package mentorship

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The moderation statuses a profile can hold. Pausing is NOT one of them: it is the
// mentor's own switch and lives in its own field, because status records what a MODERATOR
// decided. Folded together, a mentor resuming would have to remember the moderator's
// decision and a moderator acting on a paused profile would silently unpause it.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	// StatusWithdrawn is a mentor who has left. The row SURVIVES it: bookings and reviews
	// reference the profile ON DELETE CASCADE, so deleting would erase every session that
	// ever happened — and a session belongs to both people who were in it, not to whether
	// the mentor is still here. Not permanent: Reactivate sends it back to StatusPending.
	StatusWithdrawn = "withdrawn"
)

// The sentinels the profile use cases raise, each carrying the HTTP status the handler's
// one error switch maps it to — the convention internal/engage/referral established.
var (
	// ErrInvalidProfile → 422. A submission that could never yield a working profile.
	ErrInvalidProfile = errors.New("mentorship: invalid profile")
	// ErrProfileNotFound → 404. No profile, or one belonging to somebody else — the two
	// are deliberately the same answer, so an endpoint never confirms that a profile
	// exists to a caller who does not own it.
	ErrProfileNotFound = errors.New("mentorship: profile not found")
	// ErrAlreadyAMentor → 409. One profile per account.
	ErrAlreadyAMentor = errors.New("mentorship: this account already has a mentor profile")
	// ErrSlugTaken → 409. The public URL is somebody else's.
	ErrSlugTaken = errors.New("mentorship: that profile address is taken")
	// ErrCompanyNotFound → 404. A company the catalogue does not carry.
	ErrCompanyNotFound = errors.New("mentorship: company not found")
	// ErrProfileNotPending → 409. A decision on a profile already decided.
	ErrProfileNotPending = errors.New("mentorship: profile is not pending")
	// ErrProfileNotWithdrawn → 409. Resubmission is only for a withdrawn profile —
	// forcing a pending, rejected or approved one back to review out of turn is refused
	// rather than treated as a no-op, symmetric with ErrProfileNotPending.
	ErrProfileNotWithdrawn = errors.New("mentorship: profile is not withdrawn")
)

// slugPattern is the shape of a profile's public address, deliberately identical to the
// one users.username carries (migration 0128): lowercase, digits, single hyphens between
// them, 3–30 characters.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Profile is a mentor as the product talks about one.
type Profile struct {
	ID          int64
	UserID      int64
	CompanySlug string
	CompanyName string
	// Slug is the profile's public address. It is COPIED from the account's username at
	// creation rather than following it, because a username change would otherwise 404
	// every link anybody has shared.
	Slug string
	// DisplayName is the name the public sees. Required, and the reason this profile is
	// not a referral offer: a referral is anonymous by design, a mentor is chosen.
	DisplayName string
	Headline    string
	Bio         string
	Topics      []string
	Languages   []string
	Timezone    string
	Session     SessionParams
	MeetingURL  string
	Status      string
	Paused      bool
	DecidedBy   int64
	RatingCount int64
	RatingAvg   float64
	CreatedAt   time.Time
}

// There is no Published() helper on this type, and that is deliberate. The publication
// predicate lives in the SQL, where the directory, the public read and every other reader
// share one copy of it — a Go method saying the same thing would be a second copy, free to
// drift, and the first caller to reach for it would be a reader that should have been a
// query.

// PendingProfile is a queue entry: the profile plus what the moderator needs beside it.
type PendingProfile struct {
	Profile
	// HasApprovedReferralOffer says this account is already an approved referrer for the
	// same company. It is EVIDENCE for the human deciding and never a gate — nothing in
	// this package approves a profile automatically, and a test holds that.
	HasApprovedReferralOffer bool
}

// ProfileInput is a submission or an edit. On an edit the slug and the company are
// ignored: the first must stay stable, and changing the second is a new claim about where
// somebody works, which is a new profile and a new decision.
type ProfileInput struct {
	UserID      int64
	CompanySlug string
	Slug        string
	DisplayName string
	Headline    string
	Bio         string
	Topics      []string
	Languages   []string
	Timezone    string
	Session     SessionParams
	MeetingURL  string
}

// DirectoryFilter narrows the public directory. An empty field means unfiltered, matching
// the SQL's "NULL means unfiltered" — a filter that meant "match nothing" when absent
// would empty the directory.
type DirectoryFilter struct {
	CompanySlug string
	Topic       string
	Language    string
	Limit       int32
}

// SubmitProfile records a mentor profile, awaiting moderation. Everything that could stop
// it working is refused here rather than at the first booking: a zone that does not
// resolve has no schedule, a session that cannot yield a slot has no availability, and a
// meeting link that is not a link has no meeting.
func (s *Service) SubmitProfile(ctx context.Context, in ProfileInput) (Profile, error) {
	if err := validateProfile(in, true); err != nil {
		return Profile{}, err
	}
	return s.repo.CreateProfile(ctx, normaliseProfile(in))
}

// MyProfile is the owner's own profile whatever its status — a pending or rejected one
// must still be visible to the person who submitted it.
func (s *Service) MyProfile(ctx context.Context, userID int64) (Profile, bool, error) {
	return s.repo.ProfileByUser(ctx, userID)
}

// PublicProfile is the profile a visitor sees. A profile that is not published answers as
// though it does not exist.
func (s *Service) PublicProfile(ctx context.Context, slug string) (Profile, error) {
	profile, found, err := s.repo.PublishedProfileBySlug(ctx, slug)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

// UpdateProfile edits the owner's own profile. It does NOT reset moderation: rewording a
// headline is not a new claim about where somebody works.
func (s *Service) UpdateProfile(ctx context.Context, in ProfileInput) (Profile, error) {
	if err := validateProfile(in, false); err != nil {
		return Profile{}, err
	}
	return s.repo.UpdateProfile(ctx, normaliseProfile(in))
}

// SetPaused flips the mentor's own switch, leaving the moderator's decision alone.
func (s *Service) SetPaused(ctx context.Context, userID int64, paused bool) (Profile, error) {
	return s.repo.SetPaused(ctx, userID, paused)
}

// Decide approves or rejects a pending profile. Only those two are decisions — anything
// else is a caller confusing moderation with the mentor's own pause switch.
func (s *Service) Decide(ctx context.Context, profileID, moderatorID int64, status string) (Profile, error) {
	if status != StatusApproved && status != StatusRejected {
		return Profile{}, fmt.Errorf("%w: %q is not a moderation decision", ErrInvalidProfile, status)
	}
	return s.repo.DecideProfile(ctx, profileID, moderatorID, status)
}

// PendingQueue is the moderation queue, oldest first.
func (s *Service) PendingQueue(ctx context.Context) ([]PendingProfile, error) {
	return s.repo.ListPendingProfiles(ctx)
}

// Directory is the public list of mentors.
func (s *Service) Directory(ctx context.Context, f DirectoryFilter) ([]Profile, error) {
	if f.Limit <= 0 || f.Limit > maxDirectoryPage {
		f.Limit = defaultDirectoryPage
	}
	return s.repo.ListPublishedProfiles(ctx, f)
}

// There is deliberately NO "does this company have a mentor?" call here.
//
// A vacancy page asking that is asking the directory a narrower question, and Directory
// already answers it: filter by company, take one, look at the count. Adding a second way
// to ask meant a second copy of the publication predicate — and the integration test that
// existed to check the two agreed was the sign that they could stop agreeing. The
// dead-code guard found the method unreachable before a caller was written, which was the
// moment to remove it rather than find it a caller.

// Withdraw takes a mentor off the marketplace.
//
// It MARKS the profile withdrawn rather than deleting it. Bookings and reviews reference
// the profile row ON DELETE CASCADE, so a delete would erase every session that ever
// happened — and the spec requires the opposite: "SHALL retain past bookings as history
// rather than deleting them". A session that took place belongs to both people who were
// in it, not to whether the mentor is still here.
//
// The order is still load-bearing, and is the same shape referral's withdrawal uses:
// cancel the future bookings and tell their seekers FIRST. A seeker who learns their
// session is off from an empty calendar has been told by nobody.
//
// Notification failure does not fail the withdrawal. By the time it could, the
// cancellations have already committed; refusing here would strand a mentor who cannot
// leave and sessions that are already cancelled.
func (s *Service) Withdraw(ctx context.Context, userID int64) error {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return err
	}

	cancelled, err := s.repo.CancelFutureBookings(ctx, profile.ID, userID, reasonMentorWithdrew)
	if err != nil {
		return err
	}
	s.notifyCancelled(ctx, cancelled, CancelledByMentor, reasonMentorWithdrew)

	return s.repo.WithdrawProfile(ctx, userID)
}

// Reactivate resubmits a withdrawn profile for moderation, moving it back to `pending`
// and clearing the pause switch. It is not a way to force review out of turn: a profile
// that is pending, rejected or approved was never withdrawn, and ErrProfileNotWithdrawn
// refuses those rather than silently doing nothing.
func (s *Service) Reactivate(ctx context.Context, userID int64) (Profile, error) {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	if profile.Status != StatusWithdrawn {
		return Profile{}, ErrProfileNotWithdrawn
	}
	return s.repo.ReactivateProfile(ctx, userID)
}

// validateProfile checks what the database cannot. `creating` gates the fields an edit
// does not carry.
func validateProfile(in ProfileInput, creating bool) error {
	if strings.TrimSpace(in.DisplayName) == "" {
		return fmt.Errorf("%w: a name is required", ErrInvalidProfile)
	}
	if strings.TrimSpace(in.Headline) == "" {
		return fmt.Errorf("%w: a headline is required", ErrInvalidProfile)
	}
	if len(in.Topics) == 0 {
		return fmt.Errorf("%w: at least one topic is required", ErrInvalidProfile)
	}
	if len(in.Languages) == 0 {
		return fmt.Errorf("%w: at least one language is required", ErrInvalidProfile)
	}
	if err := validateMeetingURL(in.MeetingURL); err != nil {
		return err
	}
	if err := validateMentorZone(in.Timezone); err != nil {
		return err
	}
	if err := in.Session.Validate(); err != nil {
		return err
	}
	if creating {
		if strings.TrimSpace(in.CompanySlug) == "" {
			return fmt.Errorf("%w: a company is required", ErrInvalidProfile)
		}
		if len(in.Slug) < 3 || len(in.Slug) > 30 || !slugPattern.MatchString(in.Slug) {
			return fmt.Errorf("%w: %q is not a usable profile address", ErrInvalidProfile, in.Slug)
		}
	}
	return nil
}

// validateMentorZone refuses a zone the slot engine could not use.
//
// The shape check is what does the work, and it is not decoration: time.LoadLocation
// accepts several names that are not a person's zone. "Local" returns the SERVER's zone,
// so a mentor's stated hours would be read in the host's clock and every slot would be
// wrong in a way nothing in the response reveals. "EST" and "Factory" resolve too, and
// mean something other than a place. Requiring a slash — or exactly "UTC" — admits the
// IANA names and nothing else.
func validateMentorZone(name string) error {
	if name == "" {
		return fmt.Errorf("%w: a timezone is required", ErrInvalidProfile)
	}
	if name != "UTC" && !strings.Contains(name, "/") {
		return fmt.Errorf("%w: %q is not an IANA timezone", ErrInvalidProfile, name)
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("%w: timezone %q does not resolve", ErrInvalidProfile, name)
	}
	return nil
}

// validateMeetingURL shape-checks the link, in the manner of referral's LinkedIn check:
// http(s) and parseable, never fetched. A scheme check specifically — a javascript: URL
// rendered as a link on a public profile is a click away from being executed.
func validateMeetingURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%w: a meeting link is required", ErrInvalidProfile)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: the meeting link is not a URL", ErrInvalidProfile)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: the meeting link must be http or https", ErrInvalidProfile)
	}
	return nil
}

// normaliseProfile trims what a form leaves ragged, so two profiles that differ only in
// whitespace are the same profile.
func normaliseProfile(in ProfileInput) ProfileInput {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Headline = strings.TrimSpace(in.Headline)
	in.Bio = strings.TrimSpace(in.Bio)
	in.MeetingURL = strings.TrimSpace(in.MeetingURL)
	in.Topics = normaliseTags(in.Topics)
	in.Languages = normaliseTags(in.Languages)
	return in
}

// normaliseTags lowercases, trims and de-duplicates a tag list while keeping its order,
// so "Career", "career " and "career" are one topic rather than three facets.
func normaliseTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}
