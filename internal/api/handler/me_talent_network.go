package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// visibilityOff is the stored value for "not a member". Named because three places read
// it and a bare "off" in a condition reads like a comment rather than a state.
const visibilityOff = "off"

// talentNetworkVisibilityValues are the only values SetTalentNetworkVisibility accepts.
// Kept as a set here (not a shared vocab package) because the enum's authority is the
// Postgres CHECK on users.talent_network_visibility (migration 0145) — this mirrors it
// for a cheap 400 without a round trip, not the other way around.
//
// Two values, not three: 'public' was retired with the mode picker itself. The product no
// longer asks a candidate how much of themselves to disclose — the public projection is
// fixed and anonymised, and membership is the only decision left. A stale client still
// sending 'public' gets a 400 from here rather than a 500 from the constraint.
var talentNetworkVisibilityValues = map[string]bool{visibilityOff: true, "anonymous": true}

// talentNetworkStore is the slice of *db.Queries the owner-facing visibility endpoint
// needs, kept narrow so the handler is unit-testable without a database.
type talentNetworkStore interface {
	GetTalentNetworkVisibility(ctx context.Context, id int64) (db.GetTalentNetworkVisibilityRow, error)
	SetTalentNetworkVisibility(ctx context.Context, arg db.SetTalentNetworkVisibilityParams) error
	GetUserResumeStructuredOnly(ctx context.Context, id int64) ([]byte, error)
	SetTalentHandleIfUnset(ctx context.Context, arg db.SetTalentHandleIfUnsetParams) (int64, error)
}

// handleMintAttempts bounds the re-mint loop when a freshly minted handle collides with
// one another account already holds. The suffix draws from ~1M values per base, so two
// collisions in a row is already implausible; the bound is here so a store that fails
// every claim ends the request instead of the loop.
const handleMintAttempts = 5

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
	PublicID   string `json:"talent_network_public_id"`
	Handle     string `json:"talent_handle,omitempty"`
}

func toTalentNetworkResponse(row db.GetTalentNetworkVisibilityRow) talentNetworkResponse {
	return talentNetworkResponse{
		Visibility: row.TalentNetworkVisibility,
		PublicID:   row.TalentNetworkPublicID.String(),
		Handle:     row.TalentHandle.String,
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
		if err := h.ensureHandle(c.Context(), userID); err != nil {
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

// ensureHandle mints and stores the caller's catalogue handle if they do not have one.
//
// It is idempotent by the claim's own predicate (`talent_handle IS NULL`), which is what
// makes leaving and rejoining keep the URL a candidate already shared: the second join
// claims nothing and the stored handle stands. A collision with another account's handle
// comes back as a unique violation and is answered by minting a new suffix — the same
// allocate-and-retry shape internal/identity/accounts uses for usernames.
func (h *talentNetworkHandlers) ensureHandle(ctx context.Context, userID int64) error {
	row, err := h.store.GetTalentNetworkVisibility(ctx, userID)
	if err != nil {
		return err
	}
	if row.TalentHandle.Valid && row.TalentHandle.String != "" {
		return nil
	}

	title, err := h.primaryTitle(ctx, userID)
	if err != nil {
		return err
	}

	for attempt := 0; attempt < handleMintAttempts; attempt++ {
		handle, err := talentnetwork.MintHandle(title)
		if err != nil {
			return err
		}
		_, err = h.store.SetTalentHandleIfUnset(ctx, db.SetTalentHandleIfUnsetParams{
			ID:           userID,
			TalentHandle: pgtype.Text{String: handle, Valid: true},
		})
		switch {
		case err == nil:
			// Zero rows means the account already had a handle — a concurrent join won
			// the race — which is the same success as having written one.
			return nil
		case pgerr.IsUniqueViolation(err):
			continue
		default:
			return err
		}
	}
	return fmt.Errorf("talent network: could not mint a free handle in %d attempts", handleMintAttempts)
}

// primaryTitle reads the caller's stored CV and returns the title of the role that best
// describes what they do now. An unreadable or absent CV yields "" and is not an error:
// MintHandle falls back to a neutral base, because being unable to name somebody's
// discipline is not a reason to refuse them a URL.
func (h *talentNetworkHandlers) primaryTitle(ctx context.Context, userID int64) (string, error) {
	raw, err := h.store.GetUserResumeStructuredOnly(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}
	var structured resumeextract.Structured
	if err := json.Unmarshal(raw, &structured); err != nil {
		// A stored structure we cannot parse is a broken extract, not a broken join.
		return "", nil
	}
	return talentnetwork.PrimaryTitle(structured), nil
}
