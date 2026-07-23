## Why

Jobseekers have no way to signal what they think of a job posting or a company,
and visitors have no crowd signal to judge either by. A lightweight thumbs
up / thumbs down vote gives signed-in users one-tap feedback and gives everyone
a public quality signal — the same crowd-rating pattern that already works for
engagement counters (`job-engagement-counts`), extended to an explicit
sentiment vote and to companies.

## What Changes

- Signed-in users can cast a **thumbs up** or **thumbs down** on any job and any
  company. The vote is a toggle: re-casting the same direction clears it, and
  casting the opposite direction flips it. At most one vote per (user, target).
- Each job and each company carries **materialized public counters**
  (`upvote_count`, `downvote_count`) on its own row, served directly on every
  read path (list, detail, search) with no per-request counting — mirroring how
  `view_count`/`applied_count` already work.
- Job votes are stored as a new `vote` column on the existing `user_jobs`
  per-(user, job) row. Company votes get a new `company_votes`
  per-(user, company) table — the first per-user company interaction that needs
  its own row (following did not; see `company-follow`).
- New authenticated endpoints to cast and clear a vote on a job and on a company;
  public read shapes (`jobview` and the company wire shape) gain the two counters
  plus the caller's own vote (`my_vote`) when the request is authenticated.
- The SPA renders a 👍 / 👎 control on the job detail page and the company page,
  showing the aggregate counts and highlighting the caller's own vote.

## Capabilities

### New Capabilities

- `thumbs-voting`: signed-in thumbs up/down on jobs and companies; per-user vote
  storage with toggle semantics; materialized public up/down counters on the job
  and company rows; authenticated cast/clear endpoints; public counters plus
  caller `my_vote` exposed on the job and company wire shapes; SPA vote control.

### Modified Capabilities

<!-- No spec-level requirement changes to existing capabilities; the new
     counters and wire fields are additive and specified under thumbs-voting. -->

## Impact

- **Schema**: new migration — `user_jobs.vote smallint` (nullable, CHECK IN
  (-1, 1)); new `company_votes` table; `upvote_count`/`downvote_count integer`
  counters on `jobs` and `companies` (default 0). New indexes to make the
  single-target counter recompute cheap (`user_jobs(job_id)` where vote set;
  `company_votes(company_slug)`).
- **DB layer**: new sqlc queries for the vote upsert/clear + single-target
  counter recompute (one transaction), and for reading `my_vote`.
- **Domain**: extend `internal/userjob` for job votes; a small company-vote path
  (new `internal/companyvote` or a handler-level query — decided in design).
- **HTTP**: `internal/handler` — new job and company vote routes behind
  `RequireAuthOrKey`; counters + `my_vote` added to `jobview` and the company
  response shape.
- **Web**: `web/` — a shared vote control component used on the job detail page
  and the company page.
- **Search**: `internal/search` seam — the counters could later feed ranking or
  a sort key; out of scope here, noted only.
