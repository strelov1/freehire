package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/platform/db"
)

// dbApplyFormReader adapts the generated queries to atsapply.StoredFormReader — the
// preview pass's own read of what cmd/capture-apply-form already persisted, mirroring
// internal/api/handler/apply_form.go's own JobApplyForm read (decode + attach the row's
// own provider, since a re-capture rewrites only the row, not the JSON inside it).
type dbApplyFormReader struct {
	q *db.Queries
}

func (r *dbApplyFormReader) GetStoredForm(ctx context.Context, jobID int64) (applyform.Form, bool, error) {
	row, err := r.q.GetApplyFormByJobID(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return applyform.Form{}, false, nil
	}
	if err != nil {
		return applyform.Form{}, false, err
	}
	var form applyform.Form
	if err := json.Unmarshal(row.Payload, &form); err != nil {
		return applyform.Form{}, false, fmt.Errorf("decode stored apply form for job %d: %w", jobID, err)
	}
	form.Provider = row.Provider
	return form, true, nil
}
