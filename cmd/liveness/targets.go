package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

type probeScope struct {
	source string
	jobIDs []int64
}

type candidateQueries interface {
	GetJobsByIDs(context.Context, []int64) ([]db.Job, error)
	SelectOrphanLivenessCandidates(context.Context, []string) ([]db.SelectOrphanLivenessCandidatesRow, error)
	SelectStaleRegisteredCandidates(context.Context, db.SelectStaleRegisteredCandidatesParams) ([]db.SelectStaleRegisteredCandidatesRow, error)
}

func parseProbeJobIDs(raw string) ([]int64, error) {
	if raw == "" {
		return nil, nil
	}
	var ids []int64
	for _, value := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("job-ids must contain positive integer IDs")
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
		if len(ids) > 500 {
			return nil, fmt.Errorf("job-ids supports at most 500 distinct IDs")
		}
	}
	return ids, nil
}

func selectProbeCandidates(ctx context.Context, q candidateQueries, scope probeScope, excluded, probed []string, cutoff time.Time) ([]db.SelectOrphanLivenessCandidatesRow, []db.SelectStaleRegisteredCandidatesRow, error) {
	if len(scope.jobIDs) == 0 {
		orphans, err := q.SelectOrphanLivenessCandidates(ctx, excluded)
		if err != nil {
			return nil, nil, err
		}
		if scope.source != "" {
			orphans = slices.DeleteFunc(orphans, func(c db.SelectOrphanLivenessCandidatesRow) bool { return c.Source != scope.source })
		}
		stale, err := q.SelectStaleRegisteredCandidates(ctx, db.SelectStaleRegisteredCandidatesParams{
			Sources: filterSources(probed, scope.source), Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
		})
		return orphans, stale, err
	}
	jobs, err := q.GetJobsByIDs(ctx, scope.jobIDs)
	if err != nil {
		return nil, nil, err
	}
	var orphans []db.SelectOrphanLivenessCandidatesRow
	var stale []db.SelectStaleRegisteredCandidatesRow
	for _, j := range jobs {
		if !slices.Contains(scope.jobIDs, j.ID) || j.ClosedAt.Valid || j.IsPrivate || j.URL == "" || (scope.source != "" && j.Source != scope.source) {
			continue
		}
		if slices.Contains(probed, j.Source) {
			if j.LastSeenAt.Valid && j.LastSeenAt.Time.Before(cutoff) {
				stale = append(stale, db.SelectStaleRegisteredCandidatesRow{ID: j.ID, Source: j.Source, URL: j.URL, ExternalID: j.ExternalID, PublicSlug: j.PublicSlug, LivenessStrikes: j.LivenessStrikes})
			}
		} else if !slices.Contains(excluded, j.Source) {
			orphans = append(orphans, db.SelectOrphanLivenessCandidatesRow{ID: j.ID, Source: j.Source, URL: j.URL, PublicSlug: j.PublicSlug, LivenessStrikes: j.LivenessStrikes})
		}
	}
	return orphans, stale, nil
}
