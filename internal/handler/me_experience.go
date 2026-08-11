package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/experience"
)

// experienceHandlers serve the owner's view of their experience bank: what has been
// recorded about them, and the ability to correct, merge or remove any of it.
//
// This surface is the reason the bank is allowed to be written by an agent at all. A
// system that records claims about a person and gives them no way to see or delete those
// claims is a trust problem before it is a compliance one — and provenance is served on
// every entry precisely so the distinction the model cannot fake is visible to the person
// it describes.
type experienceHandlers struct {
	bank           experienceBankOwner
	requireContext experienceRequireContext
}

// experienceBankOwner is what the owner-facing surface needs from the bank: read it all,
// correct an entry, merge a pair, remove one. Narrow handler-side interface, as
// everywhere else in this layer — and it is what lets the routes be tested without a
// database.
type experienceBankOwner interface {
	ListEmployments(ctx context.Context, userID int64) ([]experience.Employment, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (experience.Atom, error)
	CreateEmployment(ctx context.Context, userID int64, e experience.Employment) (experience.Employment, error)
	UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e experience.Employment) (experience.Employment, error)
	DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) error
	AddAtom(ctx context.Context, userID int64, a experience.Atom) (experience.Atom, error)
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a experience.Atom) (experience.Atom, error)
	DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) error
	MergeAtoms(ctx context.Context, userID int64, a, b uuid.UUID) (experience.Atom, error)
}

// experienceRequireContext is the per-user opt-in that gates interactive atom creates.
// Nil means ungated (tests, or a deploy that has not wired the flag reader).
type experienceRequireContext interface {
	GetUserExperienceRequireContext(ctx context.Context, userID int64) (bool, error)
}

func newExperienceHandlers(bank experienceBankOwner) *experienceHandlers {
	return &experienceHandlers{bank: bank}
}

func (h *experienceHandlers) withRequireContext(r experienceRequireContext) *experienceHandlers {
	h.requireContext = r
	return h
}

func (h *experienceHandlers) register(api fiber.Router, mw middleware) {
	// The read takes a key, like the profile read, so a programmatic consumer can ground
	// itself in what the candidate has actually done. Correcting, merging or removing an
	// EXISTING entry stays cookie-only: a key that leaks out of a script's environment
	// must not edit or erase someone's career. Creating a new one takes a key too — it is
	// additive-only, and a full-scope key already reaches everything more destructive than
	// this (cv edit, apply) through the rest of the CLI's surface, so gating this narrower
	// than that would protect a boundary the same key crosses everywhere else.
	api.Get("/me/experience", mw.key, h.ListExperience)
	api.Post("/me/experience/employments", mw.key, h.AddEmployment)
	api.Put("/me/experience/employments/:id", mw.cookie, h.UpdateEmployment)
	api.Delete("/me/experience/employments/:id", mw.cookie, h.DeleteEmployment)
	api.Post("/me/experience/atoms", mw.key, h.AddAtom)
	api.Post("/me/experience/atoms/merge", mw.cookie, h.MergeAtoms)
	api.Put("/me/experience/atoms/:id", mw.cookie, h.UpdateAtom)
	api.Delete("/me/experience/atoms/:id", mw.cookie, h.DeleteAtom)
}

// experienceResponse is the bank as its owner sees it: the places, and the achievements
// grouped under the place each belongs to. Achievements belonging to no place come back
// under `unplaced` rather than being hidden — they are usually the ones volunteered in
// conversation, which makes them the most important ones to be able to check.
type experienceResponse struct {
	Employments []employmentWithAtoms `json:"employments"`
	Unplaced    []atomView            `json:"unplaced"`
}

type employmentWithAtoms struct {
	experience.Employment
	Atoms []atomView `json:"atoms"`
}

// MarshalJSON flattens the employment wire shape with atoms. Employment's own
// MarshalJSON would otherwise replace the whole object and drop atoms (Go embeds a
// Marshaler as the sole encoded value for the outer struct).
func (w employmentWithAtoms) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(w.Employment)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	m["atoms"] = w.Atoms
	return json.Marshal(m)
}

// atomView is an atom plus read-time richness / soft-dup flags. Those flags are not
// fields on experience.Atom — the client must not POST them back.
type atomView struct {
	experience.Atom
	NeedsContext bool   `json:"needs_context"`
	NeedsMetrics bool   `json:"needs_metrics"`
	ClusterID    string `json:"cluster_id,omitempty"`
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
	var unplaced []experience.Atom
	for _, a := range atoms {
		// An achievement whose place is unknown is shown as unplaced rather than filed
		// under a heading that will never be rendered. The store refuses to create that
		// state, but grouping by a pointer without checking it means any state nobody
		// anticipated costs the owner an achievement they can no longer see or delete —
		// and this view exists precisely so nothing is invisible to them.
		if a.EmploymentID == nil || !known[*a.EmploymentID] {
			unplaced = append(unplaced, a)
			continue
		}
		grouped[*a.EmploymentID] = append(grouped[*a.EmploymentID], a)
	}

	views := projectAtomViews(atoms)
	resp := experienceResponse{
		Employments: make([]employmentWithAtoms, 0, len(employments)),
		Unplaced:    atomViewsFor(unplaced, views),
	}
	for _, e := range employments {
		entry := employmentWithAtoms{Employment: e, Atoms: atomViewsFor(grouped[e.ID], views)}
		if entry.Atoms == nil {
			entry.Atoms = []atomView{}
		}
		resp.Employments = append(resp.Employments, entry)
	}
	return c.JSON(fiber.Map{"data": resp})
}

// projectAtomViews builds the id→view map with richness and soft-dup cluster ids.
func projectAtomViews(atoms []experience.Atom) map[uuid.UUID]atomView {
	clusterOf := map[uuid.UUID]string{}
	for _, cluster := range experience.SoftDuplicateClusters(atoms) {
		if len(cluster) < 2 {
			continue
		}
		cid := cluster[0].String()
		for _, id := range cluster {
			clusterOf[id] = cid
		}
	}
	out := make(map[uuid.UUID]atomView, len(atoms))
	for _, a := range atoms {
		nc, nm := experience.Richness(a)
		out[a.ID] = atomView{
			Atom: a, NeedsContext: nc, NeedsMetrics: nm, ClusterID: clusterOf[a.ID],
		}
	}
	return out
}

func atomViewsFor(atoms []experience.Atom, views map[uuid.UUID]atomView) []atomView {
	if len(atoms) == 0 {
		return []atomView{}
	}
	out := make([]atomView, 0, len(atoms))
	for _, a := range atoms {
		if v, ok := views[a.ID]; ok {
			out = append(out, v)
			continue
		}
		nc, nm := experience.Richness(a)
		out = append(out, atomView{Atom: a, NeedsContext: nc, NeedsMetrics: nm})
	}
	return out
}

// AddEmployment records a new place — a job or a project — under the caller. Unlike
// UpdateEmployment this creates rather than corrects, so nothing about provenance applies:
// an employment carries no claim of its own, only the atoms attached to it do.
func (h *experienceHandlers) AddEmployment(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in experience.Employment
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	created, err := h.bank.CreateEmployment(c.Context(), userID, in)
	if err != nil {
		return experienceError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": created})
}

// AddAtom records a new achievement under the caller. This is the only route that can add
// evidence outside a chat session — the assistant's own experience_add tool is the other —
// so provenance is forced to manual regardless of what the body sends, the same way
// UpdateAtom already forces it on a correction: an authenticated POST with no chat
// transcript behind it can only ever honestly be "the owner typed this themselves".
func (h *experienceHandlers) AddAtom(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in experience.Atom
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Provenance = experience.ProvenanceManual
	if err := h.checkContextRequired(c.Context(), userID, in.Context); err != nil {
		return experienceError(err)
	}
	created, err := h.bank.AddAtom(c.Context(), userID, in)
	if err != nil {
		return experienceError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": created})
}

// MergeAtoms folds two owned atoms into one richer keep. Cookie-only — merge deletes.
func (h *experienceHandlers) MergeAtoms(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if len(in.IDs) != 2 {
		return experienceError(experience.ErrInvalidMerge)
	}
	a, err := uuid.Parse(strings.TrimSpace(in.IDs[0]))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	b, err := uuid.Parse(strings.TrimSpace(in.IDs[1]))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	kept, err := h.bank.MergeAtoms(c.Context(), userID, a, b)
	if err != nil {
		return experienceError(err)
	}
	return c.JSON(fiber.Map{"data": kept})
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

// checkContextRequired refuses an interactive create with empty context when the owner
// has opted in. Import and UpdateAtom never call this.
func (h *experienceHandlers) checkContextRequired(ctx context.Context, userID int64, contextText string) error {
	if h.requireContext == nil || strings.TrimSpace(contextText) != "" {
		return nil
	}
	on, err := h.requireContext.GetUserExperienceRequireContext(ctx, userID)
	if err != nil {
		return err
	}
	if on {
		return experience.ErrContextRequired
	}
	return nil
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
		errors.Is(err, experience.ErrInvalidKind),
		errors.Is(err, experience.ErrInvalidMerge),
		errors.Is(err, experience.ErrMergeCrossEmployment),
		errors.Is(err, experience.ErrContextRequired):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, experience.ErrAlreadyBanked):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	default:
		return err
	}
}
