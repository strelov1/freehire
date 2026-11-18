package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-faster/jx"
	"github.com/strelov1/freehire/internal/api/apigen"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

type coverLetterEvidenceResponse struct {
	EmploymentLabels map[string]string `json:"employment_labels"`
	Revision         int64             `json:"revision"`
	Explicit         bool              `json:"explicit"`
	AtomIDs          []uuid.UUID       `json:"atom_ids"`
	SuggestedIDs     []uuid.UUID       `json:"suggested_ids"`
	Available        []experience.Atom `json:"available"`
	InvalidIDs       []uuid.UUID       `json:"invalid_ids"`
}

func mapLetterSelectionError(err error) error {
	if errors.Is(err, coverletter.ErrInvalidSelection) || errors.Is(err, coverletter.ErrSelectionConflict) {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	return err
}

func (h *cvHandlers) GetCVCoverLetterEvidence(ctx context.Context, params apigen.GetCVCoverLetterEvidenceParams) (*apigen.GetCVCoverLetterEvidenceOK, error) {
	userID, jobID, err := h.coverLetterTargetOf(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	if h.letter.letters == nil || h.letter.bank == nil {
		return nil, fiber.ErrServiceUnavailable
	}
	bank, err := h.letter.bank.ListAtoms(ctx, userID)
	if err != nil {
		return nil, err
	}
	selected, err := h.letter.letters.GetSelection(ctx, userID, jobID)
	if err != nil {
		return nil, err
	}
	out := coverLetterEvidenceResponse{AtomIDs: []uuid.UUID{}, SuggestedIDs: []uuid.UUID{}, Available: []experience.Atom{}, InvalidIDs: []uuid.UUID{}}
	roles, err := h.letter.bank.ListEmployments(ctx, userID)
	if err != nil {
		return nil, err
	}
	out.EmploymentLabels = make(map[string]string, len(roles))
	for _, role := range roles {
		label := role.Company
		if label == "" {
			label = role.Role
		} else if role.Role != "" {
			label += " · " + role.Role
		}
		out.EmploymentLabels[role.ID.String()] = label
	}
	known := make(map[uuid.UUID]bool)
	for _, a := range bank {
		if a.Provenance.Publishable() {
			out.Available = append(out.Available, a)
			known[a.ID] = true
		}
	}
	if selected != nil {
		out.Explicit = true
		out.Revision = selected.Revision
		out.AtomIDs = selected.AtomIDs
		for _, id := range selected.AtomIDs {
			if !known[id] {
				out.InvalidIDs = append(out.InvalidIDs, id)
			}
		}
	}
	var reqs []matchanalysis.Requirement
	if h.fit != nil && h.jobReader != nil {
		job, e := h.jobReader.GetJob(ctx, jobID)
		if e != nil {
			return nil, e
		}
		tailoring, e := h.fit.TailoringContext(ctx, userID, job, nil)
		if e != nil && !errors.Is(e, fitanalysis.ErrNoAnalysis) {
			return nil, e
		}
		if e == nil {
			reqs = tailoring.MissingHave
		}
	}
	suggested, err := coverletter.Gather(ctx, h.letter.bank, userID, reqs)
	if err != nil {
		return nil, err
	}
	for _, a := range suggested {
		if known[a.ID] && len(out.SuggestedIDs) < coverletter.MaxSelectedAtoms {
			out.SuggestedIDs = append(out.SuggestedIDs, a.ID)
		}
	}
	raw, err := json.Marshal(out)
	return &apigen.GetCVCoverLetterEvidenceOK{Data: raw}, err
}

func (h *cvHandlers) SaveCVCoverLetterEvidence(ctx context.Context, req jx.Raw, params apigen.SaveCVCoverLetterEvidenceParams) (*apigen.SaveCVCoverLetterEvidenceOK, error) {
	userID, jobID, err := h.coverLetterTargetOf(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	if h.letter.letters == nil || h.letter.bank == nil {
		return nil, fiber.ErrServiceUnavailable
	}
	if len(req) > 8192 {
		return nil, fiber.ErrRequestEntityTooLarge
	}
	var body struct {
		Revision *int64       `json:"revision"`
		AtomIDs  *[]uuid.UUID `json:"atom_ids"`
	}
	if err := json.Unmarshal(req, &body); err != nil || body.Revision == nil || body.AtomIDs == nil {
		return nil, fiber.NewError(422, "revision and atom_ids are required")
	}
	bank, err := h.letter.bank.ListAtoms(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := h.letter.letters.SaveSelection(ctx, userID, jobID, *body.Revision, *body.AtomIDs, bank)
	if errors.Is(err, coverletter.ErrInvalidSelection) {
		return nil, fiber.NewError(422, err.Error())
	}
	if err != nil {
		return nil, mapLetterSelectionError(err)
	}
	raw, err := json.Marshal(row)
	return &apigen.SaveCVCoverLetterEvidenceOK{Data: raw}, err
}
