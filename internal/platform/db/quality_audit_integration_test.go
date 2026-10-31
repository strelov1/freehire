//go:build integration

// Integration tests for the sampled quality audit's queries (quality_audit.sql). Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestQualityAuditSamplesOnlyTheShownCatalogue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// open/remote/global, open/remote, open/onsite, closed/remote, private/remote, and another
	// source's open/remote.
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, work_mode, regions, closed_at, is_private)
		VALUES ('src', 'a', 'http://example.test/a', 'A', 'job-a', 'remote', '{global}', NULL, false),
		       ('src', 'b', 'http://example.test/b', 'B', 'job-b', 'remote', '{eu}', NULL, false),
		       ('src', 'c', 'http://example.test/c', 'C', 'job-c', 'onsite', '{}', NULL, false),
		       ('src', 'd', 'http://example.test/d', 'D', 'job-d', 'remote', '{}', now(), false),
		       ('src', 'e', 'http://example.test/e', 'E', 'job-e', 'remote', '{}', NULL, true),
		       ('other', 'f', 'http://example.test/f', 'F', 'job-f', 'remote', '{}', NULL, false)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	live, err := q.SampleLivenessAuditJobs(ctx, SampleLivenessAuditJobsParams{Source: "src", Fraction: 1, MaxRows: 10})
	if err != nil {
		t.Fatalf("liveness sample: %v", err)
	}
	if got := slugsOf(live, func(r SampleLivenessAuditJobsRow) string { return r.PublicSlug }); got != "job-a,job-b,job-c" {
		t.Errorf("liveness sample = %s, want the open public postings of src", got)
	}

	remote, err := q.SampleRemoteAuditJobs(ctx, SampleRemoteAuditJobsParams{Source: "src", Fraction: 1, MaxRows: 10})
	if err != nil {
		t.Fatalf("remote sample: %v", err)
	}
	if got := slugsOf(remote, func(r SampleRemoteAuditJobsRow) string { return r.PublicSlug }); got != "job-a,job-b" {
		t.Errorf("remote sample = %s, want the open public remote postings of src", got)
	}
	for _, r := range remote {
		if r.TaggedGlobal != (r.PublicSlug == "job-a") {
			t.Errorf("%s tagged_global = %v", r.PublicSlug, r.TaggedGlobal)
		}
	}

	counts, err := q.CountOpenRemoteJobsBySource(ctx, 2)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(counts) != 1 || counts[0] != (CountOpenRemoteJobsBySourceRow{Source: "src", RemoteJobs: 2}) {
		t.Errorf("counts = %+v, want src:2 only (other is under the floor)", counts)
	}
}

func TestQualityAuditTallyWindow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE remote_audit_judgements"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if latest, err := q.LatestRemoteAuditJudgement(ctx); err != nil || latest.Valid {
		t.Fatalf("latest on an empty table = %+v, %v; want NULL", latest, err)
	}
	for _, j := range []InsertRemoteAuditJudgementParams{
		{JobID: 1, Source: "s", WorkMode: "remote", Worldwide: "yes", TaggedGlobal: true},
		{JobID: 2, Source: "s", WorkMode: "remote", Worldwide: "no", TaggedGlobal: true},
		{JobID: 3, Source: "s", WorkMode: "hybrid", Worldwide: "no", TaggedGlobal: false},
		{JobID: 4, Source: "s", WorkMode: "onsite", Worldwide: "unclear", TaggedGlobal: true},
		{JobID: 5, Source: "s", WorkMode: "unclear", Worldwide: "unclear", TaggedGlobal: false},
	} {
		if err := q.InsertRemoteAuditJudgement(ctx, j); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	// Outside the window: must not count.
	if _, err := pool.Exec(ctx, `INSERT INTO remote_audit_judgements (job_id, source, work_mode, worldwide, tagged_global, judged_at)
		VALUES (6, 's', 'onsite', 'no', true, now() - interval '60 days')`); err != nil {
		t.Fatalf("old: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO remote_audit_judgements (job_id, source, work_mode, worldwide, tagged_global)
		VALUES (7, 's', 'maybe', 'yes', false)`); err == nil {
		t.Error("a work mode outside the vocabulary was accepted")
	}

	rows, err := q.TallyRemoteAuditJudgements(ctx, pgtype.Timestamptz{Time: time.Now().Add(-56 * 24 * time.Hour), Valid: true})
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	want := TallyRemoteAuditJudgementsRow{
		Source: "s", Sampled: 5, Confirmed: 2, Contradicted: 2,
		GlobalSampled: 3, GlobalConfirmed: 1, GlobalContradicted: 1,
	}
	if len(rows) != 1 || rows[0] != want {
		t.Errorf("tally = %+v, want %+v", rows, want)
	}
	if latest, err := q.LatestRemoteAuditJudgement(ctx); err != nil || !latest.Valid {
		t.Errorf("latest = %+v, %v; want a time", latest, err)
	}
}

func slugsOf[T any](rows []T, slug func(T) string) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, slug(r))
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}
