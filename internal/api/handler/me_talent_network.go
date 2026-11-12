package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *talentNetworkHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The read takes a key (like GET /me/profile) so a script or the CLI can ground
	// itself in the caller's current setting. The write stays cookie-only — a leaked API
	// key must not be able to flip a candidate's visibility to the public internet
	// (see me_profile.go's PUT/DELETE comment for the same reasoning applied there).
	api.Get("/me/talent-network", mw.key, contract)
	api.Put("/me/talent-network", mw.cookie, contract)
}

func talentNetworkMembershipOut(row db.GetTalentNetworkVisibilityRow) apigen.TalentNetworkMembership {
	return apigen.TalentNetworkMembership{
		TalentNetworkVisibility: apigen.TalentNetworkVisibility(row.TalentNetworkVisibility),
		TalentHandle:            optString(row.TalentHandle.String),
		Listed:                  row.Listed,
	}
}

// GetVisibility returns the authenticated caller's current Talent Network visibility
// and public id. A user who has never touched the setting reads "off" — the column
// default, not a sentinel the handler manufactures.
func (h *talentNetworkHandlers) GetTalentNetworkMembership(ctx context.Context) (*apigen.GetTalentNetworkMembershipOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	row, err := h.store.GetTalentNetworkVisibility(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetTalentNetworkMembershipOK{Data: talentNetworkMembershipOut(row)}, nil
}

// PutVisibility updates the authenticated caller's own Talent Network membership.
// Any value outside the two valid strings is a 400 and never reaches the store.
// Cookie-only (see register).
func (h *talentNetworkHandlers) SetTalentNetworkVisibility(ctx context.Context, req *apigen.TalentNetworkVisibilityUpdate) (*apigen.SetTalentNetworkVisibilityOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	visibility := req.Visibility.Value
	if !talentNetworkVisibilityValues[visibility] {
		return nil, fiber.NewError(fiber.StatusBadRequest, "visibility must be one of off, anonymous")
	}

	// Mint BEFORE the visibility write, not after. A member without a handle is a
	// member the catalogue cannot link to, so a failure here must leave the candidate
	// outside the network rather than inside it with no address.
	if visibility != visibilityOff {
		if err := talentnetwork.Join(ctx, h.store, userID); err != nil {
			return nil, err
		}
	}

	if err := h.store.SetTalentNetworkVisibility(ctx, db.SetTalentNetworkVisibilityParams{
		ID:                      userID,
		TalentNetworkVisibility: visibility,
	}); err != nil {
		return nil, err
	}

	row, err := h.store.GetTalentNetworkVisibility(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.SetTalentNetworkVisibilityOK{Data: talentNetworkMembershipOut(row)}, nil
}
