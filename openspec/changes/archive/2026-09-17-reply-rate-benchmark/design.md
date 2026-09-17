## Context

See `proposal.md` - Why for the motivation and the production measurement that ruled out a
stage-based conversion metric in favor of the mail-observed reply signal.

Relevant existing state:

- `internal/platform/db/queries/insights.sql`'s `RebuildInsightsCompanyResponse` already computes,
  per company, `applications` (observable: the applicant has a connected Gmail/hosted/external
  mailbox) and `answered` (a non-retracted `employer_reply` event exists), storing one row per
  `company_slug` in `insights_company_response`, rebuilt by `cmd/rollup-company` in the same
  transaction as its other company rollups. The per-company figure is served only at ≥10
  observable applications (`company-hiring-signal`'s existing sample gate).
- `GET /api/v1/me/tracking/pipeline` (`application-pipeline`) already returns per-stage counts;
  Interview Rate / Offer Rate on the Pipeline tab are computed **client-side** from those counts
  (`web/src/lib/pipeline.ts`) — the backend never serves a pre-divided rate today.
- Production volume measured 2026-09-17: `insights_company_response` covers 287 observable
  applications, 97 answered (~34%), across 222 companies — comfortably above the 10-application
  gate in aggregate, even though most individual companies are below it.

## Goals / Non-Goals

**Goals:**
- Serve one global (all-companies) response rate, reusing the existing per-company rollup's
  numbers rather than a second, independent computation.
- Serve one personal response rate for the pipeline endpoint's caller, using the exact same
  observable/answered definitions, computed live (no rollup lag — a candidate connecting their
  mailbox today should not wait for tomorrow's cron to see their own number).
- Keep the "not enough data" and "no connected mailbox" cases indistinguishable at the wire level:
  both simply omit the field, matching `company-hiring-signal`'s existing convention of absence
  over zero.

**Non-Goals:**
- No segmentation (by category, seniority, source, or ATS channel) — out of scope per the
  proposal's MVP framing.
- No time-based metric (median days to reply) on the personal side — the per-company page already
  owns that, and duplicating it here is not needed for a single comparison card.
- No new page — the comparison lives in the existing Pipeline tab.
- No change to how `employer_reply` events are recorded, retracted, or linked.

## Decisions

### The global figure is a live SUM over `insights_company_response`, not a stored scalar

**Decision:** Add one sqlc query that sums `applications`/`answered` across every row of
`insights_company_response` and run it at read time, rather than computing and storing a separate
global scalar inside `cmd/rollup-company`'s rebuild transaction.

**Why:** `insights_company_response` holds one row per company that has ever had a tracked,
observable application — bounded by company count, not by application or job count, so it stays
small (222 rows today) and a full scan costs nothing worth precomputing for. Reading it live also
sidesteps an atomicity question for free: a stored scalar would need its own delete-and-reinsert
inside the same transaction as the per-company rebuild to avoid ever showing a global figure computed
from a different snapshot than the per-company ones (the exact hazard `company-hiring-signal`'s
"Rebuilt atomically" scenario already guards against for the per-company table). A live SUM has no
second copy to fall out of step with the first — it is definitionally consistent with whatever the
last completed rollup left in the table.

**Alternative considered:** A stored `insights_global_response` single-row table, rebuilt in the
same transaction as the per-company tables. Rejected: it buys nothing (no meaningful query-time
cost to avoid) at the price of one more table, one more delete-and-reinsert step, and a place the
two figures could theoretically diverge if a future edit touched one rebuild step and not the
other.

### The ten-application sample gate also does the "no connected mailbox" job, by construction

**Decision:** There is no separate boolean check for "does the caller have a connected mailbox."
The personal query defines `observable` exactly as the per-company rollup does — requiring a
connected Gmail/hosted/external mailbox — so a caller with none simply has an observable count of
zero, which the same `>= 10` gate already excludes.

**Why:** Two independent gates that must both pass are two places to get the boundary condition
right and two things a future reader has to reconcile as "why are there two checks here." Folding
"no mailbox" into "observable count is zero, which is less than ten" removes one of them without
losing any served behavior — the spec's two scenarios (`Caller below their own sample gate`,
`Caller has no connected mailbox`) describe the same code path from two different real-world
starting points, which is worth keeping as two scenarios (both are real situations a reader should
recognize) even though the implementation is one gate, not two.

### Personal counts are computed live, not cached

**Decision:** The per-user observable/answered computation runs on every `GET
/api/v1/me/tracking/pipeline` request, with no caching layer.

**Why:** Scoped to one user's rows via `application_events_user_occurred_idx` (leading `user_id`),
this is a cheap, already-indexed read — nothing like the catalogue-scale scans that
`internal/ingest/catalogstats` exists to keep off the request path. A stale personal number the
moment someone connects their mailbox or gets a reply would undercut the point of showing it
live at all.

### Response omits the comparison entirely when the global side is unavailable

**Decision:** If the global rate is not yet available (sum of observable applications across
companies is under ten), the endpoint omits the personal figure too, even if the caller
individually clears their own gate.

**Why:** A personal rate with nothing to compare it against is not a benchmark — it is a number
with no stated meaning, and the whole point of this feature (per the proposal) is the comparison,
not the personal figure in isolation. This also means the feature is inert immediately after
deploy if global volume were ever to regress below ten, rather than serving a half-formed card.

### Wire shape follows the existing raw-counts convention, not a pre-divided percentage

**Decision:** The new field carries raw counts (`you_applications`, `you_answered`,
`global_applications`, `global_answered`), matching `GetCompanyResponse`'s existing shape and the
pipeline endpoint's own precedent — Interview Rate / Offer Rate are already computed client-side
from raw stage counts (`web/src/lib/pipeline.ts`), never served pre-divided.

**Why:** Consistency with both precedents already in this codebase; also keeps the backend from
making a rounding/formatting decision (percentage precision, rounding mode) that belongs to
presentation.

## Risks / Trade-offs

- **[Risk]** `insights_company_response` could grow large enough that a live SUM stops being free
  (e.g. if per-region or per-role rows were added later, multiplying its row count).
  → **Mitigation:** none needed today: the table is one row per company, bounded by company count.
  Revisit with a stored scalar if a future change to `company-hiring-signal` changes that grain.
- **[Risk]** The global figure can cross the ten-application gate and later fall back under it if
  companies are pruned or a data-quality fix retracts a batch of `employer_reply` events, causing
  the comparison card to appear and disappear across deploys.
  → **Mitigation:** accepted — this mirrors the existing per-company behavior (a company can also
  cross the gate in either direction), and the spec already requires absence over a stale or
  estimated figure in that case.
- **[Risk]** A candidate who just connected their mailbox sees no personal figure until they
  individually clear the ten-application gate, which can take a long time at typical application
  volumes.
  → **Mitigation:** accepted for MVP — matches the proposal's explicit preference for absence over
  a noisy estimate, and is the same trade-off `company-hiring-signal` already made for individual
  companies.

## Migration Plan

No database migration. Deploy is additive and backward compatible: the new sqlc queries read
existing tables (`insights_company_response`, `application_events`, `gmail_connections`,
`mailboxes`), and the new pipeline response field is optional — existing clients that ignore
unknown fields are unaffected, and the SPA only renders the new card when the field is present.
Rollback is a plain revert; no data was written that needs unwinding.
