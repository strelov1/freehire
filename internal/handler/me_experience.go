package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/experience"
)

// experienceHandlers serve the owner's view of their experience bank: what has been
// recorded about them, and the ability to correct or remove any of it.
//
// This surface is the reason the bank is allowed to be written by an agent at all. A
// system that records claims about a person and gives them no way to see or delete those
// claims is a trust problem before it is a compliance one — and provenance is served on
// every entry precisely so the distinction the model cannot fake is visible to the person
// it describes.
type experienceHandlers struct{ bank experienceBankOwner }

// experienceBankOwner is what the owner-facing surface needs from the bank: read it all,
// correct an entry, remove one. Narrow handler-side interface, as everywhere else in this
// layer — and it is what lets the routes be tested without a database.
type experienceBankOwner interface {
	ListEmployments(ctx context.Context, userID int64) ([]experience.Employment, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (experience.Atom, error)
	UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e experience.Employment) (experience.Employment, error)
	DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) error
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a experience.Atom) (experience.Atom, error)
	DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) error
}

func newExperienceHandlers(bank experienceBankOwner) *experienceHandlers {
	return &experienceHandlers{bank: bank}
}

func (h *experienceHandlers) register(api fiber.Router, mw middleware) {
	// The read takes a key, like the profile read, so a programmatic consumer can ground
	// itself in what the candidate has actually done. The writes stay cookie-only: a key
	// that leaks out of a script's environment must not edit or erase someone's career.
	api.Get("/me/experience", mw.key, h.ListExperience)
	api.Put("/me/experience/employments/:id", mw.cookie, h.UpdateEmployment)
	api.Delete("/me/experience/employments/:id", mw.cookie, h.DeleteEmployment)
	api.Put("/me/experience/atoms/:id", mw.cookie, h.UpdateAtom)
	api.Delete("/me/experience/atoms/:id", mw.cookie, h.DeleteAtom)
}

// experienceResponse is the bank as its owner sees it: the places, and the achievements
// grouped under the place each belongs to. Achievements belonging to no place come back
// under `unplaced` rather than being hidden — they are usually the ones volunteered in
// conversation, which makes them the most important ones to be able to check.
type experienceResponse struct {
	Employments []employmentWithAtoms `json:"employments"`
	Unplaced    []experience.Atom     `json:"unplaced"`
}

type employmentWithAtoms struct {
	experience.Employment
	Atoms []experience.Atom `json:"atoms"`
}

// ListExperience returns the caller's whole bank. Cookie or key.
func (h *experienceHandlers) ListExperience(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	employments, err := h.bank.ListEmployments(c.Context(), userID)
	if err != nil {
		return err
	}
	atoms, err := h.bank.ListAtoms(c.Context(), userID)
	if err != nil {
		return err
	}

	known := make(map[uuid.UUID]bool, len(employments))
	for _, e := range employments {
		known[e.ID] = true
	}

	grouped := make(map[uuid.UUID][]experience.Atom, len(employments))
	resp := experienceResponse{
		Employments: make([]employmentWithAtoms, 0, len(employments)),
		Unplaced:    []experience.Atom{},
	}
	for _, a := range atoms {
		// An achievement whose place is unknown is shown as unplaced rather than filed
		// under a heading that will never be rendered. The store refuses to create that
		// state, but grouping by a pointer without checking it means any state nobody
		// anticipated costs the owner an achievement they can no longer see or delete —
		// and this view exists precisely so nothing is invisible to them.
		if a.EmploymentID == nil || !known[*a.EmploymentID] {
			resp.Unplaced = append(resp.Unplaced, a)
			continue
		}
		grouped[*a.EmploymentID] = append(grouped[*a.EmploymentID], a)
	}
	for _, e := range employments {
		entry := employmentWithAtoms{Employment: e, Atoms: grouped[e.ID]}
		if entry.Atoms == nil {
			entry.Atoms = []experience.Atom{}
		}
		resp.Employments = append(resp.Employments, entry)
	}
	return c.JSON(fiber.Map{"data": resp})
}

// UpdateEmployment replaces one place's fields with what the owner typed — blanks
// included. This is the user editing their own history, so unlike import it means exactly
// what it says.
func (h *experienceHandlers) UpdateEmployment(c *fiber.Ctx) error {
	userID, id, err := ownedEntry(c)
	if err != nil {
		return err
	}
	var in experience.Employment
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	updated, err := h.bank.UpdateEmployment(c.Context(), id, userID, in)
	if err != nil {
		return experienceError(err)
	}
	return c.JSON(fiber.Map{"data": updated})
}

// DeleteEmployment removes a place and, with it, the achievements that were evidence of
// it. Idempotent from the caller's side: a second delete is a 404, never a 500.
func (h *experienceHandlers) DeleteEmployment(c *fiber.Ctx) error {
	userID, id, err := ownedEntry(c)
	if err != nil {
		return err
	}
	if err := h.bank.DeleteEmployment(c.Context(), id, userID); err != nil {
		return experienceError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// UpdateAtom rewrites one achievement. Editing it makes it the owner's own statement —
// they typed it — so the provenance becomes `manual` regardless of how it arrived. That is
// the confirmation path for something the agent inferred: correct it, and it becomes
// yours.
func (h *experienceHandlers) UpdateAtom(c *fiber.Ctx) error {
	userID, id, err := ownedEntry(c)
	if err != nil {
		return err
	}
	var in experience.Atom
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Provenance = experience.ProvenanceManual
	updated, err := h.bank.UpdateAtom(c.Context(), id, userID, in)
	if err != nil {
		return experienceError(err)
	}
	return c.JSON(fiber.Map{"data": updated})
}

// DeleteAtom removes one achievement. This is the only path that takes evidence out of the
// bank — import never deletes — so it belongs to the owner and nobody else.
func (h *experienceHandlers) DeleteAtom(c *fiber.Ctx) error {
	userID, id, err := ownedEntry(c)
	if err != nil {
		return err
	}
	if err := h.bank.DeleteAtom(c.Context(), id, userID); err != nil {
		return experienceError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ownedEntry resolves the caller and the addressed entry id together, since every route
// here needs both and neither is useful alone.
func ownedEntry(c *fiber.Ctx) (int64, uuid.UUID, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return 0, uuid.Nil, err
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		// A malformed id is indistinguishable from an id that does not exist, and saying
		// so is the same disclosure choice ownership already makes.
		return 0, uuid.Nil, fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return userID, id, nil
}

// experienceError maps the bank's sentinels onto HTTP. ErrNotFound covers both "no such
// entry" and "not yours" — the two are deliberately indistinguishable.
func experienceError(err error) error {
	switch {
	case errors.Is(err, experience.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, experience.ErrEmptyClaim),
		errors.Is(err, experience.ErrInvalidProvenance),
		errors.Is(err, experience.ErrEmptyEmployment),
		errors.Is(err, experience.ErrInvalidKind):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}
