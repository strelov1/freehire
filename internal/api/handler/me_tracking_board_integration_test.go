//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TestTrackedBoardPagesOrphanedApplicationsWithoutDuplication covers the fix for
// ListOrphanedApplications' missing OFFSET (it always re-read the same top-N orphans on
// every page) and for CountUserJobs never counting them (meta.counts under-reported
// while the merged data could exceed it). Several applications whose postings were
// pruned, paged one at a time, must come back as distinct rows — never the same one
// twice — and meta.counts.board/applied/all must include every orphan.
func TestTrackedBoardPagesOrphanedApplicationsWithoutDuplication(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('orphan-paging@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	const orphanCount = 3
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	for i := 0; i < orphanCount; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at)
			 VALUES ($1, $2, 'Orphaned Role', NULL, $3)`,
			userID, "orphanco", base.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("seed orphaned application %d: %v", i, err)
		}
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	queries := db.New(pool)
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{trackingHandlers: h}))

	type response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Meta struct {
			Counts struct {
				All     int64 `json:"all"`
				Applied int64 `json:"applied"`
				Board   int64 `json:"board"`
			} `json:"counts"`
		} `json:"meta"`
	}

	get := func(offset int) response {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet,
			fmt.Sprintf("/api/v1/me/tracking?filter=board&limit=1&offset=%d", offset), nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token}) //nolint:gosec // a request cookie: the Set-Cookie attributes do not apply
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("request offset=%d: %v", offset, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("offset=%d status = %d, want 200", offset, resp.StatusCode)
		}
		var body response
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode offset=%d: %v", offset, err)
		}
		return body
	}

	seen := map[string]bool{}
	var counts response
	for offset := 0; offset < orphanCount; offset++ {
		page := get(offset)
		counts = page
		if len(page.Data) != 1 {
			t.Fatalf("offset=%d returned %d rows, want exactly 1", offset, len(page.Data))
		}
		id := page.Data[0].ID
		if seen[id] {
			t.Fatalf("offset=%d returned id %q again — paging is re-reading the same orphan", offset, id)
		}
		seen[id] = true
	}
	if len(seen) != orphanCount {
		t.Fatalf("saw %d distinct orphaned applications across %d pages, want %d", len(seen), orphanCount, orphanCount)
	}

	if counts.Meta.Counts.All != orphanCount {
		t.Errorf("meta.counts.all = %d, want %d — CountInteractions must fold in the orphaned applications", counts.Meta.Counts.All, orphanCount)
	}
	if counts.Meta.Counts.Applied != orphanCount {
		t.Errorf("meta.counts.applied = %d, want %d", counts.Meta.Counts.Applied, orphanCount)
	}
	if counts.Meta.Counts.Board != orphanCount {
		t.Errorf("meta.counts.board = %d, want %d", counts.Meta.Counts.Board, orphanCount)
	}
}

// TestTrackedBoardPagesTheMergedListAsOneWindow covers the fix for ListInteractions
// applying ONE (limit, offset) to TWO queries.
//
// The board is posting-backed rows followed by orphaned applications, and (limit, offset)
// addresses that merged list. Handing the same window to both queries returned up to
// 2*limit rows on a page — so a client walking by response length (which is exactly what
// the SPA's Paginator does) jumped past a window it never received — and returned an empty
// page past the posting-backed total under a meta.total that says there is more, which is a
// loop that does not terminate.
//
// The assertion is the walk itself: page by page at limit=2 over 3 posting-backed and 2
// orphaned rows, every page is at most a full limit, every id appears exactly once, and all
// five arrive.
func TestTrackedBoardPagesTheMergedListAsOneWindow(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('merged-window@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	const (
		postingBacked = 3
		orphaned      = 2
		pageSize      = 2
	)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	for i := range postingBacked {
		var jobID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
			 VALUES ('greenhouse', $1, 'http://example.test', 'Go Developer', 'Acme', 'acme', $1)
			 RETURNING id`, fmt.Sprintf("window-job-%d", i)).Scan(&jobID); err != nil {
			t.Fatalf("seed job %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, now())`,
			userID, jobID); err != nil {
			t.Fatalf("seed user_job %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO applications (user_id, job_id, company_slug, role_title, applied_at)
			 VALUES ($1, $2, 'acme', 'Go Developer', $3)`,
			userID, jobID, base.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("seed application %d: %v", i, err)
		}
	}
	for i := range orphaned {
		if _, err := pool.Exec(ctx,
			`INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at)
			 VALUES ($1, 'orphanco', 'Orphaned Role', NULL, $2)`,
			userID, base.Add(-time.Duration(postingBacked+i)*time.Minute)); err != nil {
			t.Fatalf("seed orphaned application %d: %v", i, err)
		}
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(db.New(pool), pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{trackingHandlers: h}))

	type page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	get := func(offset int) page {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, fiber.MethodGet,
			fmt.Sprintf("/api/v1/me/tracking?filter=board&limit=%d&offset=%d", pageSize, offset), nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token}) //nolint:gosec // a request cookie: the Set-Cookie attributes do not apply
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("request offset=%d: %v", offset, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("offset=%d status = %d, want 200", offset, resp.StatusCode)
		}
		var body page
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode offset=%d: %v", offset, err)
		}
		return body
	}

	// Walk the way a client does: advance by however many rows the last page returned,
	// and stop when the walked count reaches the reported total.
	seen := map[string]bool{}
	offset := 0
	for step := 0; offset < postingBacked+orphaned; step++ {
		if step > postingBacked+orphaned {
			t.Fatalf("the walk did not terminate: %d pages for %d rows", step, postingBacked+orphaned)
		}
		p := get(offset)
		if len(p.Data) > pageSize {
			t.Fatalf("offset=%d returned %d rows, want at most the requested %d — the two "+
				"queries are being given the same window instead of dividing it",
				offset, len(p.Data), pageSize)
		}
		if len(p.Data) == 0 {
			t.Fatalf("offset=%d returned nothing while total says %d — the page after the "+
				"posting-backed rows must carry the orphans, not an empty list",
				offset, p.Meta.Total)
		}
		for _, row := range p.Data {
			if seen[row.ID] {
				t.Fatalf("offset=%d returned id %q again", offset, row.ID)
			}
			seen[row.ID] = true
		}
		offset += len(p.Data)
	}
	if len(seen) != postingBacked+orphaned {
		t.Errorf("the walk saw %d distinct rows, want %d — a window skipped over is a row "+
			"nobody can reach", len(seen), postingBacked+orphaned)
	}
}
