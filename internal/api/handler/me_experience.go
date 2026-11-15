package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// experienceHandlers serve the owner's view of their experience bank: what has been
// recorded about them, and the ability to correct, merge or remove any of it. Provenance
// is served on every entry so the owner can see what the model, not they, asserted.
type experienceHandlers struct {
	bank           experienceBankOwner
	requireContext experienceRequireContext
}

// experienceBankOwner is what the owner-facing surface needs from the bank. Narrow
// handler-side interface so the routes are testable without a database.
type experienceBankOwner interface {
	ListEmployments(ctx context.Context, userID int64) ([]experience.Employment, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (experience.Atom, error)
	CreateEmployment(ctx context.Context, userID int64, e experience.Employment) (experience.Employment, error)
	UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e experience.Employment) (experience.Employment, error)
	DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) error
	AddAtom(ctx context.Context, userID int64, a experience.Atom, author experience.Author) (experience.Atom, error)
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a experience.Atom, author experience.Author) (experience.Atom, error)
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

func (h *experienceHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Reads, creates, corrects and deletes take a key. UpdateAtom keeps a keyed caller's edit
	// from moving the claim's provenance (see updateAuthor), so a key cannot launder an
	// inference into `manual`.
	//
	// DeleteEmployment cascades to its achievements (experience_atoms.employment_id, migration
	// 0047) and the bank has no undo, so it refuses a keyed caller while the place is occupied.
	//
	// The merge stays cookie-only: unionForMerge folds the discarded atom's metrics and skills
	// into the kept one while the kept one's provenance stands, so a `manual` atom could
	// absorb numbers an agent invented.
	api.Get("/me/experience", mw.key, contract)
	api.Post("/me/experience/employments", mw.key, contract)
	api.Put("/me/experience/employments/:id", mw.key, contract)
	api.Delete("/me/experience/employments/:id", mw.key, contract)
	api.Post("/me/experience/atoms", mw.key, contract)
	api.Post("/me/experience/atoms/merge", mw.cookie, contract)
	api.Put("/me/experience/atoms/:id", mw.key, contract)
	api.Delete("/me/experience/atoms/:id", mw.key, contract)
}

// Achievements belonging to no place come back under `unplaced` rather than being hidden:
// they are usually the ones volunteered in conversation, so the owner most needs to check them.
// atomView is an atom plus read-time richness / soft-dup flags. Those flags are not
// fields on experience.Atom — the client must not POST them back.
type atomView struct {
	experience.Atom
	NeedsContext bool   `json:"needs_context"`
	NeedsMetrics bool   `json:"needs_metrics"`
	ClusterID    string `json:"cluster_id,omitempty"`
}

// ListExperience returns the caller's whole bank. Cookie or key.
func (h *experienceHandlers) ListExperience(ctx context.Context) (*apigen.ListExperienceOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	employments, err := h.bank.ListEmployments(ctx, userID)
	if err != nil {
		return nil, err
	}
	atoms, err := h.bank.ListAtoms(ctx, userID)
	if err != nil {
		return nil, err
	}

	known := make(map[uuid.UUID]bool, len(employments))
	for _, e := range employments {
		known[e.ID] = true
	}

	grouped := make(map[uuid.UUID][]experience.Atom, len(employments))
	var unplaced []experience.Atom
	for _, a := range atoms {
		// An achievement whose place is unknown is shown as unplaced, never dropped: the owner
		// must always be able to see and delete every entry.
		if a.EmploymentID == nil || !known[*a.EmploymentID] {
			unplaced = append(unplaced, a)
			continue
		}
		grouped[*a.EmploymentID] = append(grouped[*a.EmploymentID], a)
	}

	views := projectAtomViews(atoms)
	resp := apigen.ExperienceBank{
		Employments: make([]apigen.ExperienceEmploymentWithAtoms, 0, len(employments)),
		Unplaced:    atomsOut(atomViewsFor(unplaced, views)),
	}
	for _, e := range employments {
		resp.Employments = append(resp.Employments, employmentOut(e, atomsOut(atomViewsFor(grouped[e.ID], views))))
	}
	return &apigen.ListExperienceOK{Data: resp}, nil
}

// employmentOut is the kind-aware wire shape of a place: jobs keep `company`, projects
// expose the same stored label as `name`.
func employmentOut(e experience.Employment, atoms []apigen.ExperienceAtom) apigen.ExperienceEmploymentWithAtoms {
	out := apigen.ExperienceEmploymentWithAtoms{
		ID: e.ID.String(), Kind: apigen.ExperienceEmploymentKind(e.Kind),
		Role: optString(e.Role), Location: optString(e.Location),
		Summary: optString(e.Summary), Link: optString(e.Link), Stack: nilIfEmpty(e.Stack),
		Atoms: atoms,
	}
	if e.Current {
		out.Current = apigen.NewOptBool(true)
	}
	if e.Start != nil {
		out.Start = apigen.NewOptPeriodDate(periodDateOut(*e.Start))
	}
	if e.End != nil {
		out.End = apigen.NewOptPeriodDate(periodDateOut(*e.End))
	}
	if e.Kind == experience.KindProject {
		out.Name = optString(e.Company)
	} else {
		out.Company = optString(e.Company)
	}
	return out
}

func periodDateOut(d perioddate.PeriodDate) apigen.PeriodDate {
	out := apigen.PeriodDate{Year: int32(d.Year)}
	if d.Month != 0 {
		out.Month = apigen.NewOptInt32(int32(d.Month))
	}
	return out
}

func atomsOut(views []atomView) []apigen.ExperienceAtom {
	out := make([]apigen.ExperienceAtom, 0, len(views))
	for _, v := range views {
		a := apigen.ExperienceAtom{
			ID: v.ID.String(), Claim: v.Claim, Context: optString(v.Context),
			Metrics: nilIfEmpty(v.Metrics), Skills: nilIfEmpty(v.Skills),
			Provenance: apigen.ExperienceProvenance(v.Provenance), SourceRef: optString(v.SourceRef),
			NeedsContext: v.NeedsContext, NeedsMetrics: v.NeedsMetrics, ClusterID: optString(v.ClusterID),
		}
		if v.EmploymentID != nil {
			a.EmploymentID = apigen.NewOptString(v.EmploymentID.String())
		}
		out = append(out, a)
	}
	return out
}

// marshalRaws writes each value as the JSON its type marshals to, for an entry whose shape
// belongs to the bank.
func marshalRaws[T any](vs []T) ([]jx.Raw, error) {
	out := make([]jx.Raw, 0, len(vs))
	for _, v := range vs {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
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

// AddEmployment records a new place under the caller. An employment carries no claim of its
// own, so provenance does not apply; only its atoms do.
func (h *experienceHandlers) AddEmployment(ctx context.Context, req jx.Raw) (*apigen.AddEmploymentCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var in experience.Employment
	if err := decodeBody(req, &in); err != nil {
		return nil, err
	}
	created, err := h.bank.CreateEmployment(ctx, userID, in)
	if err != nil {
		return nil, experienceError(err)
	}
	raw, err := json.Marshal(created)
	if err != nil {
		return nil, err
	}
	return &apigen.AddEmploymentCreated{Data: raw}, nil
}

// decodeBody reads a body that travels as raw JSON into the bank's own type, so its decoding
// rules stay the type's.
func decodeBody(raw jx.Raw, into any) error {
	if err := json.Unmarshal(raw, into); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	return nil
}

// AddAtom records a new achievement under the caller. Provenance is forced to manual
// regardless of the body: a POST with no chat transcript behind it can only honestly be
// "the owner typed this themselves".
func (h *experienceHandlers) AddAtom(ctx context.Context, req jx.Raw) (*apigen.AddAtomCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var in experience.Atom
	if err := decodeBody(req, &in); err != nil {
		return nil, err
	}
	if err := h.checkContextRequired(ctx, userID, in.Context); err != nil {
		return nil, experienceError(err)
	}
	// The door names who is asserting this; a provenance in the body is inert.
	created, err := h.bank.AddAtom(ctx, userID, in, experience.AuthorCandidate)
	if err != nil {
		return nil, experienceError(err)
	}
	raw, err := json.Marshal(created)
	if err != nil {
		return nil, err
	}
	return &apigen.AddAtomCreated{Data: raw}, nil
}

// MergeAtoms folds two owned atoms into one richer keep. Cookie-only — merge deletes.
func (h *experienceHandlers) MergeAtoms(ctx context.Context, req *apigen.MergeAtoms) (*apigen.MergeAtomsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	ids := req.Ids.Value
	if len(ids) != 2 {
		return nil, experienceError(experience.ErrInvalidMerge)
	}
	a, err := uuid.Parse(strings.TrimSpace(ids[0]))
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "not found")
	}
	b, err := uuid.Parse(strings.TrimSpace(ids[1]))
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "not found")
	}
	kept, err := h.bank.MergeAtoms(ctx, userID, a, b)
	if err != nil {
		return nil, experienceError(err)
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	return &apigen.MergeAtomsOK{Data: raw}, nil
}

// UpdateEmployment replaces one place's fields with what the owner typed, blanks included.
func (h *experienceHandlers) UpdateEmployment(ctx context.Context, req jx.Raw, params apigen.UpdateEmploymentParams) (*apigen.UpdateEmploymentOK, error) {
	userID, id, err := ownedEntry(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	var in experience.Employment
	if err := decodeBody(req, &in); err != nil {
		return nil, err
	}
	updated, err := h.bank.UpdateEmployment(ctx, id, userID, in)
	if err != nil {
		return nil, experienceError(err)
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateEmploymentOK{Data: raw}, nil
}

// DeleteEmployment removes a place and the achievements recorded under it. A second delete
// is a 404, never a 500.
func (h *experienceHandlers) DeleteEmployment(ctx context.Context, params apigen.DeleteEmploymentParams) error {
	userID, id, err := ownedEntry(ctx, params.ID)
	if err != nil {
		return err
	}
	if auth.ViaAPIKeyFromContext(ctx) {
		if err := h.refuseKeyedCascade(ctx, userID, id); err != nil {
			return err
		}
	}
	return experienceError(h.bank.DeleteEmployment(ctx, id, userID))
}

// refuseKeyedCascade stops a keyed caller from deleting a place that still holds
// achievements: the cascade lives in the schema (migration 0047) and cannot be undone, and a
// script's credential has not been told what it would lose. Unknown or foreign ids 404
// before the count, so the refusal cannot be used to probe which ids exist.
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
	// The message names the way out: the caller is usually an agent and this is its only
	// route to correcting itself inside the turn.
	return fiber.NewError(fiber.StatusConflict, fmt.Sprintf(
		"this place still holds %d achievement(s), and deleting it would delete them too. "+
			"Move them first (PUT /me/experience/atoms/:id with another employment_id, or "+
			"delete them one by one), or remove the place from the experience page on the site.", n))
}

// UpdateAtom rewrites one achievement. A cookie edit is the owner's own statement, so
// provenance becomes `manual`; this is how they confirm something the agent inferred.
func (h *experienceHandlers) UpdateAtom(ctx context.Context, req jx.Raw, params apigen.UpdateAtomParams) (*apigen.UpdateAtomOK, error) {
	userID, id, err := ownedEntry(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	var in experience.Atom
	if err := decodeBody(req, &in); err != nil {
		return nil, err
	}
	// A keyed caller may change what an achievement says, not who is held to have said it;
	// the bank reads the standing itself rather than trusting a label computed here.
	updated, err := h.bank.UpdateAtom(ctx, id, userID, in, updateAuthor(auth.ViaAPIKeyFromContext(ctx)))
	if err != nil {
		return nil, experienceError(err)
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateAtomOK{Data: raw}, nil
}

// DeleteAtom removes one achievement. The only path that takes evidence out of the bank
// (import never deletes), so it belongs to the owner.
func (h *experienceHandlers) DeleteAtom(ctx context.Context, params apigen.DeleteAtomParams) error {
	userID, id, err := ownedEntry(ctx, params.ID)
	if err != nil {
		return err
	}
	return experienceError(h.bank.DeleteAtom(ctx, id, userID))
}

// checkContextRequired refuses an interactive create with empty context when the owner
// has opted in. Import and UpdateAtom never call it.
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

// updateAuthor says who an edit is attributed to. The cookie path is the person, so
// AuthorCandidate. A key rewrites the words but leaves the provenance label untouched
// (AuthorRewrite): otherwise the tailoring agent could bank an inference as `agent_inferred`,
// PUT it, read back `manual`, and cite it past the CV evidence gate.
func updateAuthor(viaKey bool) experience.Author {
	if viaKey {
		return experience.AuthorRewrite
	}
	return experience.AuthorCandidate
}

// ownedEntry resolves the caller and the addressed entry id together.
func ownedEntry(ctx context.Context, rawID string) (int64, uuid.UUID, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return 0, uuid.Nil, err
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		// A malformed id looks the same as a missing one, like ownership.
		return 0, uuid.Nil, fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return userID, id, nil
}

// experienceError maps the bank's sentinels onto HTTP. ErrNotFound covers "no such entry"
// and "not yours" alike.
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
