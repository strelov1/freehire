//go:build integration

// Integration tests for the global discussions feed (GET /api/v1/threads/recent) and
// for the end-of-pagination signal shared by every community listing. Both against a
// real Postgres, because what is being asserted is the SQL: the two subject joins that
// resolve a stored slug to a display name, and their LEFT-ness.
// Run with: go test -tags=integration ./internal/api/handler/
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

	"github.com/strelov1/freehire/internal/engage/community"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// feedRow is one row of the feed's wire shape.
type feedRow struct {
	ID             int64  `json:"id"`
	SubjectType    string `json:"subject_type"`
	SubjectSlug    string `json:"subject_slug"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	SubjectTitle   string `json:"subject_title"`
	SubjectCompany string `json:"subject_company"`
}

// getFeed issues a feed request and decodes the envelope. cookie is empty for the
// unauthenticated case, which must still succeed.
func getFeed(t *testing.T, app *fiber.App, cookie, cursor string) (rows []feedRow, nextCursor string, raw string) {
	t.Helper()
	url := "/api/v1/threads/recent"
	if cursor != "" {
		url += "?cursor=" + cursor
	}
	r := httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, url, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d, want 200: %s", resp.StatusCode, body)
	}
	var env struct {
		Data []feedRow `json:"data"`
		Meta struct {
			NextCursor string `json:"next_cursor"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode feed: %v (%s)", err, body)
	}
	return env.Data, env.Meta.NextCursor, string(body)
}

// seedThread opens a thread and asserts it was created, closing the response body —
// these tests only care that the fixture exists, not what the create returned.
func seedThread(t *testing.T, app *fiber.App, cookie, subjectType, slug string) {
	t.Helper()
	resp := postThread(t, app, cookie, subjectType, slug)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("seed thread on %s/%s: status %d: %s", subjectType, slug, resp.StatusCode, body)
	}
}

func TestCommunityFeedResolvesSubjects(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('feed@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('acme', 'Acme Inc.', 1)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	// The company column is what the feed projects as subject_company for a job, and
	// it is the key the logo proxy resolves by — so it is seeded deliberately here.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('greenhouse', 'eng:1', 'http://example.test', 'Senior Go Engineer', 'Acme Inc.', 'senior-go-engineer-eng-1')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(user, testTokenVersion)
	queries := db.New(pool)
	app := newCommunityApp(queries, iss, community.Config{})

	// A thread on each subject type, company first so the job thread is the newer one.
	for _, subj := range []struct{ typ, slug string }{
		{community.SubjectCompany, "acme"},
		{community.SubjectJob, "senior-go-engineer-eng-1"},
	} {
		seedThread(t, app, cookie, subj.typ, subj.slug)
	}

	t.Run("spans subjects, newest first, names each", func(t *testing.T) {
		rows, _, raw := getFeed(t, app, cookie, "")
		if len(rows) != 2 {
			t.Fatalf("want 2 rows, got %d: %s", len(rows), raw)
		}
		if rows[0].SubjectType != community.SubjectJob {
			t.Fatalf("want the job thread first (newest), got %s", rows[0].SubjectType)
		}
		if rows[0].SubjectTitle != "Senior Go Engineer" || rows[0].SubjectCompany != "Acme Inc." {
			t.Fatalf("job row subject = %q / %q", rows[0].SubjectTitle, rows[0].SubjectCompany)
		}
		if rows[1].SubjectTitle != "Acme Inc." || rows[1].SubjectCompany != "Acme Inc." {
			t.Fatalf("company row subject = %q / %q", rows[1].SubjectTitle, rows[1].SubjectCompany)
		}
		if strings.Contains(raw, "user_id") {
			t.Fatalf("feed leaks a user id: %s", raw)
		}
	})

	t.Run("readable without a session", func(t *testing.T) {
		rows, _, _ := getFeed(t, app, "", "")
		if len(rows) != 2 {
			t.Fatalf("want 2 rows for an anonymous reader, got %d", len(rows))
		}
		if rows[0].Author == "" {
			t.Fatal("want the persona handle present for an anonymous reader")
		}
	})

	// No FK binds a thread to its subject, and cmd/prune hard-deletes jobs. The thread
	// must survive in the feed with an unresolved name; an INNER JOIN would drop it.
	t.Run("thread outlives a deleted subject", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE public_slug = 'senior-go-engineer-eng-1'`); err != nil {
			t.Fatalf("delete job: %v", err)
		}
		rows, _, raw := getFeed(t, app, cookie, "")
		if len(rows) != 2 {
			t.Fatalf("want the orphaned thread kept, got %d rows: %s", len(rows), raw)
		}
		if rows[0].SubjectTitle != "" || rows[0].SubjectCompany != "" {
			t.Fatalf("want empty subject names, got %q / %q", rows[0].SubjectTitle, rows[0].SubjectCompany)
		}
		if rows[0].SubjectSlug != "senior-go-engineer-eng-1" {
			t.Fatalf("want the slug preserved for the client fallback, got %q", rows[0].SubjectSlug)
		}
	})

	t.Run("closed threads excluded", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE threads SET status = 'closed'`); err != nil {
			t.Fatalf("close threads: %v", err)
		}
		rows, _, raw := getFeed(t, app, cookie, "")
		if len(rows) != 0 {
			t.Fatalf("want closed threads excluded, got %d rows: %s", len(rows), raw)
		}
	})
}

// The cursor must promise a further page only when one exists. A cursor on a partial
// page is what drew a "Load more" button that fetched nothing.
func TestCommunityListingsCursorOnlyOnFullPage(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('cursor@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('acme', 'Acme Inc.', 1)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(user, testTokenVersion)
	queries := db.New(pool)
	// A page size of 2 keeps the fixture small; the rule under test is
	// "rows == page size", not the production number.
	const pageSize = 2
	app := newCommunityApp(queries, iss, community.Config{PageSize: pageSize, ThreadCap: 100, ReplyCap: 100})

	subjectListURL := "/api/v1/threads?subject_type=" + community.SubjectCompany + "&subject_slug=acme"

	// One thread: a partial page, so no cursor anywhere.
	seedThread(t, app, cookie, community.SubjectCompany, "acme")
	t.Run("partial page omits the cursor", func(t *testing.T) {
		if cur := cursorOf(t, app, subjectListURL); cur != "" {
			t.Fatalf("want no cursor on a 1-row page, got %q", cur)
		}
		if _, cur, _ := getFeed(t, app, cookie, ""); cur != "" {
			t.Fatalf("want no feed cursor on a 1-row page, got %q", cur)
		}
	})

	// A second thread fills the page exactly, so a cursor is owed.
	seedThread(t, app, cookie, community.SubjectCompany, "acme")
	t.Run("full page carries a cursor that pages", func(t *testing.T) {
		cur := cursorOf(t, app, subjectListURL)
		if cur == "" {
			t.Fatalf("want a cursor on a full page")
		}
		if _, feedCur, _ := getFeed(t, app, cookie, ""); feedCur == "" {
			t.Fatal("want a feed cursor on a full page")
		}
		// The continuation is the last page: 2 threads total, 2 consumed.
		var env struct {
			Data []feedRow `json:"data"`
		}
		decodeGet(t, app, subjectListURL+"&cursor="+cur, &env)
		if len(env.Data) != 0 {
			t.Fatalf("want the continuation empty, got %d rows", len(env.Data))
		}
	})

	// Replies follow the same rule; one reply is a partial page.
	t.Run("a short reply list omits the cursor", func(t *testing.T) {
		var threadID int64
		if err := pool.QueryRow(ctx, `SELECT id FROM threads ORDER BY id LIMIT 1`).Scan(&threadID); err != nil {
			t.Fatalf("read thread id: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO thread_replies (thread_id, author_user_id, body) VALUES ($1, $2, 'one')`,
			threadID, user); err != nil {
			t.Fatalf("seed reply: %v", err)
		}
		var env struct {
			Meta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"meta"`
		}
		decodeGet(t, app, fmt.Sprintf("/api/v1/threads/%d", threadID), &env)
		if env.Meta.NextCursor != "" {
			t.Fatalf("want no reply cursor on a 1-reply thread, got %q", env.Meta.NextCursor)
		}
	})
}

// cursorOf returns meta.next_cursor for a listing URL, or "" when absent.
func cursorOf(t *testing.T, app *fiber.App, url string) string {
	t.Helper()
	var env struct {
		Meta struct {
			NextCursor string `json:"next_cursor"`
		} `json:"meta"`
	}
	decodeGet(t, app, url, &env)
	return env.Meta.NextCursor
}

// decodeGet GETs url, asserts 200, and decodes the body into out.
func decodeGet(t *testing.T, app *fiber.App, url string, out any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, url, nil))
	if err != nil {
		t.Fatalf("app.Test(%s): %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s: status %d: %s", url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v (%s)", url, err, body)
	}
}
