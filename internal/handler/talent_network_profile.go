package handler

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// talentNetworkPublicStore is the slice of *db.Queries the public Talent Network page
// needs, kept narrow so the handler is unit-testable without a database.
type talentNetworkPublicStore interface {
	GetTalentNetworkProfileByPublicID(ctx context.Context, talentNetworkPublicID uuid.UUID) (db.GetTalentNetworkProfileByPublicIDRow, error)
}

// talentNetworkProfileHandlers serves the PUBLIC, unauthenticated Talent Network
// profile page — the shareable counterpart to talentNetworkHandlers' owner-only
// settings endpoint (me_talent_network.go). Nothing here requires a session: the whole
// point of this route is a link a candidate can hand to someone who has never signed
// in, so it takes no `mw` middleware at all.
type talentNetworkProfileHandlers struct {
	store talentNetworkPublicStore
}

func newTalentNetworkProfileHandlers(store talentNetworkPublicStore) *talentNetworkProfileHandlers {
	return &talentNetworkProfileHandlers{store: store}
}

func (h *talentNetworkProfileHandlers) register(api fiber.Router) {
	// :publicID is talent_network_public_id (opaque by construction — see the
	// migration), never users.id.
	api.Get("/talent-network/:publicID", h.GetProfile)
}

// talentNetworkProfileResponse is the wire shape of the public page. It mirrors
// profileResponse's (me_profile.go) split between the user_profiles facets and the CV
// projection, nesting the CV under "cv" rather than flattening it — resumeextract.
// Professional already has its own `skills` field, which would otherwise collide with
// the user_profiles `skills` facet at the top level.
//
// full_name is populated only in "public" mode; GetProfile leaves it as the zero value
// for "anonymous", which json's `omitempty` then drops from the response entirely —
// there is no name field to accidentally leak.
type talentNetworkProfileResponse struct {
	FullName        string                     `json:"full_name,omitempty"`
	Specializations []string                   `json:"specializations"`
	Skills          []string                   `json:"skills"`
	CV              resumeextract.Professional `json:"cv"`
}

// talentNetworkPublicID parses the :publicID route param as a UUID. A malformed value
// cannot name any profile, so it is reported as missing rather than as a bad request —
// mirrors cvPathID (cv.go) — so a probe cannot distinguish "not a UUID" from "no such
// profile".
func talentNetworkPublicID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("publicID"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return id, nil
}

// GetProfile serves the public Talent Network page for one candidate. "off" and "no
// such id" answer with the identical 404 body — the route must not let a caller
// distinguish a hidden profile from a nonexistent one (see design.md, "404, not 403,
// for off and not-found").
func (h *talentNetworkProfileHandlers) GetProfile(c *fiber.Ctx) error {
	id, err := talentNetworkPublicID(c)
	if err != nil {
		return err
	}

	row, err := h.store.GetTalentNetworkProfileByPublicID(c.Context(), id)
	if err != nil {
		// A miss (pgx.ErrNoRows) propagates as-is: RenderError already turns it into
		// {"error":"not found"} at 404, byte-identical to the "off" branch below.
		return err
	}
	if row.TalentNetworkVisibility == "off" {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}

	// A missing, empty, or corrupt structured CV is treated as absent rather than an
	// error — a candidate can enable visibility before finishing their CV (see
	// spec.md, "Enabling visibility does not require a complete CV"). This mirrors
	// resume.Store.Structured's same treatment of an unmarshal failure.
	var structured resumeextract.Structured
	if len(row.ResumeStructured) > 0 {
		_ = json.Unmarshal(row.ResumeStructured, &structured)
	}

	resp := talentNetworkProfileResponse{
		Specializations: row.Specializations,
		Skills:          row.Skills,
	}
	if row.TalentNetworkVisibility == "anonymous" {
		resp.CV = structured.Anonymous()
	} else {
		pub := structured.Public()
		resp.FullName = pub.FullName
		resp.CV = pub.Professional
	}

	return c.JSON(fiber.Map{"data": resp})
}
