## Why

We already answer "does this posting mention visa sponsorship?" — `enrichment.visa_sponsorship`, an LLM reading of the job text. But most postings say nothing about sponsorship at all, so the filter is silent exactly where a relocating candidate needs it most. The UK and the Netherlands both publish an authoritative register of employers licensed to sponsor work visas; those registers answer a different and stronger question — *can this employer sponsor at all?* — for every company, whether or not any posting mentions it.

The second reason is structural. Company-level signals have accreted one migration at a time (`remote_regions`, `yc_batch`/`yc_status`, `yc_stage`/`yc_flags`, `maturity`, `subindustry` — five migrations, five facets), and each new filterable Meili attribute opens a ~26-minute hard-500 window on deploy. Meanwhile `internal/collections` already *is* the reusable layer for this shape of data (external dataset → name match → tag on the company → denormalized onto jobs → one filterable facet). Landing the registers there instead of as a sixth column both ships the feature with zero migrations and makes the next register a single registry entry.

## What Changes

- **`internal/collections` grows from "editorial themes" into a general company-tag registry.** A `Kind` discriminates editorial collections (Big Tech, YC, Unicorns) from **credentials** — verifiable facts about a company sourced from an authoritative register.
- **`Dataset.Parse` returns records, not bare names.** `[]byte → []Record` where a `Record` carries a name plus source metadata (route, town, rating, KvK). The eight existing parsers wrap trivially. **BREAKING** for the package's internal API only; no wire, DB, or URL contract changes.
- **Collections gain an optional `Gate`.** A predicate over (company, record) that must hold before a tag is applied. `Gate == nil` preserves today's behaviour byte-for-byte.
- **`Dataset` gains an optional `ResolveURL`.** The GOV.UK CSV URL embeds a snapshot date and rotates monthly, so a fixed URL cannot work; UK resolves via the GOV.UK Content API with an HTML-scrape fallback.
- **Two new registry entries:** `uk-skilled-worker-sponsor` and `nl-recognised-sponsor`.
- **A deliberately conservative matching policy** — legal-suffix stripping, a geography gate, a stricter rule for single-token names, and an ambiguity guard — so that a badge is never applied on a coincidence of names.
- **`cmd/import-collections` gains `-dry-run`** so the policy can be calibrated against production before the first write.
- **The frontend registry mirror becomes generated** (`cmd/gen-contracts`) instead of hand-kept, because `kind` now drives which filter group a tag lands in — drift there is a correctness bug, not a cosmetic one.
- **UI:** the job-search facet splits into "Collections" and "Credentials"; a badge on the job card and the company page carries an explicit disclaimer; `/collections/uk-skilled-worker-sponsor` becomes a landing page for free.

Explicitly **out of scope**: migrating `yc_*`, `maturity`, or `subindustry` into the tag model. That is churn without value today; the seam is noted in the design.

## Capabilities

### New Capabilities

- `company-credential-registries`: credential collections sourced from authoritative public registers — the two register sources and their fetch strategies, the matching policy that decides when a company earns a credential, and how a credential is presented so it is never mistaken for a promise about a specific role.

### Modified Capabilities

- `job-collections`: the registry entry gains a kind and an optional gate; a dataset parser yields records rather than names and may resolve its URL dynamically; the search facet renders credentials as a distinct group; the import worker supports a non-writing dry run.

## Impact

**Code**

- `internal/collections` — `Kind`, `Record`, `Gate`, `Dataset.ResolveURL`, legal-name normalization, the two new entries, and the eight existing parsers adapted to the record shape.
- `cmd/import-collections` — record-shaped resolve, gate application, ambiguity precomputation, `-dry-run`.
- `cmd/gen-contracts` — emit the collection registry (slug, title, description, kind).
- `internal/db/queries/companies.sql` — `ListCompanyCollections` widens to `countries, hq_country`. A query edit, **not** a migration.
- `web/src/lib/facets.ts`, `web/src/lib/collections.ts`, the job card, and the company page.

**Not affected**

- No migration. `companies.collections` and `jobs.collections` already exist.
- No new filterable Meili attribute. `collections` is already filterable on both indexes, so there is no hard-500 deploy window.
- `enrichment.visa_sponsorship` is untouched and keeps its current meaning.

**Operational**

- Ordinary release, then `cmd/import-collections`, then `make reindex` (never stacked with `reindex-companies`).
- A `-dry-run` pass against production is a precondition of the first write: the single-token rule leans on `hq_country`, and if that column is sparsely populated the rule would cut exactly the single-word UK brands worth having.

**Dependencies**

- Two new upstream fetches: GOV.UK (Content API + assets host) and `ind.nl`. Both are public and unauthenticated. Both are HTML/CSV shapes that will eventually change; the existing abort-before-write is what keeps a broken parser from stripping the tag off every company.

**Provenance**

- The idea and the two public register URLs were taken from [`DaKheera47/job-ops`](https://github.com/DaKheera47/job-ops). That project is AGPLv3 + Commons Clause and freehire is MIT, so **no code was copied** — the fetch strategies, matching policy, and storage model here are our own and are built on `internal/collections`, which predates this change.
