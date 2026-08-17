package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/auth"
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
	// itself in what the candidate has actually done. Creating takes a key too — it is
	// additive-only, and a full-scope key already reaches everything more destructive than
	// this (cv edit, apply) through the rest of the CLI's surface.
	//
	// Correcting takes a key as well, but only because UpdateAtom below keeps a keyed
	// caller's edit from moving the claim's provenance. Without that, a key reaching this
	// route would be a laundering step rather than a correction — see provenanceForUpdate.
	// A caller able to ADD an achievement from a script and unable to fix a typo in it has
	// a bank it can only make worse: the duplicate check refuses the corrected retry.
	//
	// Deleting takes a key, but the CASCADE does not come with it. Removing an achievement
	// removes the row named and nothing else. Removing a place takes every achievement
	// under it (experience_atoms.employment_id is ON DELETE CASCADE, migration 0047), and
	// the bank has no undo — so DeleteEmployment refuses a keyed caller while the place is
	// still occupied. Move the achievements first, then remove the shell.
	//
	// The merge stays cookie-only, and unlike the deletes it cannot simply be widened:
	// unionForMerge folds the discarded atom's metrics and skills into the kept one while
	// the kept one's provenance stands, so a `manual` atom can absorb numbers an agent
	// invented. That has to be settled before the gate moves.
	api.Get("/me/experience", mw.key, h.ListExperience)
	api.Post("/me/experience/employments", mw.key, h.AddEmployment)
	api.Put("/me/experience/employments/:id", mw.key, h.UpdateEmployment)
	api.Delete("/me/experience/employments/:id", mw.key, h.DeleteEmployment)
	api.Post("/me/experience/atoms", mw.key, h.AddAtom)
	api.Post("/me/experience/atoms/merge", mw.cookie, h.MergeAtoms)
	api.Put("/me/experience/atoms/:id", mw.key, h.UpdateAtom)
	api.Delete("/me/experience/atoms/:id", mw.key, h.DeleteAtom)
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
	if auth.ViaAPIKey(c) {
		if err := h.refuseKeyedCascade(c.Context(), userID, id); err != nil {
			return err
		}
	}
	if err := h.bank.DeleteEmployment(c.Context(), id, userID); err != nil {
		return experienceError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// refuseKeyedCascade is the one thing a keyed caller may not do to the bank: take a place
// down and every achievement recorded under it with it.
//
// The cascade lives in the schema (migration 0047), so nothing downstream will stop it and
// nothing can put the rows back. A candidate doing this in the browser has been told what
// they are about to lose; a credential in a script's environment has not, and one wrong id
// there erases years of someone's record in a single request.
//
// Confining the keyed form to an already-empty place keeps the route useful — move the
// achievements with PUT, then remove the shell — while the destructive form stays where its
// consequence is visible. Unknown or foreign ids resolve to 404 here, before the count, so
// the refusal never doubles as a way to probe which ids exist.
func (h *experienceHandlers) refuseKeyedCascade(ctx context.Context, userID int64, id uuid.UUID) error {
	employments, err := h.bank.ListEmployments(ctx, userID)
	if err != nil {
		return err
	}
	found := false
	for _, e := range employments {
		if e.ID == id {
			found = true
			break
		}
	}
	if !found {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	atoms, err := h.bank.ListAtoms(ctx, userID)
	if err != nil {
		return err
	}
	n := 0
	for _, a := range atoms {
		if a.EmploymentID != nil && *a.EmploymentID == id {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	// The message names the way out, because the caller reading it is usually an agent and
	// this is its only route to correcting itself inside the turn.
	return fiber.NewError(fiber.StatusConflict, fmt.Sprintf(
		"this place still holds %d achievement(s), and deleting it would delete them too. "+
			"Move them first (PUT /me/experience/atoms/:id with another employment_id, or "+
			"delete them one by one), or remove the place from the experience page on the site.", n))
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
	// Read the standing of the claim before overwriting its words: a keyed caller may
	// change what an achievement says and may not change who is held to have said it.
	// The read also owner-scopes the write, so a foreign id is a 404 before any update.
	existing, err := h.bank.GetAtom(c.Context(), id, userID)
	if err != nil {
		return experienceError(err)
	}
	in.Provenance = provenanceForUpdate(auth.ViaAPIKey(c), existing.Provenance)
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
// provenanceForUpdate answers what an edit does to a claim's standing, and it is the reason
// correcting an achievement can be reached with an API key at all.
//
// A candidate editing their own entry IS asserting it — that is what `manual` records, and
// it is why the browser path stamps every correction with it regardless of where the row
// came from. The tailoring agent holds a key, and the same stamp on its path would let it
// promote its own reading: bank an inference as `agent_inferred` (which the CV evidence
// gate refuses), PUT it, read back `manual`, and cite it. So a keyed edit rewrites the
// words and leaves the label untouched.
//
// An unlabelled row falls to `agent_inferred` on the keyed path rather than to the strongest
// claim, because a missing label is not evidence that anyone said anything.
func provenanceForUpdate(viaKey bool, existing experience.Provenance) experience.Provenance {
	if !viaKey {
		return experience.ProvenanceManual
	}
	if !existing.Valid() {
		return experience.ProvenanceAgentInferred
	}
	return existing
}

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
	case errors.Is(err, experience.ErrAlreadyBanked),
		errors.Is(err, experience.ErrMergeConflict):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	default:
		return err
	}
}
