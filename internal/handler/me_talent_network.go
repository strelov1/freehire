package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// talentNetworkVisibilityValues are the only values SetTalentNetworkVisibility accepts.
// Kept as a set here (not a shared vocab package) because the enum's authority is the
// Postgres CHECK/enum on users.talent_network_visibility (see migration) — this mirrors
// it for a cheap 400 without a round trip, not the other way around.
var talentNetworkVisibilityValues = map[string]bool{"off": true, "public": true, "anonymous": true}

// talentNetworkStore is the slice of *db.Queries the owner-facing visibility endpoint
// needs, kept narrow so the handler is unit-testable without a database.
type talentNetworkStore interface {
	GetTalentNetworkVisibility(ctx context.Context, id int64) (db.GetTalentNetworkVisibilityRow, error)
	SetTalentNetworkVisibility(ctx context.Context, arg db.SetTalentNetworkVisibilityParams) error
}

// talentNetworkHandlers serves the caller's own Talent Network visibility setting — a
// singleton-per-user resource living on `users`, distinct from the user_profiles-backed
// profileHandlers.
type talentNetworkHandlers struct {
	store talentNetworkStore
}

func newTalentNetworkHandlers(store talentNetworkStore) *talentNetworkHandlers {
	return &talentNetworkHandlers{store: store}
}

func (h *talentNetworkHandlers) register(api fiber.Router, mw middleware) {
	// The read takes a key (like GET /me/profile) so a script or the CLI can ground
	// itself in the caller's current setting. The write stays cookie-only — a leaked API
	// key must not be able to flip a candidate's visibility to the public internet
	// (see me_profile.go's PUT/DELETE comment for the same reasoning applied there).
	api.Get("/me/talent-network", mw.key, h.GetVisibility)
	api.Put("/me/talent-network", mw.cookie, h.PutVisibility)
}

// talentNetworkResponse is the public shape of the caller's Talent Network setting.
// public_id rides along on every read — including when visibility is "off" — so the
// client can render the shareable URL a candidate would get before they turn it on.
type talentNetworkResponse struct {
	Visibility string `json:"talent_network_visibility"`
	PublicID   string `json:"talent_network_public_id"`
}

func toTalentNetworkResponse(row db.GetTalentNetworkVisibilityRow) talentNetworkResponse {
	return talentNetworkResponse{Visibility: row.TalentNetworkVisibility, PublicID: row.TalentNetworkPublicID.String()}
}

// GetVisibility returns the authenticated caller's current Talent Network visibility
// and public id. A user who has never touched the setting reads "off" — the column
// default, not a sentinel the handler manufactures.
func (h *talentNetworkHandlers) GetVisibility(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	row, err := h.store.GetTalentNetworkVisibility(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toTalentNetworkResponse(row)})
}

// setTalentNetworkRequest is the PUT body: one of "off", "public", "anonymous".
type setTalentNetworkRequest struct {
	Visibility string `json:"visibility"`
}

// PutVisibility updates the authenticated caller's own Talent Network visibility.
// Any value outside the three valid strings is a 400 and never reaches the store.
// Cookie-only (see register).
func (h *talentNetworkHandlers) PutVisibility(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in setTalentNetworkRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if !talentNetworkVisibilityValues[in.Visibility] {
		return fiber.NewError(fiber.StatusBadRequest, "visibility must be one of off, public, anonymous")
	}

	if err := h.store.SetTalentNetworkVisibility(c.Context(), db.SetTalentNetworkVisibilityParams{
		ID:                      userID,
		TalentNetworkVisibility: in.Visibility,
	}); err != nil {
		return err
	}

	row, err := h.store.GetTalentNetworkVisibility(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toTalentNetworkResponse(row)})
}
