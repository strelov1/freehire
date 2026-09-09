## Context

See `proposal.md` for motivation and the spike numbers. Relevant existing state:

- `openspec/specs/company-info/spec.md` already documents two run-once,
  slug-matched backfills (YC directory, an undisclosed external dump) with
  fill-gap, never-overwrite semantics on `companies.tagline` / `company_info`
  (JSONB) / `company_info_at`.
- Unlike those two, the population this worker targets — companies discovered
  purely through ATS crawling with no curated-dataset match — keeps growing
  as new boards are crawled. A strictly one-off run leaves every
  subsequently-discovered company permanently unchecked, so this worker needs
  to be safely re-runnable on a schedule, not just once.
- The spike matched by company **name** against Wikipedia, not by slug (our
  slugs are derived from crawled company names and have no relationship to
  Wikipedia/Wikidata identifiers).

## Goals / Non-Goals

**Goals:**
- Fill `tagline` (and a `company_info.summary` key) for companies with none,
  using only Wikipedia/Wikidata's free public APIs.
- Reject a match with high confidence when the resolved entity is not a
  company/organization, using the entity's own type data rather than
  guessing from description wording.
- Make repeated runs cheap: never re-query Wikidata for a company already
  resolved (matched or confidently rejected) by a prior run.

**Non-Goals:**
- Resolving company-identity duplicates (e.g. `sberbank` vs. `пао-сбербанк`
  as separate catalog rows) — out of scope, tracked by the existing
  company-identity merge tooling (`docs/agents/company-identity.md`).
- Backfilling `industries` / `year_founded` / `employee_count` / `hq_country`
  from Wikidata — deferred; those need dictionary-normalization work of their
  own once the matching gate here is proven in production.
- Any UI for reviewing or overriding a match.
- Capturing a company's own first-party "about us" text from ATS platforms
  that expose one (Greenhouse's board `content` field, etc.) — that is a
  separate, ingest-time change (`ingest-native-company-description`) with
  higher precision than any name-matched external source; where both
  eventually exist, first-party ATS text should be preferred, but this
  change does not depend on it landing first, since either can fill a gap
  the other left.

## Decisions

**1. Match via Wikidata directly, not via Wikipedia's search+summary API used
in the spike.**

The spike used `en.wikipedia.org`'s search and summary REST endpoints because
they were the fastest way to eyeball match quality. Production matching goes
through Wikidata's `wbsearchentities` action instead: it returns candidate
QIDs plus multi-language labels/aliases (useful for non-English company names
like `ПАО Сбербанк`, which the spike's English-only Wikipedia search still
happened to resolve correctly via redirects, but Wikidata search does this on
purpose rather than by luck), and a QID is what the type-confidence check
below needs anyway.

**2. Type-confidence gate: a SPARQL `ASK` over `wdt:P31/wdt:P279*`, not a
keyword scan or a flat `P31` allow-list.**

The spec requires rejecting non-organization matches without relying on
description keywords — the spike's keyword heuristic both missed real
companies (`CACI` → "American defense contractor", `rosendin` → "American
electrical contractor" — neither word is a generic company keyword) and
would have accepted some wrong ones had the keyword list been looser.

A flat allow-list checked only against direct `P31` values has the same
recall problem: "defense contractor" and "electrical contractor" are
Wikidata subclasses of `company`/`business`, not direct instances of them.
Walking the class hierarchy fixes this: one SPARQL `ASK` per candidate,

```
ASK { wd:Q<id> wdt:P31/wdt:P279* ?type . VALUES ?type { wd:Q4830453 wd:Q43229 wd:Q6881511 wd:Q783794 wd:Q891723 wd:Q328664 ... } }
```

against `query.wikidata.org`, with the anchor QID set curated and reviewed
against the spike's confirmed-good sample before rollout (`Q4830453`
business, `Q43229` organization, `Q6881511` enterprise, `Q783794` company,
`Q891723` public company, `Q328664` corporation, plus a handful of common
subtypes worth anchoring directly for query-cost reasons: bank, mining
company, airline). One extra HTTP round-trip per candidate is acceptable —
this is a slow, periodic backfill, not a request-path lookup.

*Alternative considered:* keep the spike's keyword heuristic. Rejected — the
spec explicitly rules it out, and the spike already showed concrete
false-negative cases it would keep producing.

**3. Tagline vs. `company_info.summary`: two different lengths of text.**

Wikidata's own `description` value is the short, tagline-shaped string the
spike printed (e.g. "Uranium company based in Western Australia") — store
that in `tagline`. The fuller Wikipedia lead paragraph (fetched once more via
the `enwiki` sitelink's summary REST endpoint, only for an already-accepted
match) goes into `company_info.summary`, matching the JSONB's documented role
as "lower-coverage extras."

**4. New checkpoint column: `companies.company_info_wikipedia_checked_at`.**

Because this worker's target population grows continuously (new
ATS-discovered companies) rather than being a fixed historical backlog, it
needs to run repeatedly without re-hitting Wikidata for companies it already
resolved (matched or rejected) — the two existing backfills never needed
this because they process a closed dataset exactly once. This mirrors the
same class of problem `jobs.hydrated_at` (migration 0144) already solved for
"has this record been through this specific check" — the migration adds one
nullable `timestamptz` column, set by this worker only, read only by this
worker's own eligibility query (`tagline IS NULL AND
company_info_wikipedia_checked_at IS NULL`).

*Alternative considered:* store a "checked, no match" marker inside the
`company_info` JSONB instead of a new column. Rejected — that column is
merged key-wise across multiple writers per the existing spec, and mixing a
worker's own bookkeeping into a field other sources also merge into invites
exactly the kind of cross-source confusion the existing gap-fill rule is
designed to prevent.

**5. Command shape follows the existing curated-backfill convention.**

`cmd/backfill-company-info-wikipedia`: reports matches by default, `--apply`
writes (same convention as `merge-companies` and `close-chronic-boards`) so
the first production run can be reviewed against the spike's known-good
sample before trusting it unattended. `WIKIPEDIA_BACKFILL_MAX_PER_RUN` bounds
one run (same shape as every other one-off worker's per-run cap). Needs only
`DATABASE_URL` plus outbound HTTPS to `www.wikidata.org` /
`query.wikidata.org` / `en.wikipedia.org` — no API key, no billing.

## Risks / Trade-offs

- **[Risk]** The curated business-type QID anchor set is incomplete at
  launch, rejecting some real companies. → **[Mitigation]** This is the same
  shape as the existing hand-curated `CompanyTypeHints` map: a rejected
  company simply keeps `tagline = NULL`, exactly like a declined row in
  `backfill-clearance` — no corruption, just an unfilled gap the anchor set
  can be widened to close later without re-architecting.

- **[Risk]** Two catalog rows are the same real-world employer under
  different slugs (e.g. Latin vs. Cyrillic name) and both independently
  match the same Wikidata entity. → **[Mitigation]** Each gets a correct,
  independent `tagline`; this is a pre-existing company-identity dedup gap
  (see `docs/agents/company-identity.md`), not something this worker should
  try to solve.

- **[Risk]** A generic/short company name resolves to an unrelated but
  *also*-organization-typed Wikidata entity (type check passes, but it is
  the wrong organization). → **[Mitigation]** Not observed in the spike
  sample, but the checkpoint column makes this cheap to correct later: an
  operator can clear `company_info_wikipedia_checked_at` for a reviewed-bad
  slug and re-run. A name-similarity threshold between the stored company
  name and the matched label is a plausible follow-up if a larger review
  sample shows this happening in practice, but isn't needed to ship.

- **[Risk]** `query.wikidata.org`'s public SPARQL endpoint enforces stricter
  rate limits than the REST APIs used in the spike, and can be slow. →
  **[Mitigation]** This is a slow, periodic, interruptible backfill, not a
  request-path dependency — a conservative fixed request rate with
  backoff-on-429 is sufficient, and the checkpoint column means a slow or
  partial run never repeats completed work.

## Migration Plan

1. Add the migration for `companies.company_info_wikipedia_checked_at
   timestamptz NULL`.
2. Implement `cmd/backfill-company-info-wikipedia` per the decisions above,
   dry-run (report-only) by default.
3. Run once by hand against production with a small `WIKIPEDIA_BACKFILL_MAX_PER_RUN`
   and no `--apply`; review the proposed matches against the spike's
   known-good/known-bad samples before trusting the anchor set.
4. Run with `--apply` at increasing scale (mirroring `merge-companies`'s
   `--min-jobs`-staged rollout), then schedule it periodically (e.g. monthly)
   so newly-crawled companies keep getting a chance at a match.

Rollback is not automated: writes are additive fill-gap writes into nullable
columns already shared with other sources, so reverting means clearing
`tagline` / `company_info` for the specific rows this worker touched, scoped
by `company_info_wikipedia_checked_at`, by hand.
