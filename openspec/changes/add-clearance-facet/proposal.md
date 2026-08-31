## Why

A user asked: *"Any way of filtering jobs to exclude the ones that need SC
clearance"*. On the order of **15,000 catalogue postings** state a government
security-clearance requirement — SC/DV in the UK, Secret/TS-SCI in the US,
NV1/Baseline in Australia. A candidate without the right nationality or residency
history cannot be hired into any of them, yet today nothing in the wire shape says
so, so those postings sit in the listing indistinguishable from the ones they can
actually apply to.

That figure is an estimate, and the method matters because the obvious method
lies. Meilisearch's quoted-phrase syntax is **not** a phrase search on this index —
`"public trust"` and `"trust public"` both return 5,077 hits, so a `totalHits`
count answers "documents containing all these words somewhere", not "documents
containing this phrase". The estimate above instead comes from pulling a 923-row
sample spread across ten offsets of the single-token `clearance` query (38,105
hits) and matching a draft phrase list locally: 33% matched an anchor outright and
another ~7% carried a clearance requirement the draft list missed. Sizing this
facet from `totalHits` would have claimed ~37,000 — more than double the truth.

The signal is already half-extracted and thrown away: `location.eligibility`
matches `"secret clearance"` and `"ts/sci"` but consumes them purely as a
*geography* hint (US-restricted), never as a requirement a searcher can filter on.

## What Changes

- New deterministic facet `requires_clearance` — tri-state (`true` = the posting
  states a clearance requirement, `NULL` = it says nothing). Derived dict-only
  from the description, never guessed and never asked of an LLM.
- New `jobs.requires_clearance boolean` column, populated on every write path
  through the `job.New` aggregate factory, exactly as `is_tech` is.
- New anchored-phrase dictionary covering the UK (`SC cleared`, `DV clearance`,
  `security vetting`, `BPSS`), the US (`secret clearance`, `TS/SCI`, `polygraph`,
  `public trust clearance`), Australia (`NV1`, `NV2`, `baseline clearance`), and
  the scheme-neutral forms (`security clearance`, `active clearance`).
- A second rule for the **labelled-field** form the sample surfaced —
  `Clearance: Secret`, `Clearance level: Public Trust`, `Clearance Required: Yes`.
  ATS postings state the requirement as a structured field, not prose, and a
  phrase list alone misses them: they were ~7% of the sampled `clearance` rows,
  roughly a fifth of all true positives.
- Negation-aware: `"no security clearance required"` must NOT be marked as
  requiring one. This is cheap insurance rather than a measured need — a regex for
  the denial forms matched **0 of 923** sampled `clearance` rows, and the earlier
  "355 postings say no clearance is required" figure was another artefact of the
  same AND-search (`no` is a stop word, so Meilisearch discards it). The guard is
  free because `eligibility.go` already implements it; the facet just inherits it.
- New public filter `GET /api/v1/jobs?requires_clearance=false`, a new
  Meilisearch filterable attribute, and one checkbox in the web filter panel.
- Backfill of the existing catalogue, targeted rather than catalogue-wide.

No breaking changes: the facet is additive, absent from a posting means `NULL`,
and an unfiltered request behaves exactly as before.

## Capabilities

### New Capabilities

- `clearance-facet`: the `requires_clearance` signal end to end — what counts as a
  clearance requirement, how negation cancels it, how it is stored, served,
  filtered, and backfilled.

### Modified Capabilities

None. `deterministic-facets` fixes the behaviour of the *six* named dictionary
facets (`countries`, `regions`, `work_mode`, `skills`, `seniority`, `category`)
and its requirements are unchanged by adding a seventh, independent one —
`clearance-facet` states its own derivation and serving rules and follows the same
dict-only discipline.

## Impact

**Schema:** one new nullable column, `jobs.requires_clearance` (migration 0119).
Nullable and additive, so it takes no table rewrite and no backfill lock.

**Code:**
- `internal/dict/location/clearance.go` (new) — the phrase dictionary and matcher,
  reusing the negation guard already in `eligibility.go`.
- `internal/job/jobderive` — new `Derived.RequiresClearance`.
- `internal/job/job` — new aggregate field, threaded into all four persistence
  shapes.
- `internal/job/jobview` — served as `requires_clearance`, omitted when unknown.
- `internal/platform/db/queries/jobs.sql` + `make sqlc` — the column joins the
  upsert/update statements alongside `is_tech`.
- `internal/search/search` — new filterable attribute and query-param mapping.
- `cmd/backfill-derive` — the column joins the re-derivation set.

**API:** one new optional query parameter on `GET /api/v1/jobs`. Documented in
`web/static/openapi.yaml` and `web/src/lib/docs/filters.ts`.

**Web:** one checkbox in the filter panel, plus the facet model and URL encoding.

**Ops — this is the hazardous part.** A binary that requests a filterable
attribute the **live** Meilisearch index has not declared hard-500s
`/api/v1/jobs/facets` for every caller (documented at
`internal/search/search/client.go:565`). The settings patch therefore ships
**before** the binary, not with it.

**Backfill:** a full `cmd/backfill-derive` pass runs ~15 hours and is not
warranted. Meilisearch already indexes `description`, so it can name the ~38k
candidate ids (the single-token `clearance` query, plus the anchors that do not
contain the word — `ts/sci`, `polygraph`, `bpss`, `vetting`) in seconds; only
those rows get re-derived. This also sidesteps the known trap that a
`description` predicate de-TOASTs 8M rows. Over-fetching candidates is free: the
matcher decides, and a row it declines simply keeps `NULL`.
