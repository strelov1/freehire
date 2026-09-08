package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// fakeSuggestionResume is a mentorSuggestionResume returning a canned StructureForSeed
// result, so the composition can be exercised without a database.
type fakeSuggestionResume struct {
	structured resumeextract.Structured
	structOK   bool
	structErr  error
}

func (f fakeSuggestionResume) StructureForSeed(context.Context, int64) (resumeextract.Structured, bool, error) {
	return f.structured, f.structOK, f.structErr
}

type fakeSuggestionUserProfile struct {
	profile userprofile.Profile
	err     error
}

func (f fakeSuggestionUserProfile) Get(context.Context, int64) (userprofile.Profile, error) {
	return f.profile, f.err
}

type fakeSuggestionAccount struct {
	user accounts.User
	err  error
}

func (f fakeSuggestionAccount) UserByID(context.Context, int64) (accounts.User, error) {
	return f.user, f.err
}

type fakeSuggestionExperience struct {
	employments []experience.Employment
	err         error
}

func (f fakeSuggestionExperience) ListEmployments(context.Context, int64) ([]experience.Employment, error) {
	return f.employments, f.err
}

type fakeSuggestionCompanies struct {
	exists map[string]bool
	err    error
}

func (f fakeSuggestionCompanies) CompanyExists(_ context.Context, slug string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.exists[slug], nil
}

func strPtr(s string) *string { return &s }

func TestMentorSuggestions_FullData_EveryFieldPresent(t *testing.T) {
	h := &mentorSuggestionsHandlers{
		resume: fakeSuggestionResume{
			structured: resumeextract.Structured{
				FullName: "Jane Doe", Headline: "Staff Engineer", Summary: "Ten years of Go.",
				Languages: []string{"English", "German"},
			},
			structOK: true,
		},
		userProfile: fakeSuggestionUserProfile{
			profile: userprofile.Profile{Specializations: []string{"backend"}},
		},
		account: fakeSuggestionAccount{
			user: accounts.User{Timezone: strPtr("Europe/Berlin")},
		},
		experience: fakeSuggestionExperience{
			employments: []experience.Employment{
				{Kind: experience.KindJob, Company: "Acme Inc", Current: true},
			},
		},
		companies: fakeSuggestionCompanies{exists: map[string]bool{"acme": true}},
	}

	got := h.suggest(context.Background(), 1)

	if got.Name == nil || *got.Name != "Jane Doe" {
		t.Errorf("Name = %v, want \"Jane Doe\"", got.Name)
	}
	if got.Headline == nil || *got.Headline != "Staff Engineer" {
		t.Errorf("Headline = %v, want \"Staff Engineer\"", got.Headline)
	}
	if got.Bio == nil || *got.Bio != "Ten years of Go." {
		t.Errorf("Bio = %v, want the summary", got.Bio)
	}
	if len(got.Languages) != 2 {
		t.Errorf("Languages = %v, want 2", got.Languages)
	}
	if len(got.Topics) != 1 || got.Topics[0] != "backend" {
		t.Errorf("Topics = %v, want [backend]", got.Topics)
	}
	if got.Timezone == nil || *got.Timezone != "Europe/Berlin" {
		t.Errorf("Timezone = %v, want Europe/Berlin", got.Timezone)
	}
	if got.CompanySlug == nil || *got.CompanySlug != "acme" {
		t.Errorf("CompanySlug = %v, want acme", got.CompanySlug)
	}
}

func TestMentorSuggestions_NoData_SucceedsWithEveryFieldAbsent(t *testing.T) {
	h := &mentorSuggestionsHandlers{
		resume:      fakeSuggestionResume{structErr: errors.New("no résumé")},
		userProfile: fakeSuggestionUserProfile{err: userprofile.ErrNotFound},
		account:     fakeSuggestionAccount{err: errors.New("no account")},
		experience:  fakeSuggestionExperience{},
		companies:   fakeSuggestionCompanies{},
	}

	got := h.suggest(context.Background(), 1)

	if got.Name != nil || got.Headline != nil || got.Bio != nil || got.Timezone != nil || got.CompanySlug != nil {
		t.Errorf("expected every scalar field absent, got %+v", got)
	}
	if len(got.Languages) != 0 || len(got.Topics) != 0 {
		t.Errorf("expected every list field empty, got %+v", got)
	}
}

func TestMentorSuggestions_UnmatchedEmployer_CompanyFieldAbsent(t *testing.T) {
	h := &mentorSuggestionsHandlers{
		resume:      fakeSuggestionResume{},
		userProfile: fakeSuggestionUserProfile{},
		account:     fakeSuggestionAccount{},
		experience: fakeSuggestionExperience{
			employments: []experience.Employment{
				{Kind: experience.KindJob, Company: "Some Unknown Startup", Current: true},
			},
		},
		companies: fakeSuggestionCompanies{exists: map[string]bool{}},
	}

	got := h.suggest(context.Background(), 1)

	if got.CompanySlug != nil {
		t.Errorf("CompanySlug = %v, want nil for an unmatched employer", got.CompanySlug)
	}
}
