package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobfacts"
	"github.com/strelov1/freehire/internal/platform/db"
)

// metricsQueries is the slice of *db.Queries this worker needs, so collect is testable against a fake.
type metricsQueries interface {
	SearchOutboxMetrics(context.Context) (db.SearchOutboxMetricsRow, error)
	SearchDeleteOutboxMetrics(context.Context) (db.SearchDeleteOutboxMetricsRow, error)
	EnrichmentOutboxMetrics(context.Context) (db.EnrichmentOutboxMetricsRow, error)
	SemanticOutboxMetrics(context.Context) (db.SemanticOutboxMetricsRow, error)
	MailClassificationOutboxMetrics(context.Context) (db.MailClassificationOutboxMetricsRow, error)
	ApplyFormOutboxMetrics(context.Context) (db.ApplyFormOutboxMetricsRow, error)
	AdzunaDescriptionOutboxMetrics(context.Context) (db.AdzunaDescriptionOutboxMetricsRow, error)
	AutoApplyQueueMetrics(context.Context) (db.AutoApplyQueueMetricsRow, error)
	PushTicketOutboxMetrics(context.Context) (db.PushTicketOutboxMetricsRow, error)
	AppleRevocationJobMetrics(context.Context) (db.AppleRevocationJobMetricsRow, error)
	NewestOpenJobCreatedAt(context.Context) (pgtype.Timestamptz, error)
	ProviderIngestHealth(context.Context) ([]db.ProviderIngestHealthRow, error)
	NotifyBacklogMetrics(context.Context) ([]db.NotifyBacklogMetricsRow, error)
	NotifyDigestVolumeMetrics(context.Context) (db.NotifyDigestVolumeMetricsRow, error)
	IngestDuplicationMetrics(context.Context) ([]db.IngestDuplicationMetricsRow, error)
	SourceQualityMetrics(context.Context) ([]db.SourceQualityMetricsRow, error)
	EmploymentBoilerplateCandidates(context.Context) ([]db.EmploymentBoilerplateCandidatesRow, error)
}

const employmentBoilerplateCap = 500

var employmentBoilerplateKinds = []string{
	"part_time_pay_policy",
	"full_time_pay_policy",
	"contract_boilerplate",
	"permanent_status",
}

// collect runs one measurement pass. Any query failure aborts it: a partial exposition would
// silently drop families, which downstream reads as a dead exporter.
func collect(ctx context.Context, q metricsQueries) (snapshot, error) {
	search, err := q.SearchOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("search outbox metrics: %w", err)
	}
	searchDelete, err := q.SearchDeleteOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("search delete outbox metrics: %w", err)
	}
	enrichment, err := q.EnrichmentOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("enrichment outbox metrics: %w", err)
	}
	semantic, err := q.SemanticOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("semantic outbox metrics: %w", err)
	}
	mail, err := q.MailClassificationOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("mail classification outbox metrics: %w", err)
	}
	applyForms, err := q.ApplyFormOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("apply form outbox metrics: %w", err)
	}
	adzunaDescriptions, err := q.AdzunaDescriptionOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("adzuna description outbox metrics: %w", err)
	}
	autoApply, err := q.AutoApplyQueueMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("auto apply queue metrics: %w", err)
	}
	pushTickets, err := q.PushTicketOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("push ticket outbox metrics: %w", err)
	}
	appleRevocations, err := q.AppleRevocationJobMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("apple revocation job metrics: %w", err)
	}
	newestJob, err := newestJobTime(ctx, q)
	if err != nil {
		return snapshot{}, err
	}

	notifyRows, err := q.NotifyBacklogMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("notify backlog metrics: %w", err)
	}
	// A state with nothing pending has no row and stays the zero value, which render
	// still publishes.
	var notifyDeliverable, notifyUndeliverable notifyBacklog
	for _, r := range notifyRows {
		b := notifyBacklog{pendingSubscriptions: r.PendingSubscriptions, oldestAgeSeconds: r.OldestAgeSeconds}
		if r.Deliverable {
			notifyDeliverable = b
		} else {
			notifyUndeliverable = b
		}
	}

	notifyVolume, err := q.NotifyDigestVolumeMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("notify digest volume metrics: %w", err)
	}

	health, err := q.ProviderIngestHealth(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("provider ingest health: %w", err)
	}
	providers := make([]providerHealth, len(health))
	for i, r := range health {
		// An invalid Timestamptz means no live board has ever succeeded. It stays the zero time
		// and render drops it (a Unix zero would read as overdue since 1970); firstListed and
		// the board counts still carry the provider.
		p := providerHealth{
			name:      r.Provider,
			uncrawled: r.Uncrawled,
			cooled:    r.Cooled,
			failing:   r.Failing,
			healthy:   r.Healthy,
		}
		if r.LastSuccessAt.Valid {
			p.lastSuccess = r.LastSuccessAt.Time
		}
		if r.FirstListedAt.Valid {
			p.firstListed = r.FirstListedAt.Time
		}
		providers[i] = p
	}

	// Carried through untouched: which multiple is a problem depends on the source, so the
	// alert rule decides (see render.go).
	volume, err := q.IngestDuplicationMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("ingest duplication metrics: %w", err)
	}
	ingest := make([]ingestVolume, len(volume))
	for i, r := range volume {
		ingest[i] = ingestVolume{
			provider:         r.Provider,
			rowsWritten:      r.RowsWritten,
			distinctPostings: r.DistinctPostings,
		}
	}

	// Read from the snapshot cmd/rollup-stats rebuilds; measuring here would be a full catalogue scan.
	quality, err := q.SourceQualityMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("source quality metrics: %w", err)
	}
	sources := make([]sourceQuality, len(quality))
	for i, r := range quality {
		sources[i] = sourceQuality{
			source:   r.Source,
			openJobs: r.OpenJobs,
			// In qualityChecks order.
			gaps:       [len(qualityChecks)]int64{r.MissingWorkModeJobs, r.MissingGeoJobs, r.MissingCompanyJobs, r.Unseen7dJobs, r.PostedOver60dJobs},
			measuredAt: r.MeasuredAt.Time,
		}
	}
	candidates, err := q.EmploymentBoilerplateCandidates(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("employment boilerplate metrics: %w", err)
	}

	return snapshot{
		// Every pipeline queue must be listed: their workers exit 0 whether or not they kept up,
		// so an unmeasured queue has no signal. Adding an outbox means adding it here.
		queues: []queueMetrics{
			{name: "search_outbox", depth: search.Depth, deadLetters: &search.DeadLetters, oldestAgeSeconds: search.OldestAgeSeconds},
			{name: "search_delete_outbox", depth: searchDelete.Depth, deadLetters: &searchDelete.DeadLetters, oldestAgeSeconds: searchDelete.OldestAgeSeconds},
			{name: "enrichment_outbox", depth: enrichment.Depth, deadLetters: &enrichment.DeadLetters, oldestAgeSeconds: enrichment.OldestAgeSeconds},
			{name: "semantic_outbox", depth: semantic.Depth, deadLetters: &semantic.DeadLetters, oldestAgeSeconds: semantic.OldestAgeSeconds},
			{name: "email_classification_outbox", depth: mail.Depth, deadLetters: &mail.DeadLetters, oldestAgeSeconds: mail.OldestAgeSeconds},
			{name: "apply_form_outbox", depth: applyForms.Depth, deadLetters: &applyForms.DeadLetters, oldestAgeSeconds: applyForms.OldestAgeSeconds},
			{name: "adzuna_description_outbox", depth: adzunaDescriptions.Depth, deadLetters: &adzunaDescriptions.DeadLetters, oldestAgeSeconds: adzunaDescriptions.OldestAgeSeconds},
			// The only queue with a third state: a parked attempt needs new data, not a retry,
			// so it is neither depth nor a dead letter (see AutoApplyQueueMetrics).
			{name: "auto_apply_queue", depth: autoApply.Depth, deadLetters: &autoApply.DeadLetters, blocked: &autoApply.Blocked, oldestAgeSeconds: autoApply.OldestAgeSeconds},
			// No dead letters: the table has neither attempts nor failed_at. Nil, not zero.
			{name: "push_ticket_outbox", depth: pushTickets.Depth, oldestAgeSeconds: pushTickets.OldestAgeSeconds},
			// Not an outbox by name but the same by shape: cmd/apple-revoke exits 0 either way, so
			// a backlog stranded in revocation_pending is invisible without this.
			{name: "apple_revocation_jobs", depth: appleRevocations.Depth, deadLetters: &appleRevocations.DeadLetters, oldestAgeSeconds: appleRevocations.OldestAgeSeconds},
		},
		notifyDeliverable:   notifyDeliverable,
		notifyUndeliverable: notifyUndeliverable,
		notifyVolume:        notifyVolume,
		newestJob:           newestJob,
		providers:           providers,
		ingest:              ingest,
		sources:             sources,
		boilerplate:         countEmploymentBoilerplate(candidates),
	}, nil
}

func countEmploymentBoilerplate(rows []db.EmploymentBoilerplateCandidatesRow) employmentBoilerplate {
	truncated := len(rows) > employmentBoilerplateCap
	if truncated {
		rows = rows[:employmentBoilerplateCap]
	}
	counts := map[string]int64{}
	for _, row := range rows {
		kind := jobfacts.EmploymentBoilerplateKind(row.EmploymentType, row.Title, row.Description)
		if kind == "" {
			continue
		}
		counts[kind]++
	}
	return employmentBoilerplate{counts: counts, sampleSize: int64(len(rows)), truncated: truncated}
}

// newestJobTime reports when the catalogue last gained an open posting, or the zero time when
// it holds none (no row is a fresh install, not a failure).
func newestJobTime(ctx context.Context, q metricsQueries) (time.Time, error) {
	newest, err := q.NewestOpenJobCreatedAt(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("newest open job: %w", err)
	}
	if !newest.Valid {
		return time.Time{}, nil
	}
	return newest.Time, nil
}
