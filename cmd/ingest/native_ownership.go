package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

type ownershipReport struct {
	Native, Transferred, Retired, Pending int
	LastID                                int64
	More                                  bool
}

func reconcileLinkedInOwnership(ctx context.Context, q *db.Queries, source sources.Source, outbound pipeline.OutboundResolver, afterID int64) (ownershipReport, error) {
	var report ownershipReport
	reader, ok := source.(interface {
		Posting(context.Context, string) (sources.Job, error)
	})
	if !ok {
		return report, fmt.Errorf("LinkedIn source cannot recheck stored posting identities")
	}
	rows, err := q.ListLinkedInOwnershipCandidates(ctx, db.ListLinkedInOwnershipCandidatesParams{AfterID: afterID, RowLimit: 300})
	if err != nil {
		return report, err
	}
	for _, row := range rows {
		report.LastID = row.ID
		posting, err := reader.Posting(ctx, row.URL)
		if err != nil {
			report.Pending++
			log.Printf("ingest: ownership recheck unavailable for %s; preserving existing state: %v", row.PublicSlug, err)
			continue
		}
		if posting.ApplyMethod == sources.ApplyOnSite {
			report.Native++
			continue
		}
		board := strings.TrimSuffix(row.ExternalID, ":"+posting.ExternalID)
		if board == row.ExternalID {
			report.Pending++
			continue
		}
		if err := outbound.Resolve(ctx, sources.CompanyEntry{Provider: "linkedin", Board: board}, posting); err != nil {
			report.Pending++
			log.Printf("ingest: native ownership pending for %s: %v", row.PublicSlug, err)
			continue
		}
		current, err := q.GetJob(ctx, row.ID)
		if err != nil {
			return report, err
		}
		switch {
		case current.Source != "linkedin":
			report.Transferred++
		case current.ClosedAt.Valid && current.ClosedReason == "ats_covered":
			report.Retired++
		default:
			report.Pending++
		}
	}
	report.More = len(rows) == 300
	if report.Pending > 0 {
		return report, fmt.Errorf("%d discoveries still need native ATS ownership; no fallback to LinkedIn-native classification", report.Pending)
	}
	return report, nil
}
