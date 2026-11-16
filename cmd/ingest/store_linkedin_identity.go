package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// linkedInStoredIdentities lets listing-only liveness evidence reach rows stored
// under a legacy keyword key. Unknown and already-namespaced keys stay unchanged;
// the existing refresh query still excludes closed and private rows.
func linkedInStoredIdentities(ctx context.Context, q *db.Queries, ids []string) ([]string, error) {
	mapped := make([]string, 0, len(ids))
	for _, id := range ids {
		raw := "https://www.linkedin.com/jobs/view/" + id + "/"
		if id == "" || sources.LinkedInPostingID(raw) != id {
			mapped = append(mapped, id)
			continue
		}
		row, err := q.FindLinkedInJobByURL(ctx, raw)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			id = row.ExternalID
		}
		mapped = append(mapped, id)
	}
	return mapped, nil
}
