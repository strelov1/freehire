//go:build integration

// Integration test for the free capture path: a Recruitee posting arrives with its
// application form already in hand, and the write path must persist both in one go.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/testdb"
)

func recruiteePosting(externalID, title string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "recruitee",
			ExternalID:  externalID,
			Title:       title,
			Company:     "Acme",
			Location:    "Warsaw, Poland",
			Description: "<p>We are looking for a backend engineer to build services.</p>",
		},
		URL: "https://acme.recruitee.com/o/" + externalID,
	})
	if err != nil {
		panic(err)
	}
	return j
}

func storedForm(t *testing.T, pool *pgxpool.Pool, jobID int64) (string, applyform.Form) {
	t.Helper()
	var provider string
	var payload []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT provider, payload FROM apply_forms WHERE job_id = $1`, jobID).Scan(&provider, &payload); err != nil {
		t.Fatalf("select apply_forms for job %d: %v", jobID, err)
	}
	var form applyform.Form
	if err := json.Unmarshal(payload, &form); err != nil {
		t.Fatalf("decode stored payload: %v", err)
	}
	return provider, form
}

// jobID loads the persisted row's id. The package's integration tests share one database
// and keep apart by using distinct external ids rather than truncating, so lookups go
// through the identity the write path used.
func jobID(t *testing.T, pool *pgxpool.Pool, externalID string) int64 {
	t.Helper()
	return jobIDFor(t, pool, "recruitee", externalID)
}

func jobIDFor(t *testing.T, pool *pgxpool.Pool, source, externalID string) int64 {
	t.Helper()
	row, err := db.New(pool).GetJobBySourceExternalID(context.Background(), db.GetJobBySourceExternalIDParams{
		Source: source, ExternalID: externalID,
	})
	if err != nil {
		t.Fatalf("load job %q: %v", externalID, err)
	}
	return row.ID
}

func queuedCaptures(t *testing.T, pool *pgxpool.Pool, jobID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM apply_form_outbox WHERE job_id = $1`, jobID).Scan(&n); err != nil {
		t.Fatalf("count apply_form_outbox for job %d: %v", jobID, err)
	}
	return n
}

func TestSaveWithApplyForm_PersistsTheFormWithTheJob(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)
	form := applyform.Form{
		Provider: "recruitee",
		Fields: []applyform.Field{
			{ID: "name", Label: "Full name", Type: applyform.TypeText, RawType: "name", Required: true},
			{ID: "7", Label: "Contract type?", Type: applyform.TypeSelect, RawType: "single_choice",
				Required: true, Options: []applyform.Option{{Label: "B2B", Value: "91"}}},
		},
	}

	if err := store.SaveWithApplyForm(ctx, recruiteePosting("acme:1", "Backend Engineer"), form); err != nil {
		t.Fatalf("SaveWithApplyForm: %v", err)
	}

	id := jobID(t, pool, "acme:1")
	provider, got := storedForm(t, pool, id)
	if provider != "recruitee" {
		t.Errorf("provider = %q, want %q", provider, "recruitee")
	}
	if len(got.Fields) != 2 {
		t.Fatalf("stored %d fields, want 2: %+v", len(got.Fields), got.Fields)
	}
	// The option's platform value is the whole point of storing the form at all.
	if len(got.Fields[1].Options) != 1 || got.Fields[1].Options[0].Value != "91" {
		t.Errorf("options = %+v, want the platform's own value preserved through the round trip",
			got.Fields[1].Options)
	}

	// A provider whose form arrives free must never also be queued for a fetch.
	if n := queuedCaptures(t, pool, id); n != 0 {
		t.Errorf("queued %d captures, want none for a form we already hold", n)
	}
}

// Re-crawling a board rewrites the form rather than accumulating copies, and picks up an
// employer's edit without anything having to notice it changed.
func TestSaveWithApplyForm_ReCrawlReplacesTheForm(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)
	posting := recruiteePosting("acme:2", "Platform Engineer")

	for _, label := range []string{"Old question", "New question"} {
		form := applyform.Form{
			Provider: "recruitee",
			Fields:   []applyform.Field{{ID: "9", Label: label, RawType: "string"}},
		}
		if err := store.SaveWithApplyForm(ctx, posting, form); err != nil {
			t.Fatalf("SaveWithApplyForm(%s): %v", label, err)
		}
	}

	id := jobID(t, pool, "acme:2")
	_, got := storedForm(t, pool, id)
	if len(got.Fields) != 1 || got.Fields[0].Label != "New question" {
		t.Errorf("stored = %+v, want only the later capture", got.Fields)
	}
}

// The ordinary Save is unchanged for every provider that yields no form.
func TestSave_WithoutAFormStoresNone(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)
	if err := store.Save(ctx, recruiteePosting("acme:3", "Data Engineer")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	id := jobID(t, pool, "acme:3")
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM apply_forms WHERE job_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count apply_forms: %v", err)
	}
	if n != 0 {
		t.Errorf("stored %d forms, want none", n)
	}
}

func greenhousePosting(externalID, title string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "greenhouse",
			ExternalID:  externalID,
			Title:       title,
			Company:     "Stripe",
			Location:    "Remote",
			Description: "<p>We are looking for a backend engineer to build payment services.</p>",
		},
		URL: "https://job-boards.greenhouse.io/stripe/jobs/" + externalID,
	})
	if err != nil {
		panic(err)
	}
	return j
}

func capturesFor(t *testing.T, pool *pgxpool.Pool, source, externalID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM apply_form_outbox o
		 JOIN jobs j ON j.id = o.job_id
		 WHERE j.source = $1 AND j.external_id = $2`, source, externalID).Scan(&n); err != nil {
		t.Fatalf("count captures for %s/%s: %v", source, externalID, err)
	}
	return n
}

// A provider whose form costs its own request is queued rather than fetched mid-crawl.
func TestSave_QueuesACaptureForAProviderThatNeedsARequest(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)

	if err := store.Save(ctx, greenhousePosting("stripe:900001", "Backend Engineer")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := capturesFor(t, pool, "greenhouse", "stripe:900001"); n != 1 {
		t.Errorf("queued %d captures, want 1", n)
	}
}

// Re-crawling the same board every few hours must not re-queue what is already captured —
// that is the difference between fetching a posting once and fetching it once per run.
func TestSave_DoesNotRequeueAnAlreadyCapturedPosting(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)
	posting := greenhousePosting("stripe:900002", "Platform Engineer")

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	// The worker drains the queue and stores the form.
	id := jobIDFor(t, pool, "greenhouse", "stripe:900002")
	if _, err := pool.Exec(ctx, `DELETE FROM apply_form_outbox WHERE job_id = $1`, id); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO apply_forms (job_id, provider, payload) VALUES ($1, 'greenhouse', '{}'::jsonb)`, id); err != nil {
		t.Fatalf("store form: %v", err)
	}

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	if n := capturesFor(t, pool, "greenhouse", "stripe:900002"); n != 0 {
		t.Errorf("queued %d captures on re-crawl, want 0 — the form is already held", n)
	}
}

// A provider nothing can capture must never accumulate entries no worker can drain.
func TestSave_NeverQueuesAProviderWithoutAFetcher(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)

	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "workday",
			ExternalID:  "tenant:R00001",
			Title:       "Backend Engineer",
			Company:     "Toyota",
			Description: "<p>We are looking for a backend engineer to build services.</p>",
		},
		URL: "https://toyotaau.wd3.myworkdayjobs.com/Careers/job/R00001",
	})
	if err != nil {
		t.Fatalf("build posting: %v", err)
	}
	if err := store.Save(ctx, j); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := capturesFor(t, pool, "workday", "tenant:R00001"); n != 0 {
		t.Errorf("queued %d captures for a provider with no fetcher, want 0", n)
	}
}
