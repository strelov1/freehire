package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// structuredResumeReader is the slice of the résumé store profile and CV seed need
// (*resume.Store satisfies it), kept narrow so handlers are unit-testable without a
// database. Structured reports a parse current with the stored CV; ProvisionalContacts
// returns identity-only fields from a superseded blob while that stamp is pending.
type structuredResumeReader interface {
	Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
	ProvisionalContacts(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
	// Geography is where the CV says the candidate IS, under the same freshness rule as
	// Structured — a geography derived from a superseded CV reads as absent.
	Geography(ctx context.Context, userID int64) (resume.Geography, bool, error)
	CandidateOwned(ctx context.Context, userID int64) (resume.Owned, error)
	StructureForSeed(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
}

// profileHandlers serves the single-per-user profile (a specialization + skills set).
// The use cases live in userprofile.Service; the handlers translate wire ↔ domain and
// delegate to it. resume supplies the structured résumé the read carries alongside the
// profile, and may be nil where the résumé surface is not configured.
type profileHandlers struct {
	userProfile *userprofile.Service
	resume      structuredResumeReader
	// bank supplies the work history the cv block reports. Nil reads as an empty bank.
	bank candidateProfiler
}

func newProfileHandlers(userProfile *userprofile.Service, resume structuredResumeReader, bank candidateProfiler) *profileHandlers {
	return &profileHandlers{userProfile: userProfile, resume: resume, bank: bank}
}

func (h *profileHandlers) register(api fiber.Router, mw middleware) {
	// The user profile is a singleton — one per user, keyed by the authenticated caller,
	// no id in the path. GET returns the profile or null; PUT upserts (create-or-replace);
	// DELETE clears it (idempotent).
	//
	// The read takes a key so the CLI can ground itself in what the user is actually
	// looking for instead of interrogating them for it. The writes stay cookie-only: a
	// key that leaks out of a script's environment must not rewrite or clear a profile.
	// (The in-app assistant does not come through here at all — its tools run in-process
	// with the user id in hand, no credential.)
	api.Get("/me/profile", mw.key, h.GetProfile)
	api.Put("/me/profile", mw.cookie, h.PutProfile)
	api.Delete("/me/profile", mw.cookie, h.DeleteProfile)
}

// profileResponse is the public shape of the user's single profile. user_id is omitted
// (ownership, internal); there is no id or name. specializations are one or more job
// categories; skills are canonical lowercase tokens; location_preferences is the stored
// location block echoed verbatim, or null when the user set none.
//
// cv is the caller's structured résumé projected onto its contact-free view, so one
// authenticated read gives a programmatic consumer both what the user is looking for and
// what they have done. Null when no current structure exists. Contacts are served only
// by GET /me/resume — a profile is a professional self, and the agents that read this
// have no business knowing the candidate's name.
type profileResponse struct {
	Specializations     []string                    `json:"specializations"`
	Skills              []string                    `json:"skills"`
	Seniorities         []string                    `json:"seniorities"`
	ExcludedSkills      []string                    `json:"excluded_skills"`
	LocationPreferences json.RawMessage             `json:"location_preferences"`
	DerivedLocation     *derivedLocation            `json:"derived_location"`
	CV                  *resumeextract.Professional `json:"cv"`
	CreatedAt           *time.Time                  `json:"created_at"`
	UpdatedAt           *time.Time                  `json:"updated_at"`
}

// derivedLocation is where the caller's CV says they ARE, as resolved by the location
// dictionary. It is deliberately a sibling of location_preferences rather than merged
// into it: one is what the candidate asserted and the other is what was derived for them,
// and a consumer needs to know which it is holding. Read-only — no profile write can set
// it, because it is produced solely by the CV derivation.
//
// The client uses it to pre-fill "where you're based" for a user who has stated no base,
// so confirming a fact already on the CV is cheaper than retyping it. null when the
// caller has no current structured résumé.
type derivedLocation struct {
	Countries []string `json:"countries"`
	Regions   []string `json:"regions"`
	Cities    []string `json:"cities"`
}

// toProfileResponse maps a stored profile to its wire shape (no user id). The location
// block is the raw JSONB (json.RawMessage), which marshals through unchanged — a NULL
// column stays null.
func toProfileResponse(p userprofile.Profile, cv *resumeextract.Professional, loc *derivedLocation) profileResponse {
	return profileResponse{
		Specializations:     p.Specializations,
		Skills:              p.Skills,
		Seniorities:         p.Seniorities,
		ExcludedSkills:      p.ExcludedSkills,
		LocationPreferences: p.LocationPreferences,
		DerivedLocation:     loc,
		CV:                  cv,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}

// structuredCV reads the caller's structured résumé as its contact-free projection, or
// nil when there is none. Best-effort by design: the résumé supplements the profile, so
// an unconfigured reader or a failing lookup degrades to a null cv block rather than
// denying the caller their own profile.
func (h *profileHandlers) structuredCV(ctx context.Context, userID int64) *resumeextract.Professional {
	// The structure still owns education, languages, the summary and the years estimate,
	// and is read best-effort: absent or stale, the caller loses those sections rather
	// than the whole block. The work history comes from the bank either way.
	var structured resumeextract.Structured
	if h.resume != nil {
		if stored, ok, err := h.resume.Structured(ctx, userID); err == nil && ok {
			structured = stored
		}
		// Owned overrides win field by field over the current extract for the flat
		// semantic-body fields it covers — same precedence GetResume composes with. A
		// candidate who edited their summary via PUT /me/resume/contacts must see that
		// edit here too, not just on the résumé page.
		if owned, err := h.resume.CandidateOwned(ctx, userID); err == nil {
			owned.ApplyBody(&structured)
		}
	}

	// A missing bank reads as an EMPTY bank, not as "no CV". The two are different claims:
	// one says the candidate has no banked work history, the other throws away the
	// headline and education the structure still knows.
	professional := structured.Professional()
	professional.Experience = nil
	if h.bank != nil {
		composed, err := h.bank.Professional(ctx, userID, structured)
		if err != nil {
			log.Printf("profile cv block: user %d: %v", userID, err)
		} else {
			professional = composed
		}
	}
	// Nothing known at all reads as no CV, exactly as before — an empty object would tell
	// an agent there is a profile to work from when there is not. Summary/Languages/
	// Certifications are checked too: unlike Headline/Education, a candidate can now set
	// these directly via the owned-overlay editors without ever touching a CV parse (see
	// CvSummaryCard/EducationCard), so a summary-only candidate must still get a cv block.
	if len(professional.Experience) == 0 && len(professional.Education) == 0 &&
		professional.Headline == "" && professional.Summary == "" &&
		len(professional.Languages) == 0 && len(professional.Certifications) == 0 {
		return nil
	}
	return &professional
}

// profileError maps the user-profile sentinels onto HTTP statuses: an unknown/empty/
// over-long specialization set or empty skills is a 400; a missing profile is a 404 (the
// verdict/ATS sub-resources). GET translates ErrNotFound to a null payload itself, so it
// does not go through here. Anything else falls through to RenderError as a 500.
func profileError(err error) error {
	switch {
	case errors.Is(err, userprofile.ErrInvalidSpecialization):
		return fiber.NewError(fiber.StatusBadRequest, "specialization is not a known category")
	case errors.Is(err, userprofile.ErrEmptySpecializations):
		return fiber.NewError(fiber.StatusBadRequest, "at least one specialization is required")
	case errors.Is(err, userprofile.ErrTooManySpecializations):
		return fiber.NewError(fiber.StatusBadRequest, "too many specializations (max 5)")
	case errors.Is(err, userprofile.ErrEmptySkills):
		return fiber.NewError(fiber.StatusBadRequest, "at least one skill is required")
	case errors.Is(err, userprofile.ErrTooManySkills):
		return fiber.NewError(fiber.StatusBadRequest, "too many skills (max 200)")
	case errors.Is(err, userprofile.ErrInvalidSeniority):
		return fiber.NewError(fiber.StatusBadRequest, "seniority is not a known level")
	case errors.Is(err, userprofile.ErrInvalidWorkMode):
		return fiber.NewError(fiber.StatusBadRequest, "work mode is not a known value")
	case errors.Is(err, userprofile.ErrInvalidRegion):
		return fiber.NewError(fiber.StatusBadRequest, "region is not a known value")
	case errors.Is(err, userprofile.ErrInvalidCountry):
		return fiber.NewError(fiber.StatusBadRequest, "country is not a valid two-letter code")
	case errors.Is(err, userprofile.ErrTooManyCountries):
		return fiber.NewError(fiber.StatusBadRequest, "too many countries")
	case errors.Is(err, userprofile.ErrTooManyCities):
		return fiber.NewError(fiber.StatusBadRequest, "too many cities")
	case errors.Is(err, userprofile.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "profile not found")
	default:
		return err
	}
}

// saveProfileRequest is the upsert body: a non-empty set of specializations (job
// categories), a non-empty set of skills, and an optional location_preferences block. The
// whole profile is replaced on each save; an omitted/null location block clears it.
type saveProfileRequest struct {
	Specializations     []string                         `json:"specializations"`
	Skills              []string                         `json:"skills"`
	Seniorities         []string                         `json:"seniorities"`
	ExcludedSkills      []string                         `json:"excluded_skills"`
	LocationPreferences *userprofile.LocationPreferences `json:"location_preferences"`
}

// GetProfile returns the authenticated user's single profile, or {"data": null} when they
// have not saved one yet.
func (h *profileHandlers) GetProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	profile, err := h.userProfile.Get(c.Context(), userID)
	if errors.Is(err, userprofile.ErrNotFound) {
		return c.JSON(fiber.Map{"data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toProfileResponse(profile, h.structuredCV(c.Context(), userID), h.derivedLocation(c.Context(), userID))})
}

// PutProfile creates-or-replaces the authenticated user's profile (specializations +
// skills + optional excluded skills + optional location preferences). A bad/empty
// specialization set, empty skills, or an out-of-vocabulary location value is a 400.
// Cookie-only.
func (h *profileHandlers) PutProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in saveProfileRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	profile, err := h.userProfile.Save(c.Context(), userID, in.Specializations, in.Skills, in.Seniorities, in.ExcludedSkills, in.LocationPreferences)
	if err != nil {
		return profileError(err)
	}
	// The same representation the read serves — one resource, one shape, so a client that
	// saves and a client that fetches see the same profile.
	return c.JSON(fiber.Map{"data": toProfileResponse(profile, h.structuredCV(c.Context(), userID), h.derivedLocation(c.Context(), userID))})
}

// DeleteProfile clears the authenticated user's profile. Idempotent: deleting when none
// exists is still a 204. Cookie-only.
func (h *profileHandlers) DeleteProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	if err := h.userProfile.Delete(c.Context(), userID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// derivedLocation reads the geography derived from the caller's CV, or nil when there is
// none. Best-effort like the cv block: the derivation supplements the profile, so an
// unconfigured résumé service or a failing lookup degrades to a null block rather than
// denying the caller their own profile.
//
// A geography that resolved nothing reads as absent rather than as an empty block. The
// database keeps "the CV stated nothing" and "the CV stated something unresolvable"
// apart for the coverage metric, but a client asking "where is this candidate" gets the
// same answer from both — nothing to pre-fill — and an empty block would invite it to
// render an empty control as if it meant something.
func (h *profileHandlers) derivedLocation(ctx context.Context, userID int64) *derivedLocation {
	if h.resume == nil {
		return nil
	}
	geo, ok, err := h.resume.Geography(ctx, userID)
	if err != nil {
		log.Printf("profile derived location: user %d: %v", userID, err)
		return nil
	}
	if !ok || (len(geo.Countries) == 0 && len(geo.Regions) == 0 && len(geo.Cities) == 0) {
		return nil
	}
	return &derivedLocation{Countries: geo.Countries, Regions: geo.Regions, Cities: geo.Cities}
}
