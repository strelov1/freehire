# Non-tech derive coverage: admin, VA, support, immigration

**Date:** 2026-09-19
**Status:** design approved, not implemented

## Why

A candidate based in the Dominican Republic wrote in asking for remote
administrative, customer support, virtual assistant, or legal/immigration work.
The catalogue can serve that request and the search cannot.

Measured on prod (2026-09-18/19, live = `closed_at IS NULL AND duplicate_of IS
NULL AND is_private = false`):

| Fact | Value |
|---|---|
| Live jobs | 4,077,985 |
| Live jobs with `category = ''` | 2,231,530 (54%) |
| Live non-tech jobs (`is_tech IS FALSE`) | 1,154,769 |
| Non-tech flagged `remote` | 77,214 (6.7%) |
| Live jobs titled `%administrative assistant%` | 64 |
| …of those flagged `remote` or `work_mode = 'remote'` | 0 |
| Global remote, candidate's categories | 3,973 (1,325 posted in last 30d) |

The last two rows are the point. The roles exist, and a candidate filtering for
remote work cannot see them.

Uncovered titles in this segment, live and `category = ''`: `admin assistant`
(319), `personal assistant` (255), `virtual assistant` (105).

### These postings are not merely unfacetted — they are not in the index

`search.CategoryUnresolved` (`internal/search/search/document.go:175`) excludes
from Meilisearch any job whose `category` is empty and whose `is_tech` is not
confidently true. That exclusion is deliberate and correct in intent: it keeps a
broad ATS crawl's undifferentiated bulk — painters, stockers, drivers — out of an
index no category filter was meant to surface. Our segment is collateral.

Measured over live postings titled `%admin assistant%`, `%virtual assistant%`,
`%administrative coordinator%`, `%administrative specialist%`, `%front desk%` or
`%immigration%`:

| Segment postings, live | 4,440 |
|---|---|
| …excluded from the index (`category = ''` and `is_tech` not true) | **3,554 (80%)** |

So an alias added here does not improve a facet on a job a candidate can already
find. It returns the job to search at all. That is the size of this change, and
it is why the category work leads and the skills work follows.

## Scope

Categories, skills, and `work_mode` — the full path a candidate walks. Country
eligibility (matching a candidate's country of residence against a posting) is a
separate, larger problem and is explicitly out of scope here.

## Approach

Narrow core, no bare alias. The dictionaries stay dict-only: a title that is not
in the table yields nothing rather than a guess.

The rejected alternatives:

- **Decompose `assistant` as a class**, with a `gradeBlindPhrases`-style mask and
  a full qualifier table over the 2.23M tail. Correct eventually, but it is a
  taxonomy project, not an answer to this segment, and every row risks pulling a
  foreign trade into `administration`.
- **Route non-tech through LLM enrichment.** Contradicts the standing decision in
  `vocab.go:145` ("back-office roles are not [enriched]") and puts a million
  postings on the LLM budget.

## Section 1 — categories

`internal/dict/classify/dictionaries.go`.

Already present in the ADMINISTRATION block (lines 1576-1584): `administrative
assistant`, `executive assistant`, `office manager`, `office assistant`,
`receptionist`, `legal secretary`, `medical secretary`, `secretary`, `data
entry`. The gap is spelling variants, not taxonomy — `vocab.NonTechCategories`
already carries `administration`, `support`, `legal`, `operations`.

Add to ADMINISTRATION:

- `admin assistant`
- `administrative coordinator`
- `administrative specialist`
- `front desk`
- `virtual assistant`

Add to LEGAL:

- `immigration paralegal`
- `immigration specialist`
- `immigration assistant`
- `immigration consultant`
- `immigration case manager`

**The bare alias `assistant` is never added.** This is what keeps the change
small: qualifier entries and `categoryNone` sentinels for `maintenance
assistant`, `assistant controller`, `assistant superintendent`, `clinic
assistant` and the rest of the trap list are unnecessary, because those titles
only misfire against a bare `assistant`. They stay uncategorised, exactly as they
are today.

### `personal assistant` is deferred, not included

755 live postings carry it with no category, and where the phrase does resolve
today it has already split across `management` (69), `operations` (29),
`administration` (23), `support` (21). In the UK it also names a social-care
worker. `classify/AGENTS.md` requires sampling a phrase against live titles
before admitting it; do that sampling and decide separately. Adding it blind
would put care work in `administration`.

## Section 2 — skills

`internal/dict/skilltag`.

Already canonical: `zendesk`, `intercom`, `salesforce`, `hubspot`, `quickbooks`,
`notion`. Add: `freshdesk`, `calendly`, `clio`, `uscis`, and the form numbers
`i-129` / `i-130`.

Every new canonical needs its slug in `dictionaries.go`, its reader-facing label
in `labels.go`, and its sentence in `descriptions.tsv` **in the same commit** — a
canonical with no description fails the build, and the ratchet that once allowed
a backlog is gone.

## Section 3 — `work_mode`

`internal/dict/location/workmode.go`.

`descriptionWorkModePhrases` holds 17 remote phrases, all in a corporate register
(`fully remote`, `100% remote`, `remote-first`). The register VA and admin
postings actually use is absent. Add: `work from home`, `work-from-home`,
`home-based`, `home based`, `telecommute`, `virtual position`.

`work from home` also appears in benefits prose ("occasional work from home"),
so it is a candidate for `travelPerkPhrases` — the same guard `work from
anywhere` already carries for the same reason (freehire#2696). Decide it with a
sample of live descriptions, not by assumption.

This source is lowest-priority in `jobderive.go:185-200`: it only ever fills a
value the structured ATS signal and the parsed location marker left empty, so it
cannot overwrite a publisher's own statement.

## Section 4 — rollout and verification

A dictionary change reaches existing rows only through `cmd/backfill-derive`
(~15h; hold `BACKFILL_CONCURRENCY` at 2-3, it has degraded prod at 6) followed by
a full `make reindex`. There is no incremental path.

Since that run is unavoidable, it also clears the debt recorded in
`classify/AGENTS.md` — the #2847 and #2849 changes of 2026-09-15, roughly 6,300
stored postings reading `is_tech = true` against what the dictionaries now say.
Delete that AGENTS.md section once the run completes.

Verification uses the same queries that produced the table above, run before and
after:

1. Count of live segment postings excluded by `search.CategoryUnresolved`
   (`category = ''` and `is_tech` not true). Baseline 3,554 of 4,440. This is the
   headline number: it counts postings returned to the index, not facets tidied.
2. Count of live postings titled `%virtual assistant%`, `%administrative
   assistant%`, `%immigration%` that carry `remote` or `work_mode = 'remote'`.
   Today the second of those is 0.
3. Spot-check that no title from the trap list (`maintenance assistant`,
   `assistant controller`, `clinic assistant`, `shipping clerk`, `surgery
   scheduler`) acquired `administration`.

Check 3 is the one that fails loudest if the bare-alias rule is broken later.

## Out of scope, worth recording

`automotive assistant &amp; service managers` — 470 live postings carry an
unescaped HTML entity in `jobs.title`, plus 91 more under a second variant. That
is an ingest defect, it corrupts any title match containing `&`, and it wants its
own issue.
