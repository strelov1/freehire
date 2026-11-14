//go:build integration

// Integration test for GET /api/v1/jobs/changes: the opened / closed / reopened log, its
// visibility rules and a keyset walk that survives concurrent writers.
// Run with: go test -tags=integration -run JobChanges ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

type changeItem struct {
	Slug   string `json:"public_slug"`
	Kind   string `json:"kind"`
	Reason string `json:"closed_reason"`
}

type changesPage struct {
	Data []changeItem `json:"data"`
	Meta struct {
		HasMore        bool   `json:"has_more"`
		NextCursor     string `json:"next_cursor"`
		AvailableSince string `json:"available_since"`
	} `json:"meta"`
}

func getChanges(t *testing.T, app *fiber.App, q url.Values) changesPage {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/jobs/changes?"+q.Encode(), nil))
	if err != nil {
		t.Fatalf("GET changes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET changes?%s = %d", q.Encode(), resp.StatusCode)
	}
	var p changesPage
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return p
}

func seedJob(t *testing.T, pool *pgxpool.Pool, slug string, extra string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug`+extra+`)
		 VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, 'acme'`+valuesFor(extra)+`)`, slug); err != nil {
		t.Fatalf("seed %s: %v", slug, err)
	}
}

func valuesFor(extra string) string {
	switch extra {
	case ", is_private":
		return ", true"
	}
	return ""
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestJobChangesFeed(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := &jobsHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountJobsContract(app, h, middleware{})
	since := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)

	// a: opened, closed (unseen), reopened, closed again (liveness). b: opened. c: private, so
	// never logged. d: opened, then marked a duplicate (a removal) and later canonical again.
	companyRow := func() { mustExec(t, pool, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme')`) }
	companyRow()
	seedJob(t, pool, "job-a", "")
	mustExec(t, pool, `UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE public_slug = 'job-a'`)
	mustExec(t, pool, `UPDATE jobs SET closed_at = NULL, closed_reason = '' WHERE public_slug = 'job-a'`)
	seedJob(t, pool, "job-b", "")
	seedJob(t, pool, "job-c", ", is_private")
	seedJob(t, pool, "job-d", "")
	mustExec(t, pool, `UPDATE jobs SET duplicate_of_role = (SELECT id FROM jobs WHERE public_slug = 'job-b') WHERE public_slug = 'job-d'`)
	mustExec(t, pool, `UPDATE jobs SET closed_at = now(), closed_reason = 'expired' WHERE public_slug = 'job-a'`)
	// Touching an already-closed row is not a transition.
	mustExec(t, pool, `UPDATE jobs SET closed_at = now() WHERE public_slug = 'job-a'`)

	want := []changeItem{
		{"job-a", "opened", ""}, {"job-a", "closed", "unseen"}, {"job-a", "reopened", ""},
		{"job-b", "opened", ""}, {"job-d", "opened", ""}, {"job-d", "closed", "duplicate"},
		{"job-a", "closed", "expired"},
	}
	t.Run("the whole feed arrives in order", func(t *testing.T) {
		p := getChanges(t, app, url.Values{"since": {since}})
		if len(p.Data) != len(want) {
			t.Fatalf("got %+v, want %+v", p.Data, want)
		}
		for i := range want {
			if p.Data[i] != want[i] {
				t.Fatalf("item %d = %+v, want %+v", i, p.Data[i], want[i])
			}
		}
		if p.Meta.HasMore || p.Meta.NextCursor == "" || p.Meta.AvailableSince == "" {
			t.Fatalf("meta = %+v", p.Meta)
		}
	})

	t.Run("pages neither skip nor repeat", func(t *testing.T) {
		var got []changeItem
		q := url.Values{"since": {since}, "limit": {"2"}}
		for range 10 {
			p := getChanges(t, app, q)
			got = append(got, p.Data...)
			q = url.Values{"cursor": {p.Meta.NextCursor}, "limit": {"2"}}
			if !p.Meta.HasMore {
				break
			}
		}
		if len(got) != len(want) {
			t.Fatalf("walk = %+v, want %+v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("walk item %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("a duplicate leaving and returning is a removal then an opening", func(t *testing.T) {
		cursor := getChanges(t, app, url.Values{"since": {since}}).Meta.NextCursor
		mustExec(t, pool, `UPDATE jobs SET duplicate_of_role = NULL WHERE public_slug = 'job-d'`)
		p := getChanges(t, app, url.Values{"cursor": {cursor}})
		if len(p.Data) != 1 || p.Data[0] != (changeItem{"job-d", "reopened", ""}) {
			t.Fatalf("after the duplicate cleared: %+v, want job-d reopened", p.Data)
		}
		// Becoming private withdraws it; the removal still carries the slug the client holds.
		mustExec(t, pool, `UPDATE jobs SET is_private = true WHERE public_slug = 'job-d'`)
		p = getChanges(t, app, url.Values{"cursor": {p.Meta.NextCursor}})
		if len(p.Data) != 1 || p.Data[0] != (changeItem{"job-d", "closed", "private"}) {
			t.Fatalf("after going private: %+v, want job-d closed/private", p.Data)
		}
	})

	t.Run("a late commit never lands behind a held cursor", func(t *testing.T) {
		head := getChanges(t, app, url.Values{"since": {since}})
		cursor := head.Meta.NextCursor

		// A writer that began first and commits last, as a bulk close does.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if _, err := tx.Exec(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
			VALUES ('test', 'job-late', 'https://e.test/late', 'Engineer', 'job-late', 'acme')`); err != nil {
			t.Fatal(err)
		}
		seedJob(t, pool, "job-e", "") // commits while the writer above is still open

		held := getChanges(t, app, url.Values{"cursor": {cursor}})
		for _, it := range held.Data {
			if it.Slug == "job-late" || it.Slug == "job-e" {
				t.Fatalf("served %+v while an older transaction was still open", it)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		after := getChanges(t, app, url.Values{"cursor": {held.Meta.NextCursor}})
		slugs := map[string]bool{}
		for _, it := range after.Data {
			slugs[it.Slug] = true
		}
		if !slugs["job-late"] || !slugs["job-e"] || len(after.Data) != 2 {
			t.Fatalf("after commit got %+v, want job-late and job-e exactly once", after.Data)
		}
	})
}

func TestPruneJobChangesKeepsRecentEntries(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	mustExec(t, pool, `INSERT INTO job_changes (job_id, kind, at) VALUES (1, 'opened', now() - interval '40 days'), (2, 'opened', now())`)
	n, err := db.New(pool).PruneJobChanges(ctx, db.PruneJobChangesParams{
		Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-31 * 24 * time.Hour), Valid: true}, BatchSize: 10,
	})
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v; want exactly the 40-day-old entry", n, err)
	}
}
