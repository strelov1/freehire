package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// metricsQueries is the slice of *db.Queries this worker needs, declared here so the
// assembly and its error handling are testable against a fake rather than a container.
// Narrow by intent, matching internal/platform/worker's FullScanQueries.
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
	BoardHealthMetrics(context.Context) (db.BoardHealthMetricsRow, error)
	NewestOpenJobCreatedAt(context.Context) (pgtype.Timestamptz, error)
	ProviderIngestHealth(context.Context) ([]db.ProviderIngestHealthRow, error)
	NotifyBacklogMetrics(context.Context) (db.NotifyBacklogMetricsRow, error)
}

// collect runs one measurement pass. Any query failure aborts the pass: a partial
// exposition would publish some families and silently drop others, and a dropped family
// reads downstream as a dead exporter — a louder and more misleading signal than the one
// failed run this returns.
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
	boards, err := q.BoardHealthMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("board health metrics: %w", err)
	}

	newestJob, err := newestJobTime(ctx, q)
	if err != nil {
		return snapshot{}, err
	}

	notifyBacklog, err := q.NotifyBacklogMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("notify backlog metrics: %w", err)
	}

	health, err := q.ProviderIngestHealth(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("provider ingest health: %w", err)
	}
	providers := make([]providerHealth, len(health))
	for i, r := range health {
		// An invalid Timestamptz is max() over a provider whose every board has never
		// succeeded. It stays the zero time here and is dropped by render, because the
		// alternative — a Unix zero — reads downstream as a provider overdue since 1970.
		// The board counts below carry that provider instead: they are always present.
		p := providerHealth{
			name:    r.Provider,
			cooled:  r.Cooled,
			failing: r.Failing,
			healthy: r.Healthy,
		}
		if r.LastSuccessAt.Valid {
			p.lastSuccess = r.LastSuccessAt.Time
		}
		providers[i] = p
	}

	return snapshot{
		// Every queue in the pipeline, because a queue this worker does not measure has
		// no signal at all: each of these is drained by a worker that exits 0 whether it
		// kept up or not, so the per-run family stays green through any backlog. Adding
		// an outbox means adding it here.
		queues: []queueMetrics{
			{name: "search_outbox", depth: search.Depth, deadLetters: &search.DeadLetters, oldestAgeSeconds: search.OldestAgeSeconds},
			{name: "search_delete_outbox", depth: searchDelete.Depth, deadLetters: &searchDelete.DeadLetters, oldestAgeSeconds: searchDelete.OldestAgeSeconds},
			{name: "enrichment_outbox", depth: enrichment.Depth, deadLetters: &enrichment.DeadLetters, oldestAgeSeconds: enrichment.OldestAgeSeconds},
			{name: "semantic_outbox", depth: semantic.Depth, deadLetters: &semantic.DeadLetters, oldestAgeSeconds: semantic.OldestAgeSeconds},
			{name: "email_classification_outbox", depth: mail.Depth, deadLetters: &mail.DeadLetters, oldestAgeSeconds: mail.OldestAgeSeconds},
			{name: "apply_form_outbox", depth: applyForms.Depth, deadLetters: &applyForms.DeadLetters, oldestAgeSeconds: applyForms.OldestAgeSeconds},
			{name: "adzuna_description_outbox", depth: adzunaDescriptions.Depth, deadLetters: &adzunaDescriptions.DeadLetters, oldestAgeSeconds: adzunaDescriptions.OldestAgeSeconds},
			// The only queue with a third state: a parked attempt needs new data, not
			// another try, so it is neither depth nor a dead letter. See
			// AutoApplyQueueMetrics for why folding it into either would misread it.
			{name: "auto_apply_queue", depth: autoApply.Depth, deadLetters: &autoApply.DeadLetters, blocked: &autoApply.Blocked, oldestAgeSeconds: autoApply.OldestAgeSeconds},
			// No dead letters at all — the table carries neither attempts nor failed_at.
			// A nil says so; a zero would claim a measurement nobody took.
			{name: "push_ticket_outbox", depth: pushTickets.Depth, oldestAgeSeconds: pushTickets.OldestAgeSeconds},
			// Not an outbox by name, but the same thing by shape and by hazard: a queue
			// one worker drains, whose given-up entries nothing ever claims again. A
			// queue this worker does not measure has no signal at all — cmd/apple-revoke
			// exits 0 either way, so the per-run family stays green through a backlog of
			// identities stranded in revocation_pending.
			{name: "apple_revocation_jobs", depth: appleRevocations.Depth, deadLetters: &appleRevocations.DeadLetters, oldestAgeSeconds: appleRevocations.OldestAgeSeconds},
		},
		notifyPendingSubscriptions: notifyBacklog.PendingSubscriptions,
		notifyOldestAgeSeconds:     notifyBacklog.OldestAgeSeconds,
		healthyBoards:              boards.Healthy,
		failingBoards:              boards.Failing,
		cooledBoards:               boards.Cooled,
		newestJob:                  newestJob,
		providers:                  providers,
	}, nil
}

// newestJobTime reports when the catalogue last gained an open posting, or the zero time
// when it holds none. An empty catalogue answers with no row, which is a fresh-install
// state rather than a failure — every other family still deserves to be published.
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
