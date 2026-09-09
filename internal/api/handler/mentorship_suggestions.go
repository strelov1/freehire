package handler

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// mentorSuggestionResume is the one read a mentor-profile suggestion needs from the
// résumé store: StructureForSeed, the same identity-plus-body composition cv_seed.go
// uses to seed a new document from the candidate's CV — owned overrides applied over
// the current extract, both identity fields (name included) and body fields alike.
// Structured+CandidateOwned alone would miss identity: ApplyBody only ever touches the
// five BODY fields, never FullName/Email/Phone/Location/Links.
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

// mentorProfileSuggestions is a best-effort, per-field starting point for the
// mentor-profile create form. Every field is independent and omitted (rather than an
// empty string or slice) when its source has nothing to offer — see the
// mentor-profile-prefill spec. It reads only the candidate's résumé, user profile,
// account and experience bank; it never reads the mentor profile itself.
type mentorProfileSuggestions struct {
	Name        *string  `json:"name,omitempty"`
	Headline    *string  `json:"headline,omitempty"`
	Bio         *string  `json:"bio,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	Timezone    *string  `json:"timezone,omitempty"`
	CompanySlug *string  `json:"company_slug,omitempty"`
}

// mentorSuggestionsHandlers composes the mentor-profile create form's prefill from four
// unrelated blocks' own services. It depends on none of their concrete types beyond
// what it reads, so it needs no database to test.
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

func (h *mentorSuggestionsHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/mentorship/profile/suggestions", mw.key, h.GetSuggestions)
}

// GetSuggestions answers with the caller's best-effort mentor-profile prefill. It never
// fails on missing or unreadable source data — every field simply degrades to absent —
// so the only failure this route can report is an unauthenticated caller.
func (h *mentorSuggestionsHandlers) GetSuggestions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": h.suggest(c.Context(), userID)})
}

func (h *mentorSuggestionsHandlers) suggest(ctx context.Context, userID int64) mentorProfileSuggestions {
	var out mentorProfileSuggestions

	// Name, headline, bio and languages: StructureForSeed's identity-plus-body
	// composition — the current résumé extract, overlaid by whatever the candidate
	// owns, identity fields included.
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

// currentEmployerSlug resolves the candidate's current job in the experience bank to a
// company slug, but only on an EXACT catalog match — the same normalization the
// mentor-profile create flow's company_slug foreign key would accept. An employer that
// does not resolve is not guessed at further; it is simply absent from the suggestion.
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
