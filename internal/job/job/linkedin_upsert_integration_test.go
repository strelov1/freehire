//go:build integration

package job_test

import (
	"context"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

const linkedinIdentityURL = "https://www.linkedin.com/jobs/view/4490679811/"

func linkedinParams(t *testing.T, id string) db.UpsertJobParams {
	t.Helper()
	p := upsertParams(t, id)
	p.Source, p.URL = "linkedin", linkedinIdentityURL
	return p
}

func TestUpsert_LinkedInReusesLegacyIdentityAndHistory(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	var legacyID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug)
		VALUES ('linkedin','software engineer:4490679811',$1,'Backend Engineer','existing-linkedin-slug') RETURNING id`, linkedinIdentityURL).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('linkedin-history@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_jobs (user_id,job_id,saved_at) VALUES ($1,$2,now())`, userID, legacyID); err != nil {
		t.Fatal(err)
	}
	for _, externalID := range []string{"4490679811", "data engineer:4490679811"} {
		if got := upsertCommitted(t, pool, linkedinParams(t, externalID)); got.ID != legacyID {
			t.Fatalf("%q wrote job %d, want existing %d", externalID, got.ID, legacyID)
		}
	}
	saved, err := db.New(pool).GetJob(ctx, legacyID)
	if err != nil || saved.PublicSlug != "existing-linkedin-slug" || saved.ExternalID != "software engineer:4490679811" {
		t.Fatalf("legacy identity changed: id=%d slug=%q key=%q error=%v", saved.ID, saved.PublicSlug, saved.ExternalID, err)
	}
	var marked bool
	if err := pool.QueryRow(ctx, `SELECT saved_at IS NOT NULL FROM user_jobs WHERE user_id=$1 AND job_id=$2`, userID, legacyID).Scan(&marked); err != nil || !marked {
		t.Fatalf("saved mark=%t error=%v", marked, err)
	}
}

func TestUpsert_LinkedInLegacyRediscoveryReopensSameRow(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, closed_at, closed_reason)
		VALUES ('linkedin','software engineer:4490679811',$1,'Backend Engineer','closed-linkedin-slug',now(),'unseen') RETURNING id`, linkedinIdentityURL).Scan(&id); err != nil {
		t.Fatal(err)
	}
	got := upsertCommitted(t, pool, linkedinParams(t, "4490679811"))
	if got.ID != id || readActivity(t, pool, id).closed {
		t.Fatalf("rediscovery id=%d, want reopened %d", got.ID, id)
	}
}

func TestUpsert_LinkedInExpiredLegacyRowKeepsOrdinaryExpiryPolicy(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, application_deadline, closed_at, closed_reason)
		VALUES ('linkedin','software engineer:4490679811',$1,'Backend Engineer','expired-linkedin-slug',current_date-3,now(),'expired') RETURNING id`, linkedinIdentityURL).Scan(&id); err != nil {
		t.Fatal(err)
	}
	got := upsertCommitted(t, pool, linkedinParams(t, "4490679811"))
	if got.ID != id || !readActivity(t, pool, id).closed {
		t.Fatalf("rediscovery id=%d, want original expired row %d to stay closed", got.ID, id)
	}
}

func TestUpsert_LinkedInConcurrentNumericIdentityCreatesOneRow(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	params := linkedinParams(t, "4490679811")
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			tx, err := pool.Begin(ctx)
			if err != nil {
				errors <- err
				return
			}
			defer tx.Rollback(ctx)
			if _, err = job.Upsert(ctx, tx, params); err == nil {
				err = tx.Commit(ctx)
			}
			errors <- err
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE source='linkedin'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("LinkedIn rows=%d error=%v, want exactly one", count, err)
	}
}

func TestUpsert_LinkedInRefusesPrivateIdentityCollision(t *testing.T) {
	pool, ctx := testdb.Pool(t), context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug,is_private)
		VALUES ('linkedin','4490679811',$1,'Private title','private-linkedin-slug',true)`, linkedinIdentityURL); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := job.Upsert(ctx, tx, linkedinParams(t, "4490679811")); err == nil {
		t.Fatal("private identity collision was rewritten")
	}
	var title string
	if err := pool.QueryRow(ctx, `SELECT title FROM jobs WHERE source='linkedin' AND external_id='4490679811'`).Scan(&title); err != nil || title != "Private title" {
		t.Fatalf("private title=%q error=%v", title, err)
	}
}

func TestUpsert_NonLinkedInURLDoesNotMergeIdentities(t *testing.T) {
	pool := testdb.Pool(t)
	first := upsertParams(t, "one")
	second := upsertParams(t, "two")
	second.URL = first.URL
	one := upsertCommitted(t, pool, first)
	two := upsertCommitted(t, pool, second)
	if one.ID == two.ID {
		t.Fatal("non-LinkedIn source identities merged by URL")
	}
}
