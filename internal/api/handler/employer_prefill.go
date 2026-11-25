package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// PrefillEmployerVacancy reads a vacancy link into draft field values for the publish form
// and writes nothing. Unlike the public submit prefill, a vacancy the catalogue already
// carries is not refused: its stored fields come back, since an employer re-publishing or
// moving their own posting is the common case. Active accounts only.
func (h *employerHandlers) PrefillEmployerVacancy(ctx context.Context, req *apigen.PrefillRequest) (*apigen.PrefillEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.employer.ActiveAccount(ctx, userID); err != nil {
		return nil, employerError(err)
	}
	out, err := h.prefiller.prefill(ctx, req.URL.Value, true)
	if err != nil {
		return nil, err
	}
	return &apigen.PrefillEmployerVacancyOK{Data: out}, nil
}
