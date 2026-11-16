//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPromoteJobSourcePreservesIdentityAndNativeRecrawl(t *testing.T) {
	pool := startPostgres(t)
	truncate(t, pool)
	ctx, q := context.Background(), New(pool)
	p := ingestParams("software engineer:4472812273", "Senior Java Software Engineer")
	originalSlug := p.PublicSlug
	p.Source, p.URL = "linkedin", "https://www.linkedin.com/jobs/view/4472812273/"
	old, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	user := seedResponseUser(t, q, "native-source@example.test", true)
	if _, err := pool.Exec(ctx, `INSERT INTO applications (user_id, company_slug, role_title, job_id) VALUES ($1, 'acme', 'Engineer', $2)`, user, old.ID); err != nil {
		t.Fatal(err)
	}
	move := PromoteJobSourceParams{JobID: old.ID, ExpectedSource: p.Source, ExpectedExternalID: p.ExternalID, ExpectedUrl: p.URL,
		Source: "hrsmart", ExternalID: "gistworld.mua.hrdepartment.com:3735", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"}
	move.ExpectedApplicationUrl = move.URL + "?src=LinkedIn"
	if _, err := q.SetJobApplicationURL(ctx, SetJobApplicationURLParams{JobID: old.ID, ApplicationUrl: move.ExpectedApplicationUrl}); err != nil {
		t.Fatal(err)
	}
	if id, err := q.PromoteJobSource(ctx, move); err != nil || id != old.ID {
		t.Fatalf("transfer = %d, %v", id, err)
	}
	row, err := q.GetJobBySlug(ctx, p.PublicSlug)
	if err != nil || row.ID != old.ID || row.Source != move.Source || row.ExternalID != move.ExternalID || row.URL != move.URL || row.ApplicationUrl != "" {
		t.Fatalf("transferred posting = %+v, %v", row, err)
	}
	p.Source, p.ExternalID, p.URL = move.Source, move.ExternalID, move.URL
	p.PublicSlug = "a-native-crawl-would-derive-a-different-slug"
	p.Description = "The native ATS supplied this updated description."
	recrawled, err := q.UpsertJob(ctx, p)
	if err != nil || recrawled.ID != old.ID {
		t.Fatalf("native recrawl duplicated posting: %+v, %v", recrawled, err)
	}
	row, err = q.GetJob(ctx, old.ID)
	if err != nil || row.PublicSlug != originalSlug || row.Description != p.Description {
		t.Fatalf("native recrawl lost old slug or native content: %+v, %v", row, err)
	}
	var linked int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM applications WHERE user_id=$1 AND job_id=$2", user, old.ID).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("application history lost: %d, %v", linked, err)
	}
	if _, err := q.PromoteJobSource(ctx, move); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale transfer accepted: %v", err)
	}
}

func TestPromoteJobSourceCannotOverwriteAnExistingNativeIdentity(t *testing.T) {
	pool := startPostgres(t)
	truncate(t, pool)
	ctx, q := context.Background(), New(pool)
	p := ingestParams("linkedin:old", "Java Engineer")
	p.Source = "linkedin"
	old, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	native := p
	native.Source, native.ExternalID, native.PublicSlug = "hrsmart", "gistworld.mua.hrdepartment.com:3735", "existing-native-posting"
	if _, err := q.UpsertJob(ctx, native); err != nil {
		t.Fatal(err)
	}
	if _, err := q.PromoteJobSource(ctx, PromoteJobSourceParams{JobID: old.ID, ExpectedSource: p.Source, ExpectedExternalID: p.ExternalID, ExpectedUrl: p.URL, Source: native.Source, ExternalID: native.ExternalID, URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"}); err == nil {
		t.Fatal("native identity collision must fail without merging unrelated history")
	}
	row, err := q.GetJob(ctx, old.ID)
	if err != nil || row.Source != p.Source || row.ExternalID != p.ExternalID || row.PublicSlug != p.PublicSlug {
		t.Fatalf("failed transfer changed old posting: %+v, %v", row, err)
	}
}
