# Company identity

How the catalogue decides that two postings name the same employer.

## Scope

The company slug (`jobs.company_slug`, `companies.slug`) and the registry that keeps two
spellings of one employer from becoming two companies. Not company *data* — see
[company-facets](company-facets.md), [internal/companyname](../../internal/companyname/AGENTS.md)
for display names, and [internal/collections](../../internal/collections/AGENTS.md) for tags.

## Always true

- **`normalize.CompanySlug` is the company key, and `normalize.Slug` is not.** Slug is faithful
  to the name it is given, which is what a URL path segment and a job's `public_slug` need.
  CompanySlug answers "which employer is this", where "RingCentral" and "RingCentral, Inc."
  must not be two answers. Reach for CompanySlug whenever the value keys a COMPANY.
- **There is exactly one legal-form vocabulary** (`internal/normalize/company.go`), and a test
  walks the module to keep it that way. There used to be four —
  `normalize`, `collections/register.go`, `cmd/harvest-ats` and `internal/mailmatch` — and they
  disagreed on substance in both directions: one stripped `gmbh` and `co` repeatedly, another
  stripped one form from fifteen tokens and refused `co` outright. Every resulting failure was
  silent, because a slug that matches nothing looks exactly like a company nobody has.
- **A token earns its place in the data.** The question is never whether a word looks dangerous
  but whether stripping it lands on a DIFFERENT existing employer. `co` is in the list on that
  evidence (all 297 catalogue companies ending in it are `& Co.` forms); `spa` is out on the
  same evidence (`Hilton Luxor Resort & Spa` outweighs every real S.p.A.). `group` is out
  because it is not a form at all — collapsing `bosch-group` into `bosch` is a judgement, and
  judgements belong to the merge worker, which shows a dry run and records a reversible alias.
- **`company_slug_aliases` is the one company-adjacent table that is NOT derived from `jobs`.**
  Everything else here is rebuilt by `SyncCompaniesFromJobs`, and `DeleteOrphanCompanies`
  removes any `companies` row no job references. A canonical decision recorded there would
  vanish the day an employer's last posting closed, and the next posting spelled the other way
  would open a fresh company — the duplicate restored on a timer. For the same reason there is
  deliberately **no foreign key** from `canonical_slug` to `companies(slug)`: the intuitive
  constraint would delete exactly what the table exists to keep.
- **The registry is read from both ends.** Ingest asks by `folded_key` (hyphens removed) so a
  spelling nobody ever merged still reaches the canon its folded form owns; `GET
  /companies/:slug` asks by `alias_slug` to serve a 301 — and only AFTER the company read
  misses, so a company that came back is never shadowed by the row that once retired it.
- **The canon freezes at first merge.** `InsertCompanySlugAlias` is `ON CONFLICT DO NOTHING`
  and the worker holds elected slugs out of a later election. A re-election would move a URL
  that has already been redirecting and indexing.
- **One resolved map, two consumers.** `pipeline.Runner` resolves the registry once per board
  run, and that map feeds BOTH the aggregator-coverage gate and the upsert. This is a spec
  requirement, not a convenience: the two used to agree only because both applied the same pure
  function, the coverage gate has already leaked once over slug spelling, and a gate that has
  silently stopped matching is indistinguishable from a board with nothing to suppress.
- **`jobderive` stays pure** — no context, no database, asserted on its signature by reflection.
  Every write path shares it, which is what stops the deterministic facets diverging between
  ingest, moderator authoring and Telegram. That is why class 1b resolves in the pipeline.

## How it works

Two duplicate classes, measured on prod 2026-08-17 across the 152,916 companies with an open
job (1,380,940 open jobs):

| Class | Example | Groups | Companies | Open jobs |
|---|---|---|---|---|
| Trailing legal form | `ringcentral` / `ringcentral-inc` | 4,149 | 8,462 | 229,004 |
| Squashed spelling | `dollar-tree` / `dollartree` | 1,368 | 2,744 | 121,836 |

The first is a pure rule and the write path applies it unaided, so it stops happening the
moment the code ships. The second cannot be: `jpmorganchase` and `jp-morgan-chase` are both
honest output of that rule — one source wrote "JPMorganChase", another "JP Morgan Chase" — so
collapsing them means electing a winner, and the winner has to be remembered.

`cmd/merge-companies` groups companies by `normalize.CompanyKey` of the NAME (which is what
covers both classes in one pass), elects the highest `job_count` variant, and records the rest
as aliases. It reports by default; `--apply` writes and `--min-jobs` bounds a wave so the plan
stays short enough to read. Jobs move in chunks whose statement selects rows still carrying the
retired slug, so an updated row leaves the set — the loop ends on its own, a re-run moves
nothing, and an interrupted wave resumes. The alias row is written BEFORE its jobs move: a run
killed between the two then leaves a slug that redirects to a company whose count is short,
where the other order leaves a slug with no jobs and no record of where they went.

A wave ends by reconciling the derived catalogue — `SyncCompaniesFromJobs` then
`DeleteOrphanCompanies`, the same pair `cmd/backfill-company-names` runs after its own re-key.
Skipping it looks harmless and is not: until the orphan row is gone, `GET /companies/<retired>`
still FINDS a company, one with no jobs left, so it answers 200 and never falls through to the
alias lookup. The merge reads as done while the redirect it exists to serve is not running.

The re-key moves CLOSED postings too, so the row count it writes is larger than the open-job
figure the plan reports — about 3x on prod. That is deliberate: a closed posting left on the
retired slug would resurrect the duplicate the day it reopens.

**The worker does not touch the search index, deliberately.** A push to the facet index costs
90-180s regardless of batch size, so feeding a wave through `search_outbox` would be tens of
hours of pushes — the shape behind the 2026-08-05 outage. The scheduled `freehire-reindexw`
picks the re-key up; until it does, a merged company under-counts its jobs for a few hours and
nothing 404s. Do not run a manual reindex, and do not set `REINDEX_DEDUP`.

`jobs.company` is left alone by a merge. The source keeps sending "DollarTree", so the next
crawl would put it back, and the display name comes from `companies.name` regardless.

## Gotchas

- **The canonical slug must be a fixed point of the rule, and job count alone does not give
  you one.** The first prod dry run elected `danaher-corporation` over `danaher` (714 open
  jobs) purely because it was larger — making the canonical url the one carrying a corporate
  form, and 301ing the better-known slug into it. It is unstable too: every new posting derives
  the stripped slug, so the canon would depend forever on an alias row to reach itself. The
  election prefers the biggest slug `CompanySlug` can reproduce, and only then the biggest.
- **Electing by anything but job count elects backwards.** "Prefer the more readable slug" is
  the tempting rule and it picks `domino-s` (1 job) over `dominos` (14,396) and `al-fa-bank`
  (20) over `alfa-bank` (1,617). Hyphens mark the corrupted spelling about as often as the
  correct one.
- **The word break is not whitespace.** It is every rune `Slug` drops EXCEPT `.` and `/`, which
  live inside the forms themselves (`B.V.`, `A/S`). Whitespace alone loses
  `Sun Technologies,Inc.`, and 13,730 catalogue companies are written that way.
- **`collections.significantFields` is NOT the same tokenization** and must not be folded into
  it. `RequireCountry` counts how specific a register name is, and `T-Mobile Inc` has to count
  as ONE significant token or a single-token name skips its headquarters check.
- **A fixture without a corporate form proves nothing.** A test for this rule whose company is
  "DollarTree" passes whether the code is right or wrong, because `Slug` and `CompanySlug`
  agree on it. That is how the coverage-gate leak was reintroduced and shipped past two green
  tests during this very change.
