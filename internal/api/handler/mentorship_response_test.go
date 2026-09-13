package handler

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/mentorship"
)

// No dedicated test exercised profileRequest.toInput's field mapping at all before this
// change — this is the one assertion that would have caught Seniority (or any future
// field) landing in the request struct but never reaching mentorship.ProfileInput.
func TestProfileRequestToInputCarriesSeniority(t *testing.T) {
	req := profileRequest{Seniority: "staff"}
	if got := req.toInput(7).Seniority; got != "staff" {
		t.Errorf("toInput(...).Seniority = %q, want staff", got)
	}
}

// A moderator (and the owner) can see when a profile was submitted; the public response
// never carries it — nothing outside the cabinet/queue needs it, and it is not part of
// what makes a mentor look trustworthy on the public card.
func TestCreatedAtIsModeratorAndOwnerOnly(t *testing.T) {
	submitted := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	p := mentorship.Profile{CreatedAt: submitted}

	if got := toMentorResponse(p).CreatedAt; got != nil {
		t.Errorf("public response carries created_at = %v, want nil", got)
	}
	if got := toModeratorMentorResponse(p).CreatedAt; got == nil || !got.Equal(submitted) {
		t.Errorf("moderator response created_at = %v, want %v", got, submitted)
	}
	if got := toOwnMentorResponse(p).CreatedAt; got == nil || !got.Equal(submitted) {
		t.Errorf("owner response created_at = %v, want %v", got, submitted)
	}
}

// Seniority is on the base public response, unlike created_at above — a seeker deciding
// whether to book reads it on the card, so every view (public, moderator, owner) built on
// top of toMentorResponse must carry it, not just the owner/moderator ones.
func TestSeniorityIsOnEveryView(t *testing.T) {
	p := mentorship.Profile{Seniority: "senior"}

	if got := toMentorResponse(p).Seniority; got != "senior" {
		t.Errorf("public response seniority = %q, want senior", got)
	}
	if got := toModeratorMentorResponse(p).Seniority; got != "senior" {
		t.Errorf("moderator response seniority = %q, want senior", got)
	}
	if got := toOwnMentorResponse(p).Seniority; got != "senior" {
		t.Errorf("owner response seniority = %q, want senior", got)
	}
}

// A company-less mentor's response carries empty company fields, not a placeholder —
// the domain type already reads an unset company as "" (mentorship.Profile.CompanySlug),
// and the response struct is a plain, non-omitempty string that must pass that through
// on the wire unchanged, on every view built on toMentorResponse.
func TestCompanyFieldsAreEmptyForACompanyLessMentor(t *testing.T) {
	p := mentorship.Profile{CompanySlug: "", CompanyName: ""}

	resp := toMentorResponse(p)
	if resp.CompanySlug != "" {
		t.Errorf("public response company_slug = %q, want empty", resp.CompanySlug)
	}
	if resp.CompanyName != "" {
		t.Errorf("public response company_name = %q, want empty", resp.CompanyName)
	}
}

// The three new directory filters must be part of the endpoint's known vocabulary —
// otherwise they would silently narrow the answer while also being reported as ignored,
// which is the confusing direction this file's own comment on knownMentorParams warns
// about.
func TestNewDirectoryFiltersAreKnownParams(t *testing.T) {
	query := map[string][]string{"q": {"jane"}, "seniority": {"senior"}, "no_reviews": {"1"}}
	if ignored := unknownMentorParams(query); len(ignored) != 0 {
		t.Errorf("unknownMentorParams(%v) = %v, want none", query, ignored)
	}
}
