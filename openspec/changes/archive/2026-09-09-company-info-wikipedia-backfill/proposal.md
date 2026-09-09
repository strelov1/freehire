## Why

A large share of companies in the catalog carry no `tagline`/`company_info` at all — those attributes are only ever written by the two existing one-time, slug-matched datasets (the YC directory and an undisclosed external company-info dump), so a company discovered purely through ATS crawling that isn't in either dataset stays empty forever (e.g. `paladin-energy`, a real ASX-listed uranium producer with no tagline). A research spike against a 300-company sample found 111 (37%) missing `tagline`, and Wikipedia's public search+summary API (no key, no cost) could confidently supply a correct, well-formed tagline for 42 of those 111 (~38% of the gap) once results are filtered to actual company/organization entities. Raw top-hit matching is unsafe on its own — short or generic company names (`CWAN`, `takeaway`, `Evolution`, `Boardroom Appointments`) resolved to unrelated people, concepts, or disambiguation pages in the spike.

## What Changes

- Add a third run-once, host-invoked backfill worker (alongside the YC and company-info-dump backfills already described in `openspec/specs/company-info/spec.md`) that looks up each company missing `tagline` against Wikipedia by name, and writes `tagline`/`company_info.summary` only for a match confidently typed as a company/organization — never the first search hit.
- Match confidence gate: resolve the candidate's Wikidata entity and require its `P31` (instance of) to fall within a curated set of business/organization QIDs (company, business, corporation, public company, mining company, bank, etc.) rather than keyword-scanning the English-language description, so the gate is language-independent and matches the false-positive cases actually observed in the spike (a person, a military force, a biology article, a disambiguation page all fail a `P31` check that a keyword scan let through or wrongly excluded).
- Follows the existing fill-gap semantics already specified for company-info: never overwrites a `tagline` another source already set, and never touches job-derived facets or `job_count`.
- Idempotent and resumable like the other two backfills: a company that already has a `tagline` (from any source) is skipped, so re-running costs nothing.
- Out of scope for this change: no attempt to resolve the *origin* dataset problem for the two existing backfills, no UI changes, no attempt to backfill `industries`/`year_founded`/`employee_count`/`hq_country` from Wikidata (a possible follow-up once the matching gate is proven, since those fields need their own dictionary-normalization/parsing work).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `company-info`: adds a new run-once backfill source (Wikipedia/Wikidata, matched by name with a typed-entity confidence gate) that fills `tagline` and a `company_info.summary` key under the same gap-filling rules already specified for the YC and company-info-dump backfills.

## Impact

- New command: `cmd/backfill-company-info-wikipedia` (or similar; named precisely in `design.md`), read-only against Wikipedia/Wikidata's public APIs, writing only to `companies.tagline` / `companies.company_info` / `companies.company_info_at`.
- New package under `internal/job/` (company-info block, alongside `internal/job/ycdir`) for the Wikidata client and the business-type confidence gate, kept dependency-clean per the layering rules in `AGENTS.md`.
- One small migration: a `companies.company_info_wikipedia_checked_at` checkpoint column so a company is ever looked up against Wikipedia once (match or reject), letting the worker be scheduled periodically for newly-crawled companies without re-querying the ones already checked (see `design.md`).
- No impact on search indexing: `tagline`/`company_info` are not part of `content_hash` or any Meilisearch facet today (matching the existing two backfills' behavior), so no reindex is required after a run.
- Needs only `DATABASE_URL` plus outbound HTTPS to `en.wikipedia.org` / `www.wikidata.org`; no API key, no billing.
