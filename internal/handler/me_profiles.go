package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/searchprofile"
)

// searchProfileResponse is the public shape of a search profile. user_id is omitted
// (ownership, internal). specializations are one or more job categories; skills are
// canonical lowercase tokens.
type searchProfileResponse struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Specializations []string   `json:"specializations"`
	Skills          []string   `json:"skills"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

// toSearchProfileResponse maps a stored profile to its wire shape (no user id).
func toSearchProfileResponse(p db.SearchProfile) searchProfileResponse {
	return searchProfileResponse{
		ID:              p.ID,
		Name:            p.Name,
		Specializations: p.Specializations,
		Skills:          p.Skills,
		CreatedAt:       timePtr(p.CreatedAt),
		UpdatedAt:       timePtr(p.UpdatedAt),
	}
}

// searchProfileError maps the search-profile sentinels onto HTTP statuses: a bad name,
// unknown specialization, or empty skills is a 400; a duplicate name or the per-user cap
// is a 409; a missing/non-owned row is a 404. Anything else falls through to RenderError
// as a 500.
func searchProfileError(err error) error {
	switch {
	case errors.Is(err, searchprofile.ErrInvalidName):
		return fiber.NewError(fiber.StatusBadRequest, "name must be 1-100 characters")
	case errors.Is(err, searchprofile.ErrInvalidSpecialization):
		return fiber.NewError(fiber.StatusBadRequest, "specialization is not a known category")
	case errors.Is(err, searchprofile.ErrEmptySpecializations):
		return fiber.NewError(fiber.StatusBadRequest, "at least one specialization is required")
	case errors.Is(err, searchprofile.ErrTooManySpecializations):
		return fiber.NewError(fiber.StatusBadRequest, "too many specializations (max 5)")
	case errors.Is(err, searchprofile.ErrEmptySkills):
		return fiber.NewError(fiber.StatusBadRequest, "at least one skill is required")
	case errors.Is(err, searchprofile.ErrDuplicateName):
		return fiber.NewError(fiber.StatusConflict, "a profile with this name already exists")
	case errors.Is(err, searchprofile.ErrCapExceeded):
		return fiber.NewError(fiber.StatusConflict, "profile limit reached")
	case errors.Is(err, searchprofile.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "profile not found")
	default:
		return err
	}
}

// createSearchProfileRequest is the create body: a required display name, a non-empty set
// of specializations (job categories), and a non-empty set of skills.
type createSearchProfileRequest struct {
	Name            string   `json:"name"`
	Specializations []string `json:"specializations"`
	Skills          []string `json:"skills"`
}

// updateSearchProfileRequest is the partial-update body: a nil name or an omitted
// specializations/skills field is left unchanged, so a caller can rename, re-specialize,
// replace skills, or any combination. A provided-but-empty specializations/skills array is
// rejected (400).
type updateSearchProfileRequest struct {
	Name            *string  `json:"name"`
	Specializations []string `json:"specializations"`
	Skills          []string `json:"skills"`
}

// CreateSearchProfile stores a named profile (specialization + skills) for the
// authenticated user. Behind RequireAuth (cookie-only): profiles are a browser feature.
// A bad name/specialization/skills is a 400, a duplicate name or the per-user cap is a 409.
func (a *API) CreateSearchProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createSearchProfileRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	profile, err := a.searchProfile.Create(c.Context(), userID, in.Name, in.Specializations, in.Skills)
	if err != nil {
		return searchProfileError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSearchProfileResponse(profile)})
}

// ListSearchProfiles returns the authenticated user's profiles, most recently updated
// first. Owner-scoped, so it never reveals another user's. Cookie-only.
func (a *API) ListSearchProfiles(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := a.searchProfile.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]searchProfileResponse, len(rows))
	for i, r := range rows {
		out[i] = toSearchProfileResponse(r)
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"total": len(out)}})
}

// UpdateSearchProfile overwrites a profile's name, specialization, and/or skills
// (partial), scoped to its owner. A missing or non-owned id is a 404; a bad
// name/specialization/skills is a 400; a name collision is a 409.
func (a *API) UpdateSearchProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in updateSearchProfileRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	profile, err := a.searchProfile.Update(c.Context(), userID, id, in.Name, in.Specializations, in.Skills)
	if err != nil {
		return searchProfileError(err)
	}
	return c.JSON(fiber.Map{"data": toSearchProfileResponse(profile)})
}

// DeleteSearchProfile removes one of the authenticated user's profiles by id.
// Owner-scoped: an id that does not exist or belongs to another user is a 404. Cookie-only.
func (a *API) DeleteSearchProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	if err := a.searchProfile.Delete(c.Context(), userID, id); err != nil {
		return searchProfileError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
