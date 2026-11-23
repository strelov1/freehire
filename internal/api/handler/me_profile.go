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
// (*resume.Store satisfies it), kept narrow so handlers are unit-testable without a database.
// Structured reports a parse current with the stored CV; StructureForSeed composes the whole
// precedence (owned block, else current extract, else the identity-only slice of a
// superseded one) for callers that need identity while a parse is pending.
type structuredResumeReader interface {
	Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
	// Geography is where the CV says the candidate IS, under the same freshness rule as
	// Structured: a superseded CV reads as absent.
	Geography(ctx context.Context, userID int64) (resume.Geography, bool, error)
	CandidateOwned(ctx context.Context, userID int64) (resume.Owned, error)
	// SetCandidateOwned replaces the owned block after the profile page's length limits;
	// the only write the profile's headline and summary have, used by update_profile.
	SetCandidateOwned(ctx context.Context, userID int64, o resume.Owned) (resume.Owned, error)
	StructureForSeed(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
}

// profileHandlers serves the single-per-user profile (a specialization + skills set).
// The use cases live in userprofile.Service; handlers translate wire to domain. resume
// supplies the structured résumé the read carries alongside, and may be nil.
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
	// The profile is a singleton keyed by the caller, no id in the path. GET returns the
	// profile or null; PUT upserts; DELETE clears it (idempotent).
	//
	// The read takes a key so the CLI can ground itself in what the user is looking for. The
	// writes stay cookie-only: a key leaked from a script's environment must not rewrite or
	// clear a profile.
	api.Get("/me/profile", mw.key, contract)
	api.Put("/me/profile", mw.cookie, contract)
	api.Delete("/me/profile", mw.cookie, contract)
	// The excluded-companies list has its own writer: PUT /me/profile never touches it.
	api.Post("/me/profile/excluded-companies", mw.cookie, contract)
	api.Delete("/me/profile/excluded-companies/:company_slug", mw.cookie, contract)
}

// derivedLocation is where the caller's CV says they ARE, as resolved by the location
// dictionary. A sibling of location_preferences, not merged into it: one is asserted, the
// other derived, and a consumer must know which it holds. Read-only; produced solely by the
// CV derivation. The client uses it to pre-fill "where you're based". null when the caller
// has no current structured résumé.
type derivedLocation struct {
	Countries []string `json:"countries"`
	Regions   []string `json:"regions"`
	Cities    []string `json:"cities"`
}

// profileOut maps a stored profile to its wire shape (no user id). skills are canonical
// lowercase tokens; location_preferences is the stored block, or null when none was set.
//
// cv is the caller's structured résumé projected onto its contact-free view, so one read
// gives a programmatic consumer both what the user wants and what they have done. Null when
// no current structure exists. Contacts are served only by GET /me/resume: agents reading a
// profile must not learn the candidate's name.
//
// The cv and location blocks are mapped through their JSON so the wire form stays what the
// domain types marshal; the assistant's profile tool shares them and keeps its own types.
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
	// Both blocks travel as raw JSON: the stored JSONB as written, the CV as its Go type
	// marshals. Decoding into generated types would 500 on an old stored shape and drop
	// fields the contract does not name.
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

// structuredCV reads the caller's structured résumé as its contact-free projection, or nil
// when there is none. Best-effort: an unconfigured reader or failing lookup degrades to a
// null cv block rather than denying the caller their own profile.
func (h *profileHandlers) structuredCV(ctx context.Context, userID int64) *resumeextract.Professional {
	// The structure still owns education, languages, the summary and the years estimate and
	// is read best-effort: absent or stale, the caller loses those sections, not the block.
	// The work history comes from the bank either way.
	var structured resumeextract.Structured
	if h.resume != nil {
		if stored, ok, err := h.resume.Structured(ctx, userID); err == nil && ok {
			structured = stored
		}
		// Owned overrides win field by field over the current extract for the flat
		// semantic-body fields, the same precedence GetResume composes with.
		if owned, err := h.resume.CandidateOwned(ctx, userID); err == nil {
			owned.ApplyBody(&structured)
		}
	}

	// A missing bank reads as an EMPTY bank, not "no CV": the latter would discard the
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
	// Nothing known at all reads as no CV: an empty object would tell an agent there is a
	// profile to work from. Summary/Languages/Certifications are checked too, since a
	// candidate can set them via the owned-overlay editors without any CV parse.
	if len(professional.Experience) == 0 && len(professional.Education) == 0 &&
		professional.Headline == "" && professional.Summary == "" &&
		len(professional.Languages) == 0 && len(professional.Certifications) == 0 {
		return nil
	}
	return &professional
}

// profileError maps the user-profile sentinels onto HTTP: an unknown or over-long
// specialization set is a 400, a missing profile a 404 (verdict/ATS sub-resources). GET
// turns ErrNotFound into a null payload itself. Anything else is a 500.
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
	case errors.Is(err, userprofile.ErrInvalidCompany):
		return fiber.NewError(fiber.StatusBadRequest, "company_slug must be 1-100 characters")
	case errors.Is(err, userprofile.ErrCompaniesFull):
		return fiber.NewError(fiber.StatusConflict, "the companies-to-avoid list is full (max 200)")
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

// PutProfile creates-or-replaces the caller's profile. A bad/empty specialization set, empty
// skills, or an out-of-vocabulary location value is a 400. Cookie-only.
func (h *profileHandlers) PutProfile(ctx context.Context, req *apigen.ProfileUpdate) (*apigen.PutProfileOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Empty lists support progressive autosave; omitted/null lists are malformed replacements
	// and must not silently clear an existing profile.
	if req.Specializations.Value == nil || req.Skills.Value == nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "specializations and skills must be arrays; use [] for an empty list")
	}
	// Decoded straight into the domain type: absent and null keep none.
	var loc *userprofile.LocationPreferences
	if raw := req.LocationPreferences; len(raw) > 0 && string(raw) != "null" {
		loc = &userprofile.LocationPreferences{}
		if err := json.Unmarshal(raw, loc); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid body")
		}
	}

	profile, err := h.userProfile.Save(ctx, userID, req.Specializations.Value, req.Skills.Value, req.Seniorities.Value, req.ExcludedSkills.Value, req.ExcludedSources.Value, loc, req.JobLanguages.Value...)
	if err != nil {
		return nil, profileError(err)
	}
	// The same representation the read serves, so save and fetch agree.
	out, err := profileOut(profile, h.structuredCV(ctx, userID), h.derivedLocation(ctx, userID))
	if err != nil {
		return nil, err
	}
	resp := &apigen.PutProfileOK{Data: out}
	// Old clients still send the list; it is read and not applied (see ExcludeCompany).
	if req.ExcludedCompanies.Set {
		resp.Meta = apigen.NewOptIgnoredFieldsMeta(apigen.IgnoredFieldsMeta{IgnoredFields: []string{"excluded_companies"}})
	}
	return resp, nil
}

// ExcludeCompany hides one company from the caller's match feeds. Idempotent; 409 only for a
// new company when the list is full. Cookie-only.
func (h *profileHandlers) ExcludeCompany(ctx context.Context, req *apigen.ExcludeCompanyRequest) (*apigen.ExcludeCompanyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.userProfile.ExcludeCompany(ctx, userID, req.CompanySlug)
	if errors.Is(err, userprofile.ErrConflict) {
		// Retryable, and not the 409 that means a full list.
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "profile busy, retry")
	}
	if err != nil {
		return nil, profileError(err)
	}
	out, err := profileOut(profile, h.structuredCV(ctx, userID), h.derivedLocation(ctx, userID))
	if err != nil {
		return nil, err
	}
	return &apigen.ExcludeCompanyOK{Data: out}, nil
}

// RestoreCompany removes one company from the caller's list. Idempotent; data is null when
// the caller has no profile. Cookie-only.
func (h *profileHandlers) RestoreCompany(ctx context.Context, params apigen.RestoreCompanyParams) (*apigen.RestoreCompanyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.userProfile.UnexcludeCompany(ctx, userID, params.CompanySlug)
	if errors.Is(err, userprofile.ErrNotFound) {
		return &apigen.RestoreCompanyOK{Data: apigen.NilProfile{Null: true}}, nil
	}
	if err != nil {
		return nil, profileError(err)
	}
	out, err := profileOut(profile, h.structuredCV(ctx, userID), h.derivedLocation(ctx, userID))
	if err != nil {
		return nil, err
	}
	return &apigen.RestoreCompanyOK{Data: apigen.NewNilProfile(out)}, nil
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
// none. Best-effort like the cv block. A geography that resolved nothing reads as absent, not
// an empty block: the DB keeps "stated nothing" and "stated something unresolvable" apart for
// the coverage metric, but a client has nothing to pre-fill in either case.
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
