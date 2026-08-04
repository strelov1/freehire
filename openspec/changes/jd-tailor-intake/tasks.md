## 1. Schema

- [x] 1.1 Add a new migration: `jobs.is_private boolean not null default false` (never edit an
      applied migration file).
- [x] 1.2 Regenerate sqlc (`make sqlc`) so `db.Job` and relevant query params expose `IsPrivate`.

## 2. Private job creation

- [x] 2.1 Add a synthetic `external_id` generator (fresh UUID per submission) for the private
      path — never compared against the public `(source, external_id)` dedup space.
- [x] 2.2 Implement a private-job writer: given title/company/description (+ URL when present),
      run `internal/jobderive.Derive` synchronously, insert a `jobs` row with
      `is_private = true`, `created_by = userID`, `source` = `"pasted"` (text) or `"weblink"`
      (URL), and do **not** enqueue it onto `enrichment_outbox`.
- [x] 2.3 Unit tests for the writer: facets populated from `Derive`, no `enrichment_outbox` row
      created, two calls (same or different user, same input) never collide on `external_id`.

## 3. URL/text resolution branching

- [x] 3.1 Add a resolver that classifies a submitted `url`: recognized ATS (host-scoped
      `internal/linksource` adapter or `internal/atsboard` board coverage) vs. generic scrape /
      unreadable — reusing `linkimport`'s resolution step directly, without its contribution-
      reward recording.
- [x] 3.2 Wire the recognized-ATS branch to the existing `pipeline.UpsertJob` write path (public,
      enrichment-queued, indexed — identical to normal ingest).
- [x] 3.3 Wire the generic/unrecognized-URL branch and the plain-`text` branch to the private-job
      writer from 2.2; an unreadable/unparseable URL yields no row.
- [x] 3.4 Integration tests: known `job_slug` passthrough; recognized-ATS URL → public job;
      generic/unrecognized URL → private job; unreadable URL → no row; plain text → private job;
      the same text submitted by two different users → two independent private rows.

## 4. Resolve endpoint

- [x] 4.1 Define request/response types and validation for `POST /api/v1/me/jd/resolve`: exactly
      one of `job_slug` / `url` / `text` required, `text` non-empty when present.
- [x] 4.2 Implement the handler wiring the 3.x resolver behind `RequireAuth`, returning
      `{job_slug}`.
- [x] 4.3 Handler tests: 200 + slug for each of the three input kinds; 400 for zero or multiple
      inputs; 401 unauthenticated; 422 for an unreadable URL.

## 5. Visibility: unguessable + never-listed (not per-request ownership gating)

Privacy model settled after an audit found gating every job-by-slug consumer
(`GetJob`/fit-analysis/`TailorCV`, then `job_match.go`, then jobtracking's ~11 methods, then 7
more: vote/reminder/similar/copies/ghost_reports/reports/community) was an open-ended
whack-a-mole, not a fixed list. Settled instead on: a private job behaves like a **closed** job
for read access (reachable by anyone holding its exact slug, unchanged across every existing
consumer), and relies on the slug's synthetic-UUID shortcode being unguessable. Only listing
surfaces reachable WITHOUT already knowing the slug need to actively exclude `is_private`.

- [x] 5.1 Exclude `is_private` rows from `internal/search`'s Meilisearch-indexing query
      (`cmd/reindex`'s `splitJobs`).
- [x] 5.2 Exclude `is_private` rows (and their count) from the DB-backed `GET /api/v1/jobs` list
      query and `estimate_open_jobs()`.
- [x] 5.3 Exclude `is_private` rows from `GET /jobs/:slug/copies` (`ListRoleClusterCopies`): a
      private job can coincidentally share a role cluster (`company_slug` + `role_fingerprint`)
      with an unrelated public one, surfacing its slug/location/url to anyone browsing that
      public job's copies — the one listing path reachable without already knowing the private
      slug.
- [x] 5.4 Tests: a private job is absent from a Meilisearch reindex, from `GET /api/v1/jobs`, and
      from another job's copies list even when it shares a role cluster; its own slug still
      resolves normally through `GetJob`/fit-analysis/`TailorCV`/`job_match`/jobtracking (no
      regression from treating it like a closed job for direct-link access).

## 6. Frontend — `/my/cvs` entry point

- [x] 6.1 Add the API client method for `POST /api/v1/me/jd/resolve`
      (`web/src/lib/api.ts`).
- [x] 6.2 Add a "Tailor for a job" button + form (`JdIntakeDialog.svelte`, mounted from
      `CvList.svelte`) with three tabs: existing-job search/select, URL input, and text input
      (with optional title/company fields).
- [x] 6.3 Wire the "our vacancy" tab to redirect straight to `/tailor/[slug]` (no backend call).
- [x] 6.4 Wire the URL and text tabs to the new endpoint, handling loading/422/401 states, and
      redirect to `/tailor/[job_slug]` on success.
