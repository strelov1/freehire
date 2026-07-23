## Context

The codebase already has the exact building blocks this feature needs:

- **Per-(user, job) storage**: `user_jobs` holds one row per (user, job) with
  `viewed_at` / `applied_at` / `saved_at` / `dismissed_at` marks. A vote is the
  same shape of per-user mark.
- **Materialized public counters**: `job-engagement-counts` already puts
  `view_count` / `applied_count` on the `jobs` row and serves them straight from
  the row on every read (no per-request counting). `jobview.Job` exposes them as
  `int32` fields. Public vote counters follow this precedent verbatim.
- **Sibling endpoints**: `POST /jobs/:slug/save` + `DELETE /jobs/:slug/save`
  behind `RequireAuthOrKey` (`keyAuth`) are the template for the vote routes.

What is missing: companies have **no** per-user interaction table (following is
expressed through saved-searches, not a row — see `company-follow`), and no
per-user detail read attaches a caller (the detail endpoints `GET /jobs/:slug`
and `GET /companies/:slug` are registered with no auth middleware).

## Goals / Non-Goals

**Goals:**

- One-tap thumbs up/down on jobs and companies for signed-in users, with toggle
  (re-tap clears) and flip (opposite replaces) semantics; at most one vote per
  (user, target).
- Public up/down counters on each job and company, drift-free and read directly
  from the target row.
- The caller's own vote surfaced on the detail pages so the control can render
  its highlighted state on load, in one round-trip.

**Non-Goals:**

- No abuse/anti-fraud beyond "authenticated, one vote per user" (no rate limits,
  no vote-weighting, no minimum-votes display threshold). Auth already caps one
  vote per account.
- No net-score / percentage / ranking. Show the two raw counts.
- No vote highlight in list/search cards (v1 renders the control only on the job
  detail page and the company page). The counters still show everywhere.
- No search-index sorting on votes (a noted seam, not built).

## Decisions

### 1. Job votes reuse `user_jobs`; company votes get a new table

Add a nullable `vote smallint` column to `user_jobs`, constrained
`CHECK (vote IN (-1, 1))` — `NULL` means "no vote". This keeps the "one row per
(user, job), marks live as columns" invariant intact and lets the vote share the
same idempotent upsert path as save/apply.

Companies have no per-user row today, so introduce:

```sql
CREATE TABLE company_votes (
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_slug text   NOT NULL REFERENCES companies(slug) ON DELETE CASCADE,
    vote         smallint NOT NULL CHECK (vote IN (-1, 1)),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, company_slug)
);
```

The asymmetry (column vs table) is deliberate: each is the idiomatic home for its
entity given what already exists, rather than forcing a symmetric polymorphic
`votes` table (Postgres has no clean polymorphic FK, and it would fragment the
existing `user_jobs` mark model).

### 2. Counters are materialized on `jobs` and `companies`, recomputed per target in the write transaction

Add `upvote_count integer NOT NULL DEFAULT 0` and `downvote_count integer NOT
NULL DEFAULT 0` to both `jobs` and `companies`. Read paths serve them straight
from the row (like `view_count`).

On every vote write, recompute **only the affected target's** two counters from
its votes *inside the same transaction*:

```sql
UPDATE jobs SET
  upvote_count   = (SELECT count(*) FROM user_jobs WHERE job_id = $1 AND vote =  1),
  downvote_count = (SELECT count(*) FROM user_jobs WHERE job_id = $1 AND vote = -1)
WHERE id = $1;
```

Recompute-per-target (not delta bookkeeping) is chosen because it is **drift-free
by construction** — the counter is always exactly the count — and cheap: it is
two indexed counts scoped to a single job/company, run once per vote. This needs
supporting indexes:

- `CREATE INDEX ON user_jobs (job_id) WHERE vote IS NOT NULL;` (partial — votes
  are a small subset of interactions)
- `CREATE INDEX ON company_votes (company_slug);`

The vote upsert + counter recompute run in one `pgx` transaction so a reader
never sees a vote without its counter, matching the spec's drift guarantee.

### 3. Endpoints mirror the save/dismiss pair

```
POST   /api/v1/jobs/:slug/vote        body {"vote":"up"|"down"}   keyAuth
DELETE /api/v1/jobs/:slug/vote                                    keyAuth
POST   /api/v1/companies/:slug/vote   body {"vote":"up"|"down"}   keyAuth
DELETE /api/v1/companies/:slug/vote                               keyAuth
```

`POST` applies toggle/flip; `DELETE` clears (no-op if absent). The direction is a
string enum decoded and validated to `±1` before any DB touch (invalid → 400).
Every response returns `{"data": {"upvote_count", "downvote_count", "my_vote"}}`
so the SPA updates optimistically from the server's authoritative counts.

### 4. `my_vote` on detail via a new `OptionalAuth` middleware

The public counters go into `jobview.Job` (`upvote_count`, `downvote_count`,
`int32`, exactly like `view_count`) and into the company response struct.

`my_vote` is **caller-scoped** and must never be persisted into the shared
search-index shape. To render the control's highlight on first load in one
round-trip, add a small `OptionalAuth(iss, keys)` middleware: it attaches the
caller id when a valid cookie/API key is present and passes through anonymously
otherwise (never rejects). Apply it to `GET /jobs/:slug` and `GET
/companies/:slug`; when a caller is present the detail handler fills `my_vote`
(`-1|0|1`), otherwise it stays `0`.

List, search, and the index-build path pass no caller, so `my_vote` is always `0`
there and is never a meaningful indexed value. v1 therefore highlights the vote
only on the two detail pages, which is where the control lives.

### 5. Domain placement

Job-vote logic extends `internal/userjob` (it already owns the `user_jobs`
mark writes). Company-vote logic goes in a new small `internal/companyvote`
package with the same upsert/clear/read surface, since companies had no per-user
domain before. The handler wires both to the mirrored routes.

## Risks / Trade-offs

- **Counter recompute cost on a hot target**: two indexed single-target counts
  per vote. With the partial/`company_slug` indexes this is negligible at current
  scale; if a target ever attracts extreme vote volume, the seam is to switch
  that path to delta maintenance. Accepted for now (simplicity over premature
  optimization).
- **`my_vote` in the `jobview` struct is always `0` outside detail**: a reader of
  the type must know it is only populated on caller-aware detail reads. Documented
  on the field. The alternative (a separate per-user votes endpoint) adds a round
  trip and surface for no v1 benefit.
- **`OptionalAuth` is a new auth primitive**: it must fail *open* to anonymous
  (never 401) and must not treat an expired/invalid token as a hard error on
  these public reads. Covered by tests: valid caller → `my_vote` set; no/expired
  token → anonymous, `my_vote = 0`, read still succeeds.
- **Backfill**: no historical votes exist, so counters start at `0`; the
  migration needs no data backfill, only the column/table/index DDL.
