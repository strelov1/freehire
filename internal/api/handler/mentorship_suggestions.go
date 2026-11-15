package handler

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// mentorSuggestionResume is the one résumé read a suggestion needs: StructureForSeed, the same
// identity-plus-body composition cv_seed.go uses. Structured+CandidateOwned alone would miss
// identity: ApplyBody only touches the five body fields.
type mentorSuggestionResume interface {
	StructureForSeed(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
}

// mentorSuggestionUserProfile is the one read a suggestion needs from the user's
// profile: its specializations.
type mentorSuggestionUserProfile interface {
	Get(ctx context.Context, userID int64) (userprofile.Profile, error)
}

// mentorSuggestionAccount is the one read a suggestion needs from the account: its
// timezone.
type mentorSuggestionAccount interface {
	UserByID(ctx context.Context, id int64) (accounts.User, error)
}

// mentorSuggestionExperience is the one read a suggestion needs from the experience
// bank: the candidate's current employer.
type mentorSuggestionExperience interface {
	ListEmployments(ctx context.Context, userID int64) ([]experience.Employment, error)
}

// mentorSuggestionCompanies checks a candidate's current employer against the company
// catalog, the same existence check the create flow's foreign key would enforce.
type mentorSuggestionCompanies interface {
	CompanyExists(ctx context.Context, slug string) (bool, error)
}

// mentorProfileSuggestions is a best-effort, per-field prefill for the mentor-profile create
// form. Each field is omitted (not empty) when its source has nothing; it never reads the
// mentor profile itself.
type mentorProfileSuggestions struct {
	Name        *string  `json:"name,omitempty"`
	Headline    *string  `json:"headline,omitempty"`
	Bio         *string  `json:"bio,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	Timezone    *string  `json:"timezone,omitempty"`
	CompanySlug *string  `json:"company_slug,omitempty"`
}

// mentorSuggestionsHandlers composes the prefill from four blocks' services and needs no
// database to test.
type mentorSuggestionsHandlers struct {
	resume      mentorSuggestionResume
	userProfile mentorSuggestionUserProfile
	account     mentorSuggestionAccount
	experience  mentorSuggestionExperience
	companies   mentorSuggestionCompanies
}

func newMentorSuggestionsHandlers(
	resume mentorSuggestionResume,
	userProfile mentorSuggestionUserProfile,
	account mentorSuggestionAccount,
	exp mentorSuggestionExperience,
	companies mentorSuggestionCompanies,
) *mentorSuggestionsHandlers {
	return &mentorSuggestionsHandlers{
		resume: resume, userProfile: userProfile, account: account,
		experience: exp, companies: companies,
	}
}

func (h *mentorSuggestionsHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/mentorship/profile/suggestions", mw.key, contract)
}

// GetMentorProfileSuggestions answers with the caller's prefill. Missing or unreadable
// sources degrade to absent fields; only an unauthenticated caller fails.
func (h *mentorSuggestionsHandlers) GetMentorProfileSuggestions(ctx context.Context) (*apigen.GetMentorProfileSuggestionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	in := h.suggest(ctx, userID)
	out := apigen.MentorProfileSuggestions{Languages: in.Languages, Topics: in.Topics}
	optNil(in.Name, out.Name.SetTo)
	optNil(in.Headline, out.Headline.SetTo)
	optNil(in.Bio, out.Bio.SetTo)
	optNil(in.Timezone, out.Timezone.SetTo)
	optNil(in.CompanySlug, out.CompanySlug.SetTo)
	return &apigen.GetMentorProfileSuggestionsOK{Data: out}, nil
}

func (h *mentorSuggestionsHandlers) suggest(ctx context.Context, userID int64) mentorProfileSuggestions {
	var out mentorProfileSuggestions

	// Name, headline, bio and languages: StructureForSeed's identity-plus-body composition.
	var structured resumeextract.Structured
	if stored, ok, err := h.resume.StructureForSeed(ctx, userID); err == nil && ok {
		structured = stored
	}
	if structured.FullName != "" {
		out.Name = &structured.FullName
	}
	if structured.Headline != "" {
		out.Headline = &structured.Headline
	}
	if structured.Summary != "" {
		out.Bio = &structured.Summary
	}
	if len(structured.Languages) > 0 {
		out.Languages = structured.Languages
	}

	if profile, err := h.userProfile.Get(ctx, userID); err == nil && len(profile.Specializations) > 0 {
		out.Topics = profile.Specializations
	}

	if user, err := h.account.UserByID(ctx, userID); err == nil && user.Timezone != nil && *user.Timezone != "" {
		out.Timezone = user.Timezone
	}

	if slug := h.currentEmployerSlug(ctx, userID); slug != "" {
		out.CompanySlug = &slug
	}

	return out
}

// currentEmployerSlug resolves the current job to a company slug only on an EXACT catalog
// match (the normalization the company_slug foreign key accepts); otherwise it is absent.
func (h *mentorSuggestionsHandlers) currentEmployerSlug(ctx context.Context, userID int64) string {
	employments, err := h.experience.ListEmployments(ctx, userID)
	if err != nil {
		return ""
	}
	for _, e := range employments {
		if e.Kind != experience.KindJob || !e.Current || e.Company == "" {
			continue
		}
		slug := normalize.CompanySlug(e.Company)
		if slug == "" {
			continue
		}
		exists, err := h.companies.CompanyExists(ctx, slug)
		if err != nil {
			log.Printf("mentor suggestions: user %d: company lookup for %q: %v", userID, slug, err)
			continue
		}
		if exists {
			return slug
		}
	}
	return ""
}
