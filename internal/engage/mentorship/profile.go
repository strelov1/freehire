package mentorship

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/identity/username"
)

// The moderation statuses a profile can hold. Pausing is not one: it is the mentor's own
// switch in its own field, because status records what a moderator decided.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	// StatusWithdrawn is a mentor who has left. The row survives: bookings and reviews
	// cascade-delete with it, and a session belongs to both people in it. Reactivate sends
	// it back to StatusPending.
	StatusWithdrawn = "withdrawn"
)

// The sentinels the profile use cases raise, each with the HTTP status the handler maps it
// to (the convention of internal/engage/referral).
var (
	// ErrInvalidProfile → 422. A submission that could never yield a working profile.
	ErrInvalidProfile = errors.New("mentorship: invalid profile")
	// ErrProfileNotFound → 404. No profile, or somebody else's: deliberately the same
	// answer.
	ErrProfileNotFound = errors.New("mentorship: profile not found")
	// ErrAlreadyAMentor → 409. One profile per account.
	ErrAlreadyAMentor = errors.New("mentorship: this account already has a mentor profile")
	// ErrSlugTaken → 409. The public URL is somebody else's.
	ErrSlugTaken = errors.New("mentorship: that profile address is taken")
	// ErrCompanyNotFound → 404. A company the catalogue does not carry.
	ErrCompanyNotFound = errors.New("mentorship: company not found")
	// ErrProfileNotPending → 409. A decision on a profile already decided.
	ErrProfileNotPending = errors.New("mentorship: profile is not pending")
	// ErrProfileNotWithdrawn → 409. Resubmission is only for a withdrawn profile; other
	// statuses are refused, not treated as a no-op.
	ErrProfileNotWithdrawn = errors.New("mentorship: profile is not withdrawn")
)

// slugPattern is the shape of a profile's public address, identical to users.username:
// lowercase, digits, single hyphens between them, 3–30 characters.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Profile is a mentor as the product talks about one.
type Profile struct {
	ID          int64
	UserID      int64
	CompanySlug string
	CompanyName string
	// Slug is the profile's public address, fixed at creation (explicit, or derived from
	// DisplayName) because changing it would 404 every shared link.
	Slug string
	// DisplayName is the name the public sees. Required: a mentor is named, unlike a
	// referral.
	DisplayName string
	Headline    string
	Bio         string
	Topics      []string
	Languages   []string
	Timezone    string
	Session     slots.SessionParams
	MeetingURL  string
	// ShowPhoto is the mentor's opt-in to serve their stored CV headshot on their public
	// card and profile. Off by default: it is a job-search photo.
	ShowPhoto bool
	// Seniority is optional and, when set, one of vocab.SeniorityValues. Empty means unset,
	// never a value of its own.
	Seniority   string
	Status      string
	Paused      bool
	DecidedBy   int64
	RatingCount int64
	RatingAvg   float64
	CreatedAt   time.Time
}

// There is deliberately no Published() helper: the publication predicate lives in the SQL,
// shared by every reader, and a Go copy could drift.

// PendingProfile is a queue entry: the profile plus what the moderator needs beside it.
type PendingProfile struct {
	Profile
	// HasApprovedReferralOffer says this account is already an approved referrer for the
	// same company. Evidence for the moderator, never a gate; a test holds that.
	HasApprovedReferralOffer bool
}

// ProfileInput is a submission or an edit. On an edit the slug and the company are
// ignored: the slug must stay stable, and a new company is a new claim needing a new
// decision.
type ProfileInput struct {
	UserID      int64
	CompanySlug string
	// Slug is the requested public address. On a submission an empty Slug is not a refusal:
	// SubmitProfile derives one from DisplayName. Edits ignore it.
	Slug        string
	DisplayName string
	Headline    string
	Bio         string
	Topics      []string
	Languages   []string
	Timezone    string
	Session     slots.SessionParams
	MeetingURL  string
	ShowPhoto   bool
	// Seniority is optional; see Profile.Seniority for what an empty value means.
	Seniority string
	// HasCalendarLink says the caller holds a connected calendar.events grant at submission
	// time, resolved by the handler. Such a mentor gets a Meet link per booking, so
	// meeting_url may be empty (see validateMeetingURL).
	HasCalendarLink bool
}

// DirectoryFilter narrows the public directory. An empty field means unfiltered, matching
// the SQL's "NULL means unfiltered".
type DirectoryFilter struct {
	CompanySlug string
	Topic       string
	Language    string
	Seniority   string
	// Query narrows by a case-insensitive substring match against a mentor's name or
	// headline. Empty means unfiltered, same as every other field here.
	Query string
	// NoReviewsOnly narrows to mentors with zero reviews. false means unfiltered.
	NoReviewsOnly bool
	Limit         int32
}

// maxSlugAttempts bounds SubmitProfile's collision-suffix search over a derived slug so a
// run of taken addresses fails instead of looping. Mirrors accounts.maxUsernameAttempts.
const maxSlugAttempts = 100

// SubmitProfile records a mentor profile, awaiting moderation. Anything that would stop it
// working (an unresolvable zone, a session yielding no slot, a bad meeting link) is
// refused here rather than at the first booking.
//
// An empty Slug is derived from DisplayName with the account-username sanitisation. Only
// a derived slug gets a numeric-suffix retry on collision; retrying one the mentor typed
// would hand them a URL that differs from their choice.
func (s *Service) SubmitProfile(ctx context.Context, in ProfileInput) (Profile, error) {
	derived := strings.TrimSpace(in.Slug) == ""
	if derived {
		// Hyphen-joined before sanitizing, since username.Sanitize's usual inputs have no
		// spaces: "Jane Doe" should become "jane-doe", not "janedoe".
		in.Slug = username.Sanitize(strings.Join(strings.Fields(in.DisplayName), "-"))
	}
	if err := validateProfile(in, true); err != nil {
		return Profile{}, err
	}
	if !derived {
		return s.repo.CreateProfile(ctx, normaliseProfile(in))
	}

	base := in.Slug
	for attempt := 1; attempt <= maxSlugAttempts; attempt++ {
		candidate := in
		candidate.Slug = username.Candidate(base, attempt)
		profile, err := s.repo.CreateProfile(ctx, normaliseProfile(candidate))
		if errors.Is(err, ErrSlugTaken) {
			continue
		}
		return profile, err
	}
	return Profile{}, ErrSlugTaken
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

// ProfileForModeration reads a profile by its row id, whatever its status. It carries no
// access check of its own — every caller sits behind the moderator gate already, the
// same division of labour Decide relies on for the same id.
func (s *Service) ProfileForModeration(ctx context.Context, id int64) (Profile, error) {
	profile, found, err := s.repo.ProfileByID(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

// Directory is the public list of mentors.
func (s *Service) Directory(ctx context.Context, f DirectoryFilter) ([]Profile, error) {
	if f.Limit <= 0 || f.Limit > maxDirectoryPage {
		f.Limit = defaultDirectoryPage
	}
	return s.repo.ListPublishedProfiles(ctx, f)
}

// There is deliberately no "does this company have a mentor?" call: Directory answers it
// (filter by company, take one), and a second query would duplicate the publication
// predicate.

// Withdraw takes a mentor off the marketplace.
//
// It marks the profile withdrawn rather than deleting it: bookings and reviews cascade, and
// past sessions must be retained as history.
//
// Order is load-bearing, as in referral's withdrawal: cancel future bookings and tell
// their seekers first.
//
// Notification failure does not fail the withdrawal: the cancellations have already
// committed.
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
	// Best-effort, as in Cancel(): a cancelled session's Meet event must not stay live on
	// the mentor's calendar.
	for _, booking := range cancelled {
		s.deleteMeetEventBestEffort(ctx, booking)
	}

	return s.repo.WithdrawProfile(ctx, userID)
}

// Reactivate resubmits a withdrawn profile for moderation, moving it back to `pending` and
// clearing the pause switch. Any other status gets ErrProfileNotWithdrawn.
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
	// Optional: empty is valid; only a non-empty value outside the seniority vocabulary is
	// refused. Trimmed first so a non-frontend caller is not refused for whitespace.
	if seniority := strings.TrimSpace(in.Seniority); seniority != "" && !slices.Contains(vocab.SeniorityValues, seniority) {
		return fmt.Errorf("%w: %q is not a recognised seniority level", ErrInvalidProfile, in.Seniority)
	}
	if err := validateMeetingURL(in.MeetingURL, in.HasCalendarLink); err != nil {
		return err
	}
	if err := validateMentorZone(in.Timezone); err != nil {
		return err
	}
	if err := in.Session.Validate(); err != nil {
		return err
	}
	if creating {
		if len(in.Slug) < 3 || len(in.Slug) > 30 || !slugPattern.MatchString(in.Slug) {
			return fmt.Errorf("%w: %q is not a usable profile address", ErrInvalidProfile, in.Slug)
		}
	}
	return nil
}

// validateMentorZone refuses a zone the slot engine could not use. The shape check matters:
// time.LoadLocation accepts "Local" (the server's zone, silently wrong for every slot),
// "EST" and "Factory". Requiring a slash, or exactly "UTC", admits only IANA names.
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

// validateMeetingURL shape-checks the link like referral's LinkedIn check: http(s) and
// parseable, never fetched. The scheme check matters: a javascript: URL on a public
// profile is one click from executing.
//
// An empty link is refused unless the mentor holds a connected calendar.events grant (a
// link is then minted per booking). A non-empty link is validated either way.
func validateMeetingURL(raw string, hasCalendarLink bool) error {
	if strings.TrimSpace(raw) == "" {
		if hasCalendarLink {
			return nil
		}
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
	in.Seniority = strings.TrimSpace(in.Seniority)
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
