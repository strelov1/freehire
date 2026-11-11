package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// structuredResumeReader is the slice of the résumé store profile and CV seed need
// (*resume.Store satisfies it), kept narrow so handlers are unit-testable without a
// database. Structured reports a parse current with the stored CV; StructureForSeed
// composes the whole precedence (owned block, else current extract, else the identity-only
// slice of a superseded one), which is what a caller that needs identity-while-pending asks
// for rather than layering the superseded blob on itself.
type structuredResumeReader interface {
	Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
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

func (h *profileHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The user profile is a singleton — one per user, keyed by the authenticated caller,
	// no id in the path. GET returns the profile or null; PUT upserts (create-or-replace);
	// DELETE clears it (idempotent).
	//
	// The read takes a key so the CLI can ground itself in what the user is actually
	// looking for instead of interrogating them for it. The writes stay cookie-only: a
	// key that leaks out of a script's environment must not rewrite or clear a profile.
	// (The in-app assistant does not come through here at all — its tools run in-process
	// with the user id in hand, no credential.)
	api.Get("/me/profile", mw.key, contract)
	api.Put("/me/profile", mw.cookie, contract)
	api.Delete("/me/profile", mw.cookie, contract)
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

// profileOut maps a stored profile to its wire shape (no user id; user_id is ownership,
// internal, and there is no id or name). specializations are zero or more job categories;
// skills are canonical lowercase tokens; location_preferences is the stored block, or null
// when the user set none.
//
// cv is the caller's structured résumé projected onto its contact-free view, so one
// authenticated read gives a programmatic consumer both what the user is looking for and
// what they have done. Null when no current structure exists. Contacts are served only
// by GET /me/resume — a profile is a professional self, and the agents that read this
// have no business knowing the candidate's name.
//
// The cv and location blocks are mapped through their JSON, so the wire form stays what
// the domain types already marshal; the assistant's profile tool shares them and keeps its
// own types.
func profileOut(p userprofile.Profile, cv *resumeextract.Professional, loc *derivedLocation) (apigen.Profile, error) {
	out := apigen.Profile{
		Specializations:   p.Specializations,
		Skills:            p.Skills,
		Seniorities:       p.Seniorities,
		ExcludedSkills:    p.ExcludedSkills,
		ExcludedSources:   p.ExcludedSources,
		ExcludedCompanies: p.ExcludedCompanies,
		JobLanguages:      p.JobLanguages,
		CreatedAt:         nilDateTime(p.CreatedAt),
		UpdatedAt:         nilDateTime(p.UpdatedAt),
	}
	// Both blocks travel as raw JSON, exactly as before the contract: the stored JSONB as
	// written, and the CV as its Go type marshals. Decoding them into generated types would
	// turn an old stored shape into a 500 and drop any field the contract does not name.
	out.LocationPreferences = jx.Raw("null")
	if len(p.LocationPreferences) > 0 {
		out.LocationPreferences = jx.Raw(p.LocationPreferences)
	}
	if loc == nil {
		out.DerivedLocation.SetToNull()
	} else {
		out.DerivedLocation.SetTo(apigen.DerivedLocation{Countries: loc.Countries, Regions: loc.Regions, Cities: loc.Cities})
	}
	out.Cv = jx.Raw("null")
	if cv != nil {
		raw, err := json.Marshal(cv)
		if err != nil {
			return apigen.Profile{}, err
		}
		out.Cv = raw
	}
	return out, nil
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

// profileError maps the user-profile sentinels onto HTTP statuses: an unknown or
// over-long specialization set is a 400; a missing profile is a 404 (the
// verdict/ATS sub-resources). GET translates ErrNotFound to a null payload itself, so it
// does not go through here. Anything else falls through to RenderError as a 500.
func profileError(err error) error {
	switch {
	case errors.Is(err, userprofile.ErrInvalidSpecialization):
		return fiber.NewError(fiber.StatusBadRequest, "specialization is not a known category")
	case errors.Is(err, userprofile.ErrTooManySpecializations):
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("too many specializations (max %d)", userprofile.MaxSpecializations))
	case errors.Is(err, userprofile.ErrTooManySkills):
		return fiber.NewError(fiber.StatusBadRequest, "too many skills (max 200)")
	case errors.Is(err, userprofile.ErrTooManySources):
		return fiber.NewError(fiber.StatusBadRequest, "too many excluded sources (max 200)")
	case errors.Is(err, userprofile.ErrTooManyCompanies):
		return fiber.NewError(fiber.StatusBadRequest, "too many excluded companies (max 200)")
	case errors.Is(err, userprofile.ErrInvalidJobLanguage):
		return fiber.NewError(fiber.StatusBadRequest, "job languages must be two-letter language codes (max 200)")
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

// GetProfile returns the authenticated user's single profile, or {"data": null} when they
// have not saved one yet.
func (h *profileHandlers) GetProfile(ctx context.Context) (*apigen.GetProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := h.userProfile.Get(ctx, userID)
	if errors.Is(err, userprofile.ErrNotFound) {
		return &apigen.GetProfileOK{Data: apigen.NilProfile{Null: true}}, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := profileOut(profile, h.structuredCV(ctx, userID), h.derivedLocation(ctx, userID))
	if err != nil {
		return nil, err
	}
	return &apigen.GetProfileOK{Data: apigen.NewNilProfile(out)}, nil
}

// PutProfile creates-or-replaces the authenticated user's profile (specializations +
// skills + optional excluded skills + optional location preferences). A bad/empty
// specialization set, empty skills, or an out-of-vocabulary location value is a 400.
// Cookie-only.
func (h *profileHandlers) PutProfile(ctx context.Context, req *apigen.ProfileUpdate) (*apigen.PutProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Empty lists support progressive autosave; omitted/null lists are still malformed
	// replacement requests and must not silently clear an existing profile.
	if req.Specializations.Value == nil || req.Skills.Value == nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "specializations and skills must be arrays; use [] for an empty list")
	}
	// Decoded straight into the domain type, as the body parser did: absent and null keep none.
	var loc *userprofile.LocationPreferences
	if raw := req.LocationPreferences; len(raw) > 0 && string(raw) != "null" {
		loc = &userprofile.LocationPreferences{}
		if err := json.Unmarshal(raw, loc); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid body")
		}
	}

	profile, err := h.userProfile.Save(ctx, userID, req.Specializations.Value, req.Skills.Value, req.Seniorities.Value, req.ExcludedSkills.Value, req.ExcludedSources.Value, req.ExcludedCompanies.Value, loc, req.JobLanguages.Value...)
	if err != nil {
		return nil, profileError(err)
	}
	// The same representation the read serves — one resource, one shape, so a client that
	// saves and a client that fetches see the same profile.
	out, err := profileOut(profile, h.structuredCV(ctx, userID), h.derivedLocation(ctx, userID))
	if err != nil {
		return nil, err
	}
	return &apigen.PutProfileOK{Data: out}, nil
}

// DeleteProfile clears the authenticated user's profile. Idempotent: deleting when none
// exists is still a 204. Cookie-only.
func (h *profileHandlers) DeleteProfile(ctx context.Context) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	return h.userProfile.Delete(ctx, userID)
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
