//go:build integration

// A measurement, not an assertion. It exists to answer one question with a number before
// anything is optimised: when the tracking board server-loads 500 rows, what is the cost made
// of — the query, or the bytes?
//
// go test -tags=integration -run TestMeasureBoardLoad -v ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobtracking"
)

// A job description of a size the catalogue actually carries. Real postings run 3–8 KB of
// prose; 5 KB is the middle of that and keeps the arithmetic honest either way.
const measuredDescriptionBytes = 5000

func TestMeasureBoardLoad(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('board-load@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	const rows = 500
	description := strings.Repeat("Responsibilities and requirements. ", measuredDescriptionBytes/35)
	for i := range rows {
		var jobID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, company_slug, public_slug, description)
			 VALUES ('test', $1, 'http://example.test', 'Senior Go Engineer', 'acme', $2, $3)
			 RETURNING id`,
			fmt.Sprintf("load-%d", i), fmt.Sprintf("load-%d", i), description).Scan(&jobID); err != nil {
			t.Fatalf("seed job %d: %v", i, err)
		}
		if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: jobID}); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
		// Every third application has linked mail — the three correlated subqueries over
		// `emails` are the ones under suspicion, and they must have rows to find.
		if i%3 == 0 {
			if _, err := pool.Exec(ctx,
				`INSERT INTO emails (user_id, source, external_id, from_addr, from_name, subject, body_text, received_at, job_id)
				 VALUES ($1, 'gmail', $2, 'noreply@ashbyhq.com', 'Ashby', 'Thanks for applying', 'body', now(), $3)`,
				userID, fmt.Sprintf("msg-%d", i), jobID); err != nil {
				t.Fatalf("seed email %d: %v", i, err)
			}
		}
	}

	// A real mailbox is not 167 messages about applications; it is a synced inbox where those
	// are a minority. Without this the LATERAL has almost nothing to scan and an index over it
	// cannot show its worth — the measurement would say "indexes do nothing" because the
	// fixture was too small to need one.
	for i := range 5000 {
		if _, err := pool.Exec(ctx,
			`INSERT INTO emails (user_id, source, external_id, from_addr, from_name, subject, body_text, received_at)
			 VALUES ($1, 'gmail', $2, 'someone@example.test', 'Someone', 'Unrelated', 'body', now())`,
			userID, fmt.Sprintf("noise-%d", i)); err != nil {
			t.Fatalf("seed noise email %d: %v", i, err)
		}
	}

	// Bulk inserts leave the planner on statistics from an empty table, which costs two orders
	// of magnitude here and has nothing to do with the code under measurement — on production
	// autovacuum has long since done this. Without it the fixture measures its own seeding.
	if _, err := pool.Exec(ctx, `ANALYZE emails, cvs, user_jobs, applications, jobs`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking", auth.RequireAuth(iss, testVersions), h.ListTrackedJobs)

	// Warm the pool and the plan cache so the reported number is steady state, not first-call
	// cost — the board is opened repeatedly by a signed-in user, never once.
	for range 3 {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking?filter=board&limit=500", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req, 60_000)
		if err != nil {
			t.Fatalf("warmup: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	var best time.Duration
	var body []byte
	for i := range 5 {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking?filter=board&limit=500", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		start := time.Now()
		resp, err := app.Test(req, 60_000)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		elapsed := time.Since(start)
		if i == 0 || elapsed < best {
			best, body = elapsed, b
		}
	}

	// How much of the payload is description text nobody on the board renders?
	var parsed struct {
		Data []struct {
			Job *struct {
				Description string `json:"description"`
			} `json:"job"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var descriptionBytes int
	for _, item := range parsed.Data {
		if item.Job != nil {
			descriptionBytes += len(item.Job.Description)
		}
	}

	// The query alone, without serialization, so the two costs can be told apart.
	var queryBest time.Duration
	for i := range 5 {
		start := time.Now()
		if _, err := queries.ListUserJobs(ctx, db.ListUserJobsParams{
			UserID: userID, Filter: "board", Limit: 500, Offset: 0,
		}); err != nil {
			t.Fatalf("ListUserJobs: %v", err)
		}
		if elapsed := time.Since(start); i == 0 || elapsed < queryBest {
			queryBest = elapsed
		}
	}

	// The same shape of scan with and without the description column, to tell the cost of
	// READING it from the cost of SENDING it. `description` is large enough to be TOASTed, so
	// not selecting it should skip a separate fetch per row — but "should" is why we measure.
	measureScan := func(t *testing.T, label, columns string) time.Duration {
		t.Helper()
		query := `SELECT ` + columns + `
			  FROM user_jobs uj
			  JOIN jobs ON jobs.id = uj.job_id
			  LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
			 WHERE uj.user_id = $1 LIMIT 500`
		var best time.Duration
		for i := range 5 {
			start := time.Now()
			rows, err := pool.Query(ctx, query, userID)
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			for rows.Next() {
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatalf("%s rows: %v", label, err)
			}
			if elapsed := time.Since(start); i == 0 || elapsed < best {
				best = elapsed
			}
		}
		return best
	}
	withDescription := measureScan(t, "with description", "jobs.id, jobs.title, jobs.company_slug, jobs.enrichment, jobs.description")
	withoutDescription := measureScan(t, "without description", "jobs.id, jobs.title, jobs.company_slug, jobs.enrichment")

	// The guard. The board renders no posting text, so carrying it is pure freight: at 5 KB a
	// description and 500 rows it was 2.37 MB of a 2.83 MB response, serialized again into the
	// SSR payload and parsed again by the browser.
	if descriptionBytes != 0 {
		t.Errorf("the listing carries %d bytes of description; the board renders none of it, and "+
			"the full posting is one read away at GET /me/tracking/:slug", descriptionBytes)
	}

	// A ceiling rather than an exact size: the card may gain a field, and this should fail when
	// something large comes back, not when something small is added. 500 rows of card at the
	// measured ~900 bytes each leaves generous room under 1 MB.
	const payloadCeiling = 1 << 20
	if len(body) > payloadCeiling {
		t.Errorf("payload = %.2f MB for %d rows, want under %.2f MB",
			float64(len(body))/1024/1024, len(parsed.Data), float64(payloadCeiling)/1024/1024)
	}

	t.Logf("rows=%d", len(parsed.Data))
	t.Logf("scan WITH description:    %v", withDescription)
	t.Logf("scan WITHOUT description: %v", withoutDescription)
	t.Logf("query (best of 5):     %v", queryBest)
	t.Logf("endpoint (best of 5):  %v", best)
	t.Logf("payload:               %.2f MB", float64(len(body))/1024/1024)
	t.Logf("of which descriptions: %.2f MB (%.0f%%)",
		float64(descriptionBytes)/1024/1024, float64(descriptionBytes)*100/float64(len(body)))
}
