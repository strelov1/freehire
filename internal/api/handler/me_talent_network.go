package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/platform/db"
)

// visibilityOff is the stored value for "not a member". Named because a bare "off" in a
// condition reads like a comment rather than a state.
const visibilityOff = "off"

// talentNetworkVisibilityValues are the only values SetTalentNetworkVisibility accepts.
// Kept as a set here (not a shared vocab package) because the enum's authority is the
// Postgres CHECK on users.talent_network_visibility (migration 0148) — this mirrors it
// for a cheap 400 without a round trip, not the other way around.
//
// Two values, not three: 'public' was retired with the mode picker itself. The product no
// longer asks a candidate how much of themselves to disclose — the public projection is
// fixed and anonymised, and membership is the only decision left. A stale client still
// sending 'public' gets a 400 from here rather than a 500 from the constraint.
var talentNetworkVisibilityValues = map[string]bool{visibilityOff: true, "anonymous": true}

// talentNetworkStore is the slice of *db.Queries the owner-facing visibility endpoint
// needs, kept narrow so the handler is unit-testable without a database.
//
// It embeds talentnetwork.JoinStore rather than restating its three methods: what a join
// reads is that package's business, and a second copy of the list here would be a second
// answer to it.
type talentNetworkStore interface {
	talentnetwork.JoinStore
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
//
// talent_handle is empty for an account that has never joined — a handle is minted on
// the first join and kept forever after, so "empty" means "not a member yet", never
// "waiting for one". The client renders the public URL from it and shows nothing when
// it is empty, rather than inventing a placeholder link that resolves to a 404.
type talentNetworkResponse struct {
	Visibility string `json:"talent_network_visibility"`
	Handle     string `json:"talent_handle,omitempty"`

	// Listed says whether a VISITOR can actually see them, which is not the same question
	// as membership and comes apart in an ordinary way: a candidate who joins before
	// uploading a CV is a member, holds a handle, and is still excluded from the
	// catalogue by the stamp gate — so their card 404s.
	//
	// The settings page needs this and not the membership flag, or it tells somebody
	// their profile is up while linking them to a 404. It is the field that stops the
	// page from lying.
	Listed bool `json:"listed"`
}

func toTalentNetworkResponse(row db.GetTalentNetworkVisibilityRow) talentNetworkResponse {
	return talentNetworkResponse{
		Visibility: row.TalentNetworkVisibility,
		Handle:     row.TalentHandle.String,
		Listed:     row.Listed,
	}
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

// setTalentNetworkRequest is the PUT body: either "off" or "anonymous".
type setTalentNetworkRequest struct {
	Visibility string `json:"visibility"`
}

// PutVisibility updates the authenticated caller's own Talent Network membership.
// Any value outside the two valid strings is a 400 and never reaches the store.
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
		return fiber.NewError(fiber.StatusBadRequest, "visibility must be one of off, anonymous")
	}

	// Mint BEFORE the visibility write, not after. A member without a handle is a member
	// the catalogue cannot link to, so a failure here must leave the candidate outside
	// the network rather than inside it with no address.
	if in.Visibility != visibilityOff {
		if err := talentnetwork.Join(c.Context(), h.store, userID); err != nil {
			return err
		}
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
