package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/userprofile"
)

// profileResponse is the public shape of the user's single profile. user_id is omitted
// (ownership, internal); there is no id or name. specializations are one or more job
// categories; skills are canonical lowercase tokens.
type profileResponse struct {
	Specializations []string   `json:"specializations"`
	Skills          []string   `json:"skills"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

// toProfileResponse maps a stored profile to its wire shape (no user id).
func toProfileResponse(p db.UserProfile) profileResponse {
	return profileResponse{
		Specializations: p.Specializations,
		Skills:          p.Skills,
		CreatedAt:       timePtr(p.CreatedAt),
		UpdatedAt:       timePtr(p.UpdatedAt),
	}
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
	case errors.Is(err, userprofile.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "profile not found")
	default:
		return err
	}
}

// saveProfileRequest is the upsert body: a non-empty set of specializations (job
// categories) and a non-empty set of skills. The whole profile is replaced on each save.
type saveProfileRequest struct {
	Specializations []string `json:"specializations"`
	Skills          []string `json:"skills"`
}

// GetProfile returns the authenticated user's single profile, or {"data": null} when they
// have not saved one yet. Behind RequireAuth (cookie-only): profiles are a browser feature.
func (a *API) GetProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	profile, err := a.userProfile.Get(c.Context(), userID)
	if errors.Is(err, userprofile.ErrNotFound) {
		return c.JSON(fiber.Map{"data": nil})
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toProfileResponse(profile)})
}

// PutProfile creates-or-replaces the authenticated user's profile (specializations +
// skills). A bad/empty specialization set or empty skills is a 400. Cookie-only.
func (a *API) PutProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in saveProfileRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	profile, err := a.userProfile.Save(c.Context(), userID, in.Specializations, in.Skills)
	if err != nil {
		return profileError(err)
	}
	return c.JSON(fiber.Map{"data": toProfileResponse(profile)})
}

// DeleteProfile clears the authenticated user's profile. Idempotent: deleting when none
// exists is still a 204. Cookie-only.
func (a *API) DeleteProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	if err := a.userProfile.Delete(c.Context(), userID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
