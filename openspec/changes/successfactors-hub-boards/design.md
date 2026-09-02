## Context

The full measured design, including the live probes behind every claim here, is committed at
`docs/superpowers/specs/2026-09-02-successfactors-hub-design.md`. This is the decision record.

`jobsearch.createyourowncareer.com` is a SAP SuccessFactors career site with 973 postings
belonging to 40 Bertelsmann companies. The successfactors adapter's `detail` sets
`Company: e.Company` — one configured name per board — and the board is already in
`sources/successfactors.yml` under `company: Arvato`, so every one of those 40 employers'
postings is currently filed under Arvato (989 open rows against a 973-entry sitemap).

Constraints found by probing the live site:

- No per-tenant sitemap exists. `/<tenant>/job_sitemap.xml` redirects to the site's 404 page
  for every tenant tried; only the shared `/job_sitemap.xml` is served, listing all 973
  postings.
- The job pages' `hiringOrganization` microdata is unusable: on a Riverty posting it reads
  `Bertelsmann SE & Co. KGaA` (the corporate parent), and the same markup emits the property
  twice more with the values `Apply now!` and a raw JavaScript fragment.
- The tenant IS reliably encoded as the job URL's first path segment
  (`/Riverty/job/<slug>/<id>/`), with 4 of the 973 entries carrying no tenant segment at all
  (`/job/<slug>/<id>/`).
- Each tenant's landing page at `/<tenant>/` names it in `<title>` for 23 of the 40 tenants,
  in four different title shapes. The other 17 serve an empty (JS-rendered) page, the
  platform's boilerplate `Create Your Own Career`, or a page heading rather than a company.

The board-file schema already carries a generic `hub` flag, honoured by three adapters today
(huntflow, loxo, cleverstaff), each reading the employer out of its own platform's payload.

## Goals / Non-Goals

**Goals:**

- Ingest the board with each posting attributed to its real employer.
- Keep the hub concept generic: a second SuccessFactors hub should need only a YAML entry.
- Never put a wrong employer in the catalogue.

**Non-Goals:**

- Naming the 17 tenants the platform does not name (112 postings). They fall back to the
  parent brand.
- Changing how `hub` behaves for huntflow.
- Any change to schema, API or web.

## Decisions

**Employer names are curated in the board file, not resolved at crawl time.**
The alternative was fetching each tenant's landing page per run and extracting the name from
`<title>`, the way `internal/dict/companyname`'s title resolvers work. Rejected on the
measurement: it fails outright for 17 of 40 tenants and needs four different title extractors
for the rest, so it would carry the complexity of live resolution AND still need the curated
fallback. Writing 23 verified names once is the same act the repo already performs for every
`company:` in `sources/*.yml`.

**The map lives on `CompanyEntry`, not in a dict package.**
`Tenants map[string]string` sits beside `Region` and `Hub` — optional, per-entry, ignored by
adapters that do not implement it. A dict package would be infrastructure for one board.

**An unmapped tenant falls back to the configured company; it is never humanised.**
Deriving `Arvato_Systems` → "Arvato Systems" works, but the same rule turns `PRH_US`,
`BFS_Health_Finance`, `GBS` and `SDSH` into noise. `internal/dict/companyname` already argues
this: a wrong-but-plausible name reads worse than the honest fallback. The fallback matches
huntflow's `companyFromDivision(division, fallback)`.

**One board, not 41.**
Filing each tenant as its own board entry was rejected because there is no per-tenant sitemap:
41 boards would each download the same shared 223 KB sitemap every cycle.

## Risks / Trade-offs

- **The catalogue re-attributes ~570 open postings on the next crawl of this board** → the
  rows are UPDATEd in place (`UpsertJob` conflicts on `(source, external_id)` and the board,
  which namespaces external_id, does not change), so nothing is orphaned or double-written.
  The post-run unseen sweep scopes itself by the slugs the run PRODUCED
  (`distinctCompanySlugs` reads `j.Company`), not by the board file's company, so per-tenant
  closes keep working — the same mechanism huntflow's hub already relies on.
- **Stale sitemap entries** (a sampled `PRH_US` posting redirects to the site's exception
  page) → `detail` already returns `ok=false` on a failed fetch and the pipeline skips just
  that posting. The ingested count will sit below 973; that is correct, not a defect.
- **The curated map goes stale when Bertelsmann adds a brand** → the new tenant falls back to
  `Bertelsmann`. Visible in the catalogue and true, rather than silently wrong.

## Migration Plan

No schema or data migration. The board is already crawled, so the change lands on the next
scheduled run of `sources/successfactors.yml`: postings keep their rows and move to their real
employer. Rollback is restoring the entry to `company: Arvato`, which the following run undoes
the same way.

## Open Questions

None.
