## Context

Full background and the rationale behind each call is written up already in
`docs/superpowers/specs/2026-08-03-jd-tailor-intake-design.md` (approved). This document
restates the decisions in OpenSpec's shape and adds the concrete touch points found during
research.

Today, everything downstream of "start tailoring" is keyed on a real `jobs.id`:
- `internal/handler/match_analysis.go` (`GetMatchAnalysis`/`PostMatchAnalysis`/
  `StreamMatchAnalysis`, the `job-fit-analysis` capability) each independently look up
  `GetJobBySlug`, then read/run the three-stage LLM chain and cache `matchanalysis.Analysis` for
  `(userID, job.ID)`.
- `internal/handler/cv_tailor.go` (`TailorCV`) looks up `GetJobBySlug`, then requires that cached
  analysis to already exist (409 otherwise), then bootstraps the tailored CV
  (`cvs.job_id = job.ID`).
- `internal/handler/cv_job_match.go` (`GetCVJobMatch`) is a separate, deterministic (no LLM)
  CV-vs-job text score. It is keyed by the CV's own id and its already-bound `job_id`, not by a
  fresh slug lookup, so it needs no separate `is_private` gate — an owner-scoped CV can only be
  bound to a job the same owner already reached legitimately.
- None of the above has any path that accepts raw text instead of a job.

Reusable building blocks:
- `internal/jobderive.Derive` (`jobderive.go:90`) is a pure, DB-free function — title, company,
  description, etc. in, skills/facets/category/seniority out (via `skilltag.Parse` and
  `classify.Parse`). It already runs against in-memory structs with no `jobs` row backing them
  (`cmd/backfill-derive` calls it the same way).
- `internal/linkimport` (behind `POST /api/v1/jobs/resolve`, `internal/handler/intake.go`) already
  resolves one arbitrary URL: host-scoped adapters first, `internal/atsboard` board coverage
  second, a generic JSON-LD `JobPosting` scraper (`GenericSource`) last. When a recognized
  adapter resolves it, the job is written through the canonical `pipeline.UpsertJob` path
  (enrichment enqueued, indexed normally). `linkimport` already distinguishes the generic
  fallback from a real adapter match (`GenericSource` type assertion) — that same signal is what
  this change branches on.
- `jobs` has no visibility/indexing flag today (`migrations/0001_init.sql`). `created_by` exists
  and is already used for moderator/manual jobs, but every existing job — including
  `source='manual'` and `source='weblink'` ones — is public, enriched, and indexed
  (`internal/db/jobs_moderation_integration_test.go`).

## Goals / Non-Goals

**Goals:**
- Let a signed-in user reach the existing `/tailor/[slug]` workspace starting from a URL or from
  pasted text, with zero changes to the workspace itself.
- Reuse the existing URL-resolution stack for anything a recognized ATS adapter can parse — no
  parallel importer.
- Keep a pasted/unrecognized-scrape JD fully private: invisible in search, in listings, and to
  every user except the one who submitted it.
- Keep the private path cheap: no LLM enrichment queue for a submission that exists for exactly
  one tailoring session.

**Non-Goals:**
- Deduplicating private submissions against each other or against the public catalog. Every
  paste/unrecognized-URL submission is its own row.
- A UI to browse/manage a user's own past private submissions beyond what the tailored-CV list
  already surfaces.
- Any change to `matchanalysis`'s three-stage prompt chain, `internal/cvedit`, or the tailor
  workspace's own UI/behavior.
- Rewarding a URL submission through this endpoint with `link_contributions` credits — that stays
  exclusively the `/api/v1/jobs/resolve` contribution flow's concern. This endpoint calls the
  underlying resolver/importer directly, not the contribution-reward wrapper around it.

## Decisions

**1. Reuse `linkimport`'s resolver, not its whole endpoint.**
`/api/v1/jobs/resolve` bundles resolution with contribution recording (`link_contributions`,
AI-credit rewards) — semantics that don't belong to "I'm tailoring a CV against this link." The
new endpoint calls the resolution step directly (host-scoped adapters → board coverage →
generic), and branches on whether the winning adapter was `GenericSource`:
- Not generic → hand the parsed job to the same `pipeline.UpsertJob` call the contribution flow
  uses. Public, enriched, indexed. No new code path for the write itself.
- Generic (or the fetch/parse fails) → private row (decision 3).

**2. The "recognized vs. generic" boundary decides public/private, not "has a URL".**
A recognized ATS adapter parses a trusted, structured page — exactly as trustworthy as anything
already in the catalog, so it becomes a normal catalog job. A generic JSON-LD scrape of an
arbitrary site is unverified content, the same trust tier as hand-typed text — neither belongs in
the public catalog or the search index.

**3. Private rows: new `jobs.is_private` column, synthetic `external_id`, no enrichment queue.**
- `is_private boolean not null default false`, new migration.
- `source` = `'weblink'` (URL) or a new `'pasted'` (plain text) — reusing `'weblink'` for the URL
  case keeps it consistent with how the generic resolver already tags non-adapter-matched
  imports elsewhere.
- `external_id` is a fresh synthetic value (e.g. `uuid`) per submission — deliberately outside the
  public dedup key space, so concurrent/repeat submissions from the same or different users never
  collide over `(source, external_id)` uniqueness and never contend over `created_by`-based
  access.
- `created_by` = submitting user — recorded for provenance/support purposes, not as an access
  check (see decision 4: a private job's read access is not owner-gated).
- `jobderive.Derive` runs synchronously at creation (it's pure and DB-free — no reason to queue
  it). The row is **not** enqueued onto `enrichment_outbox`: that queue exists to extract
  additional LLM-derived structure for a catalog that gets crawled repeatedly and searched by
  many users; a private, single-tailoring-session row doesn't recoup that cost. `jobderive`'s
  dict-based facets plus the raw description text are what `matchanalysis` needs.

**4. Privacy model: unguessable + never-listed, not per-request ownership checks.**
An earlier pass of this design gated every job-by-slug read (`GetJob`, the three fit-analysis
handlers, `TailorCV`, plus — found only by auditing further — `job_match.go` and
`internal/jobtracking`'s ~11 slug-resolving methods) behind a `created_by == caller` check, on
the theory that a private job should be invisible to anyone but its creator even if they somehow
learned its slug. That reasoning holds for a search engine or a listing page discovering the slug
on its own, but a fresh audit turned up **7 more** call sites with the same unguarded pattern
(`vote`, `reminder`, `similar`, `copies`, `ghost_reports`, `reports`, `community`) — a
"whack-a-mole" surface that grows every time a future feature reads a job by slug, not a fixed
list that can be closed once.

Revised model: a private job behaves exactly like a **closed job** for read access — reachable
by anyone holding its exact slug (fit analysis, tailoring, vote, copies, …, all unchanged), but
never surfaced through search, listing, or reindex. Two places must actively keep it unsurfaced,
because they don't require already knowing the slug to reach:
- Meilisearch's job-indexing query (`internal/search`) — `WHERE NOT is_private` alongside the
  existing open/closed handling.
- The DB-backed `GET /api/v1/jobs` list query and its `estimate_open_jobs()` total — same
  exclusion.
- `GET /jobs/:slug/copies` (`ListRoleClusterCopies`) is the one exception that still needs an
  explicit filter despite requiring a slug to reach *at all*: it joins on
  `company_slug`/`role_fingerprint`, so a private job can coincidentally share a cluster with an
  unrelated **public** one and surface (slug, location, url) to anyone browsing that public job's
  own copies list — reachable without ever knowing the private slug, unlike every other
  surviving read path.

The security property this relies on: `external_id` (and therefore the slug's shortcode) is a
fresh synthetic UUID per submission (decision 3) — not derived from anything guessable, and never
printed anywhere but the response the creator receives. Nothing else needs auditing or
re-auditing as new features are added, because none of them are asked to treat `is_private`
specially; they just don't independently list private rows.

**5. `/my/cvs` UI: one form, three tabs, one destination.**
The "our own vacancy" tab needs no backend work — it's a search/select combobox over the existing
job search, redirecting straight to `/tailor/[slug]`. The URL and text tabs both submit to the
new endpoint and get a `job_slug` back, then redirect to the same page. The tailor workspace does
not know or care which tab produced its slug.

## Risks / Trade-offs

- **[Risk] A generic scrape captures a broken/garbage page (paywall, JS-rendered shell, wrong
  JobPosting block) and produces a low-quality private job.** → Mitigation: this is no worse than
  what the generic resolver already does for the public contribution flow today; the difference
  is scope (private, one user) not quality. If the scrape yields no usable title/description at
  all, the endpoint responds 422 rather than creating an empty row.
- **[Risk] Skipping `enrichment_outbox` for private rows means `matchanalysis` runs against
  dict-derived facets only, with no LLM-enriched structure (e.g. parsed requirements list).** →
  Mitigation: a freshly-crawled public job is in the identical position immediately after ingest
  (enrichment is async and can lag); `matchanalysis` already has to tolerate an unenriched job, so
  this isn't a new failure mode, only a permanent instance of an existing one.
- **[Risk] A private job's slug leaks outside the direct-link channel it's meant to travel
  through** (e.g. pasted into a public Slack channel, logged by a proxy, or surfaced by some
  future feature that lists jobs by slug without excluding `is_private`).** → Mitigation: this is
  the accepted trade-off of the "unguessable + never-listed" model (decision 4) rather than
  something structurally prevented. The synthetic UUID-derived slug is not brute-forceable, and
  the two listing paths (search index, DB-backed `/jobs`) plus the one coincidental-overlap path
  (`/jobs/:slug/copies`) are the only ones that could surface it without the slug already being
  known — every other future job-by-slug consumer inherits safety for free rather than needing
  its own check, which is the whole reason this model was chosen over per-consumer gating.

## Migration Plan

- Add-only migration: `ALTER TABLE jobs ADD COLUMN is_private boolean NOT NULL DEFAULT false`.
  Default `false` means every existing row is unaffected; no backfill needed.
- No rollback complexity: the column is additive and unindexed by anything until this change's
  queries reference it.

## Open Questions

None outstanding. Four clarifying rounds resolved the open branching points: provider-recognized
→ public ingest; pasted/generic-scrape → private row with a visibility flag; single new form on
`/my/cvs`; and, after implementation surfaced a growing list of unguarded job-by-slug consumers,
a fourth round settled the privacy model itself on "unguessable + never-listed" (decision 4)
rather than per-consumer ownership gating.
