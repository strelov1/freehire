# Company Closed Postings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A "Closed" tab on the company page listing that company's closed postings, read from Postgres.

**Architecture:** One sqlc query walking the existing `jobs_company_slug_idx`, one anonymous API route
answering `{data, meta.has_more}` (no total), and one SvelteKit route `/companies/[slug]/closed`
rendered as the fourth company tab. Meilisearch is untouched.

**Tech Stack:** Go + Fiber v2, sqlc, PostgreSQL, SvelteKit 3 (Svelte 5 runes).

Spec: `docs/superpowers/specs/2026-10-22-company-closed-jobs-design.md`.

## Global Constraints

- Worktree: `/Users/i_strelov/Projects/hire-closed-jobs`, branch `company-closed-jobs`. Never commit from the main checkout.
- No migration. The query's `ORDER BY posted_at DESC NULLS LAST, id DESC` must match `jobs_company_slug_idx` exactly.
- Exclusions identical to `ListJobsByCompany`: `duplicate_of IS NULL AND NOT is_private`.
- No exact count anywhere on this path.
- English in all code, comments, UI copy.
- Before committing Go: `gofmt -l .` prints nothing, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`.
- Generated files are regenerated, never hand-edited: `make sqlc`, `pnpm run gen:api-docs`, `pnpm run gen:openapi` (in `web/`).
- No prettier (the repo has none).

---

### Task 1: The query

**Files:**
- Modify: `internal/platform/db/queries/jobs.sql` (after `ListJobsByCompany`, ~line 259)
- Regenerate: `internal/platform/db/jobs.sql.go`, `internal/platform/db/querier.go` (`make sqlc`)
- Create: `internal/platform/db/company_closed_jobs_integration_test.go`

**Interfaces:**
- Produces: `func (q *Queries) ListClosedJobsByCompany(ctx context.Context, arg ListClosedJobsByCompanyParams) ([]Job, error)` with `ListClosedJobsByCompanyParams{CompanySlug string; Limit int32; Offset int32}`.

- [ ] **Step 1: Write the failing test**

```go
//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// TestListClosedJobsByCompany pins the closed-postings list: only this company's CLOSED,
// canonical, public rows, newest posting first — the same exclusions the open list makes,
// with the closed_at predicate inverted.
func TestListClosedJobsByCompany(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	now := time.Now().UTC()
	seed := func(ext, company string, posted time.Time, closed, private bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, posted_at, closed_at, is_private)
			 VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, $2, $3,
			         CASE WHEN $4::bool THEN now() END, $5)
			 RETURNING id`,
			ext, company, posted, closed, private).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", ext, err)
		}
		return id
	}

	older := seed("closed-old", "acme", now.Add(-72*time.Hour), true, false)
	newer := seed("closed-new", "acme", now.Add(-24*time.Hour), true, false)
	seed("open", "acme", now, false, false)
	seed("closed-private", "acme", now, true, true)
	seed("closed-other", "globex", now, true, false)
	dup := seed("closed-dup", "acme", now, true, false)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of = $1 WHERE id = $2`, newer, dup); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}

	rows, err := q.ListClosedJobsByCompany(ctx, ListClosedJobsByCompanyParams{CompanySlug: "acme", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got []int64
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if len(got) != 2 || got[0] != newer || got[1] != older {
		t.Fatalf("ids = %v, want [%d %d] (closed, canonical, public, newest posting first)", got, newer, older)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go vet -tags=integration ./internal/platform/db/`
Expected: FAIL — `q.ListClosedJobsByCompany undefined`.

- [ ] **Step 3: Add the query**

Insert after `ListJobsByCompany` in `internal/platform/db/queries/jobs.sql`:

```sql
-- name: ListClosedJobsByCompany :many
-- The company page's Closed tab: the postings this company has closed, newest posting first.
-- The same exclusions as ListJobsByCompany (reposts, private pasted JDs) with the closed_at
-- predicate inverted.
--
-- The ORDER BY is jobs_company_slug_idx's own (company_slug, posted_at DESC NULLS LAST,
-- id DESC — not partial, so it covers closed rows) and must stay so: a large employer holds
-- hundreds of thousands of closed rows, and any other order is a sort over all of them. That
-- is also why nothing counts this set — see the handler.
SELECT *
FROM jobs
WHERE company_slug = $1 AND closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY posted_at DESC NULLS LAST, id DESC
LIMIT $2 OFFSET $3;
```

Run: `make sqlc`

- [ ] **Step 4: Run the test**

Run: `go test -tags=integration -run TestListClosedJobsByCompany ./internal/platform/db/`
Expected: PASS (needs Docker).

Also run the static rule over queries: `go test -run Private ./internal/platform/db/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/ && go vet ./... && go test ./internal/platform/db/
git add internal/platform/db/queries/jobs.sql internal/platform/db/jobs.sql.go internal/platform/db/querier.go internal/platform/db/company_closed_jobs_integration_test.go
git commit -m "Add the query listing a company's closed postings"
```

---

### Task 2: The API route

**Files:**
- Modify: `internal/api/handler/companies.go` (register at line ~42; new handler after `GetCompany`)
- Modify: `internal/api/handler/pagination_window_routes_test.go` (one row in `windowedListRoutes`)
- Create: `internal/api/handler/company_closed_jobs_integration_test.go`
- Modify: `web/src/lib/docs/api-spec.ts` (an entry after `'/companies/{slug}'`, ~line 683) and regenerate docs

**Interfaces:**
- Consumes: `ListClosedJobsByCompany` from Task 1; existing `pageParams`, `lookupCompany`, `jobview.FromRows`.
- Produces: `GET /api/v1/companies/:slug/closed-jobs?limit=&offset=` →
  `{"data": [Job], "meta": {"limit": int, "offset": int, "has_more": bool}}`. 404 unknown slug, 301 retired slug, 400 past the pagination window.

- [ ] **Step 1: Add the window-guard row (fails: route not mounted)**

In `windowedListRoutes`:

```go
	{"GET /companies/:slug/closed-jobs", func() *fiber.App { return mountWindowed((&companiesHandlers{}).register) }, "/api/v1/companies/acme/closed-jobs"},
```

- [ ] **Step 2: Write the failing integration test**

```go
//go:build integration

// Integration test for GET /api/v1/companies/:slug/closed-jobs, the company page's Closed
// tab. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestCompanyClosedJobs(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	for i := range 3 {
		ext := fmt.Sprintf("acme-closed-%d", i)
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, posted_at, closed_at)
			 VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, 'acme', now() - make_interval(days => $2), now())`,
			ext, i); err != nil {
			t.Fatalf("seed job: %v", err)
		}
	}

	h := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies/:slug/closed-jobs", h.CompanyClosedJobs)

	get := func(t *testing.T, url string) (status int, slugs []string, hasMore bool) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			return resp.StatusCode, nil, false
		}
		var body struct {
			Data []struct {
				Slug     string  `json:"slug"`
				ClosedAt *string `json:"closed_at"`
			} `json:"data"`
			Meta struct {
				HasMore bool `json:"has_more"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, j := range body.Data {
			if j.ClosedAt == nil {
				t.Errorf("job %s carries no closed_at", j.Slug)
			}
			slugs = append(slugs, j.Slug)
		}
		return resp.StatusCode, slugs, body.Meta.HasMore
	}

	t.Run("a page short of the end says there is more", func(t *testing.T) {
		_, slugs, more := get(t, "/api/v1/companies/acme/closed-jobs?limit=2")
		if len(slugs) != 2 || !more {
			t.Fatalf("slugs=%v has_more=%v, want 2 rows and has_more", slugs, more)
		}
	})
	t.Run("the last page says there is no more", func(t *testing.T) {
		_, slugs, more := get(t, "/api/v1/companies/acme/closed-jobs?limit=2&offset=2")
		if len(slugs) != 1 || more {
			t.Fatalf("slugs=%v has_more=%v, want 1 row and no more", slugs, more)
		}
	})
	t.Run("an unknown company is a 404", func(t *testing.T) {
		if status, _, _ := get(t, "/api/v1/companies/nope/closed-jobs"); status != fiber.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
	})
}
```

The JSON key for the job's slug must match `jobview.Job`'s tag — check `internal/job/jobview/jobview.go` and adjust `Slug`'s tag if it is not `slug`.

- [ ] **Step 3: Verify both fail**

Run: `go test -run TestDeepOffsetIsRefusedBeforeAnyDatabaseWork ./internal/api/handler/` — Expected: FAIL (404 for the new row).
Run: `go vet -tags=integration ./internal/api/handler/` — Expected: FAIL, `h.CompanyClosedJobs undefined`.

- [ ] **Step 4: Implement**

In `register`, beside the `/hiring` line:

```go
	api.Get("/companies/:slug/closed-jobs", readLimit, h.CompanyClosedJobs)
```

After `GetCompany`:

```go
// CompanyClosedJobs serves the company page's Closed tab: a page of the postings this company
// has closed, newest posting first, from Postgres — closed rows are never in the search index.
//
// It answers has_more instead of a total. A large employer holds hundreds of thousands of
// closed rows, and an exact count over them is the cost this endpoint exists to avoid; one
// extra row fetched past the page says the same thing for free.
func (h *companiesHandlers) CompanyClosedJobs(c *fiber.Ctx) error {
	slug := c.Params("slug")

	// Before the lookup, so a refused page costs no database work (see GetCompany).
	limit, offset, err := pageParams(c)
	if err != nil {
		return err
	}

	if _, redirected, err := h.lookupCompany(c, slug); err != nil || redirected {
		return err
	}

	rows, err := h.queries.ListClosedJobsByCompany(c.Context(), db.ListClosedJobsByCompanyParams{
		CompanySlug: slug,
		Limit:       int32(limit + 1),
		Offset:      int32(offset),
	})
	if err != nil {
		return err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	views, err := jobview.FromRows(rows)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"data": views,
		"meta": fiber.Map{"limit": limit, "offset": offset, "has_more": hasMore},
	})
}
```

Note: `lookupCompany`'s retired-slug redirect goes to `companyPath(c, canonical)`. Read that helper; if it rebuilds only `/companies/<slug>` rather than swapping the slug in the current path, the 301 lands on the company endpoint, not on `/closed-jobs` — acceptable (same as `/hiring` today), but note it in the PR.

- [ ] **Step 5: Run the tests**

Run: `go test -run TestDeepOffsetIsRefusedBeforeAnyDatabaseWork ./internal/api/handler/` — PASS.
Run: `go test -tags=integration -run TestCompanyClosedJobs ./internal/api/handler/` — PASS.

- [ ] **Step 6: Document the route**

In `web/src/lib/docs/api-spec.ts`, directly after the `'/companies/{slug}'` entry, add an entry in the same shape:

```ts
      {
        method: 'GET',
        path: '/companies/{slug}/closed-jobs',
        auth: 'none',
        summary: "A page of a company's closed postings, newest posting first.",
        description:
          'Closed postings are not in search, so this is the only list that serves them. ' +
          'There is no total: `meta.has_more` says whether another page follows.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
        query: [
          { name: 'limit', type: 'integer', description: 'Page size.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/companies/acme/closed-jobs"`,
        responseExample: `{
  "data": [{ "slug": "backend-engineer-acme-1a2b3c4d", "title": "Backend Engineer", "closed_at": "2026-10-01T09:00:00Z" }],
  "meta": { "limit": 20, "offset": 0, "has_more": true }
}`,
      },
```

Match the neighbouring entry's exact field names (`method`, `description` etc.) — copy from the entry at ~line 683 and drop any field it does not have.

Run in `web/`: `pnpm run gen:api-docs && pnpm run gen:openapi && pnpm run gen:openapi:smoke`

- [ ] **Step 7: Commit**

```bash
gofmt -l internal/ && go vet ./... && go vet -tags=integration ./... && go test ./...
git add internal/api/handler/companies.go internal/api/handler/pagination_window_routes_test.go internal/api/handler/company_closed_jobs_integration_test.go web/src/lib/docs/ docs/API*.md
git commit -m "Serve a company's closed postings at /companies/:slug/closed-jobs"
```

---

### Task 3: The Closed tab

**Files:**
- Modify: `web/src/lib/api.ts` (method after `getCompany`, ~line 873; export it in the returned object ~line 3153)
- Modify: `web/src/lib/components/CompanyTabs.svelte` (fourth tab)
- Create: `web/src/routes/companies/[slug]/closed/+page.server.ts`
- Create: `web/src/routes/companies/[slug]/closed/+page.svelte`

**Interfaces:**
- Consumes: the route from Task 2.
- Produces: `companyClosedJobs(slug: string, limit: number, offset: number): Promise<{ jobs: Job[]; hasMore: boolean }>`.

- [ ] **Step 1: API client method**

```ts
  /** The company page's Closed tab: a page of the company's closed postings. No total —
   *  the endpoint reports only whether another page follows. */
  async function companyClosedJobs(
    slug: string,
    limit: number,
    offset: number,
  ): Promise<{ jobs: Job[]; hasMore: boolean }> {
    const res = await request<{ data: Job[]; meta: { has_more: boolean } }>(
      `/api/v1/companies/${encodeURIComponent(slug)}/closed-jobs${query(limit, offset)}`,
    );
    return { jobs: res.data, hasMore: res.meta.has_more };
  }
```

Add `companyClosedJobs,` to the returned object beside `getCompany,`.

- [ ] **Step 2: Tab**

In `CompanyTabs.svelte`: `type CompanyTab = 'jobs' | 'hiring' | 'closed' | 'discussion';` and in `tabs`, after the hiring entry:

```ts
    { id: 'closed' as const, label: 'Closed', href: resolve('/companies/[slug]/closed', { slug }) },
```

Update the comment above the type to name the closed list among the views.

- [ ] **Step 3: Loader**

`web/src/routes/companies/[slug]/closed/+page.server.ts`:

```ts
import { error, redirect } from '@sveltejs/kit';
import { ApiError, MovedError } from '#lib/api.js';
import { PAGE_SIZE, pageOffset, pageWithinWindow, parsePage } from '#lib/pagination.js';
import { serverApi } from '#lib/server/api.js';
import { rethrowUpstream } from '#lib/server/upstream.js';
import type { PageServerLoad } from './$types';

// The Closed tab: the company's closed postings, read from Postgres (closed rows are never
// in the search index). The company read decides the page's fate exactly as on the other
// tabs; a failed LIST resolves to null so the header still renders.
export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const client = serverApi(fetch);
  const pageNumber = parsePage(url.searchParams);
  if (!pageWithinWindow(pageNumber)) error(404, 'Page not found');

  const closed = client.companyClosedJobs(params.slug, PAGE_SIZE, pageOffset(pageNumber)).catch(() => null);

  let entity;
  try {
    entity = await client.getCompany(params.slug, 1, 0);
  } catch (e) {
    if (e instanceof MovedError) redirect(301, `/companies/${e.canonicalSlug}/closed${url.search}`);
    if (e instanceof ApiError && e.status === 404) error(404, 'Company not found');
    rethrowUpstream(e);
  }

  return { slug: params.slug, company: entity.company, pageNumber, closed: await closed };
};
```

- [ ] **Step 4: Page**

`web/src/routes/companies/[slug]/closed/+page.svelte`:

```svelte
<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import CompanyHeader from '#lib/components/CompanyHeader.svelte';
  import CompanyTabs from '#lib/components/CompanyTabs.svelte';
  import JobRow from '#lib/components/JobRow.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { pageHref } from '#lib/pagination.js';
  import { Breadcrumbs } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const companyHref = $derived(resolve('/companies/[slug]', { slug: data.slug }));
  const name = $derived(data.company.name);
  const pathname = $derived(page.url.pathname);
  const noParams = new URLSearchParams();
  const breadcrumbItems = $derived([
    { name: 'Companies', href: '/companies' },
    { name, href: companyHref },
    { name: 'Closed' },
  ]);

  const closedOn = (iso: string) =>
    new Date(iso).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
</script>

<!-- noindex: one page per company per offset of postings nobody can apply to is not
     something to hand a crawler. The postings' own pages stay indexable. -->
<Seo
  title={`${name} — closed positions · freehire`}
  description={`Positions ${name} posted that are no longer accepting applications.`}
  robots="noindex, follow"
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <Breadcrumbs items={breadcrumbItems} class="mb-4" />

  {#key data.slug}
    <CompanyHeader company={data.company} slug={data.slug} />

    <CompanyTabs slug={data.slug} active="closed">
      <div class="mt-6 max-w-3xl">
        {#if !data.closed}
          <p class="py-16 text-center text-sm text-muted-foreground">Couldn't load closed positions.</p>
        {:else if data.closed.jobs.length === 0}
          <p class="py-16 text-center text-sm text-muted-foreground">
            {data.pageNumber > 1 ? 'No more closed positions.' : `${name} has no closed positions on freehire.`}
          </p>
        {:else}
          <ul class="flex flex-col">
            {#each data.closed.jobs as job (job.slug)}
              <li>
                <JobRow {job} />
                {#if job.closed_at}
                  <p class="px-4 pb-3 text-xs text-muted-foreground">Closed {closedOn(job.closed_at)}</p>
                {/if}
              </li>
            {/each}
          </ul>

          <nav class="mt-6 flex justify-between text-sm" aria-label="Pages">
            {#if data.pageNumber > 1}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- pathname comes from page.url, already resolved (see Pagination.svelte) -->
              <a class="underline hover:text-foreground" href={pageHref(pathname, noParams, data.pageNumber - 1)}>← Newer</a>
            {:else}
              <span></span>
            {/if}
            {#if data.closed.hasMore}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- as above -->
              <a class="underline hover:text-foreground" href={pageHref(pathname, noParams, data.pageNumber + 1)}>Older →</a>
            {/if}
          </nav>
        {/if}
      </div>
    </CompanyTabs>
  {/key}
</div>
```

Check before running: the `Job` type's slug field name (`job.slug` vs `job.public_slug`) in `web/src/lib/types.ts`, and `Seo`'s prop list (whether `canonical` is required). Adjust the markup to match; use whatever `JobsView` keys its rows by.

- [ ] **Step 5: Verify**

In `web/`: `pnpm exec svelte-kit sync && pnpm run check && pnpm exec eslint src/routes/companies src/lib/components/CompanyTabs.svelte src/lib/api.ts && pnpm test --run`
Expected: no errors.

Then run the app against local API (`make up`, `pnpm dev` in `web/`), open `/companies/<a slug with closed rows>/closed`, confirm: the tab is active, rows show "Closed <date>", "Older →" pages, an unknown slug 404s, page source carries `noindex`.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/components/CompanyTabs.svelte web/src/routes/companies/\[slug\]/closed/
git commit -m "Add a Closed tab to the company page"
```

---

### Task 4: Ship

- [ ] `go vet -tags=integration ./...` and the tagged tests touched above, once more.
- [ ] Push `company-closed-jobs`, open the PR (spec + plan linked), wait for CI.
- [ ] Merge, deploy (autodeploy, or `ssh root@136.243.174.114 /opt/freehire/bin/release.sh freehire`).
- [ ] On prod: `curl -s "https://freehire.me/api/v1/companies/<big employer slug>/closed-jobs?limit=20" -w '%{time_total}\n' -o /dev/null` — expect well under a second; open the tab in a browser.
