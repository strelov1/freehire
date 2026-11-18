package main

import (
	"context"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/platform/db"
)

var _ pipeline.StoredBodyBatchLookup = (*dbStore)(nil)

func (s *dbStore) StoredDescriptions(ctx context.Context, source string, externalIDs []string) (map[string]string, error) {
	rows, err := s.q.StoredJobDescriptions(ctx, db.StoredJobDescriptionsParams{Source: source, ExternalIds: externalIDs})
	if err != nil {
		return nil, err
	}
	bodies := make(map[string]string, len(rows))
	for _, row := range rows {
		bodies[row.ExternalID] = row.Description
	}
	return bodies, nil
}
