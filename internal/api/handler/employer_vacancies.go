package handler

import (
	"context"

	"github.com/go-faster/jx"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/employer"
)

// ListEmployerVacancies returns the caller's dashboard list: the vacancies its team published
// and the open ones of its company collected from other sources. Active-account-only.
func (h *employerHandlers) ListEmployerVacancies(ctx context.Context) (*apigen.ListEmployerVacanciesOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	jobs, extras, err := h.employer.ListVacancies(ctx, userID)
	if err != nil {
		return nil, employerError(err)
	}
	out := make([]jx.Raw, len(jobs))
	for i := range jobs {
		if out[i], err = vacancyRaw(jobs[i], extras[i]); err != nil {
			return nil, err
		}
	}
	return &apigen.ListEmployerVacanciesOK{Data: out}, nil
}

// CreateEmployerVacancy publishes a new vacancy for the caller's own claimed company, or
// idempotently updates/reopens one the caller already owns at the same URL — see
// employer.Service.CreateVacancy. A URL already owned by a different employer is a 409.
func (h *employerHandlers) CreateEmployerVacancy(ctx context.Context, req *apigen.EmployerVacancyCreate) (*apigen.CreateEmployerVacancyCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	domainJob, extras, err := h.employer.CreateVacancy(ctx, userID, vacancyInput(req))
	if err != nil {
		return nil, employerError(err)
	}
	raw, err := vacancyRaw(domainJob, extras)
	if err != nil {
		return nil, err
	}
	return &apigen.CreateEmployerVacancyCreated{Data: raw}, nil
}

// UpdateEmployerVacancy edits a vacancy the caller's own account owns, addressed by public
// slug. Any other slug (missing, another owner's, another source's) is a 404. An absent or
// null field is left unchanged. The URL and company identity are not editable; apply_mode may
// only restate the vacancy's mode; questions replace a freehire vacancy's questions.
func (h *employerHandlers) UpdateEmployerVacancy(ctx context.Context, req *apigen.EmployerVacancyPatch, params apigen.UpdateEmployerVacancyParams) (*apigen.UpdateEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := employer.VacancyPatch{
		Title:       ptrOf(req.Title.Get),
		Location:    ptrOf(req.Location.Get),
		Remote:      ptrOf(req.Remote.Get),
		Description: ptrOf(req.Description.Get),
		PostedAt:    ptrOf(req.PostedAt.Get),
	}
	if mode, ok := req.ApplyMode.Get(); ok {
		m := employer.ApplyMode(mode)
		patch.ApplyMode = &m
	}
	if in, ok := req.Questions.Get(); ok {
		qs := toEmployerQuestions(in)
		patch.Questions = &qs
	}
	domainJob, extras, err := h.employer.UpdateVacancy(ctx, userID, params.Slug, patch)
	if err != nil {
		return nil, employerError(err)
	}
	raw, err := vacancyRaw(domainJob, extras)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateEmployerVacancyOK{Data: raw}, nil
}

// CloseEmployerVacancy soft-closes a vacancy the caller's own account owns.
func (h *employerHandlers) CloseEmployerVacancy(ctx context.Context, params apigen.CloseEmployerVacancyParams) (*apigen.CloseEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.employer.CloseVacancy(ctx, userID, params.Slug); err != nil {
		return nil, employerError(err)
	}
	return &apigen.CloseEmployerVacancyOK{Data: apigen.EmployerClosed{Closed: true}}, nil
}

// DeleteEmployerVacancy soft-deletes a vacancy the caller's own team published. A collected
// vacancy of the same company, another company's, or an already deleted one is a 404.
func (h *employerHandlers) DeleteEmployerVacancy(ctx context.Context, params apigen.DeleteEmployerVacancyParams) (*apigen.DeleteEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.employer.DeleteVacancy(ctx, userID, params.Slug); err != nil {
		return nil, employerError(err)
	}
	return &apigen.DeleteEmployerVacancyOK{Data: apigen.EmployerRemoved{Removed: true}}, nil
}

// ReopenEmployerVacancy undoes the caller's own close of their vacancy — the only way back
// for a vacancy taking applications on freehire, which has no URL to re-create under.
func (h *employerHandlers) ReopenEmployerVacancy(ctx context.Context, params apigen.ReopenEmployerVacancyParams) (*apigen.ReopenEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.employer.ReopenVacancy(ctx, userID, params.Slug); err != nil {
		return nil, employerError(err)
	}
	return &apigen.ReopenEmployerVacancyOK{Data: apigen.EmployerClosed{Closed: false}}, nil
}
