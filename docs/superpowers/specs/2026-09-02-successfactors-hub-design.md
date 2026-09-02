# SuccessFactors hub boards

**Date:** 2026-09-02
**Status:** design approved

## The problem

`jobsearch.createyourowncareer.com` is a live SAP SuccessFactors career site with 973
postings, surfaced by a link contribution (`link_contributions` row 293, a Riverty
engineering role). It is not one employer's board: it is Bertelsmann's, and the postings
belong to 40 different companies — Arvato, Arvato Systems, Riverty, RTL, Penguin Random
House, Fremantle, smartclip and so on.

The adapter cannot ingest it as it stands. `successfactors.detail` sets
`Company: e.Company`, one configured name per board, so the board would file all 973
postings under a single employer. The catalogue would gain a company with 973 jobs that no
job seeker is actually applying to, and Riverty — which today has only aggregator coverage
(adzuna 24, whatjobs 4) — would still have no board of its own.

The site offers no way out of this by configuration alone:

- There is no per-tenant sitemap. `/<tenant>/job_sitemap.xml` redirects to the site's 404
  page for every tenant tried. Only the shared `/job_sitemap.xml` exists, and it lists all
  973 postings across all 40 tenants.
- The job pages' `hiringOrganization` microdata says `Bertelsmann SE & Co. KGaA` on a
  Riverty posting — the corporate parent, not the employer — and the same markup also
  emits `hiringOrganization` twice more with the junk values `Apply now!` and a raw
  JavaScript fragment. It cannot be trusted.

What the site does encode reliably is the tenant, in the job URL's first path segment:
`/Riverty/job/Berlin-Software-Engineer-…/1425618633/`.

## The design

### 1. `CompanyEntry.Tenants`

`internal/ingest/sources.CompanyEntry` gains one optional field beside the existing
`Region` and `Hub`:

```go
Tenants map[string]string `yaml:"tenants"`
```

It maps a hub's per-posting tenant key to that employer's display name. Like `Hub`, it is
ignored by adapters that do not implement hub resolution.

`Hub` already exists and is already documented as a generic, opt-in per-entry flag —
huntflow honours it by reading the employer out of each vacancy's division breadcrumb. This
change adds a second honouring adapter, and the field it needs to do so.

### 2. Hub resolution in the successfactors adapter

When `e.Hub` is set, `detail` derives the tenant key from the job URL's first path segment
and looks it up in `e.Tenants`. An unmapped key, or a URL with no tenant segment at all,
falls back to `e.Company`.

The fallback is deliberate and matches huntflow's `companyFromDivision(division, fallback)`:
a posting under an unrecognised Bertelsmann brand is still a real posting, and filing it
under the parent is accurate rather than merely tolerable. Dropping it would lose jobs
silently; inventing a name from the slug would put a wrong employer in the catalogue, which
is the failure `internal/dict/companyname` exists to argue against.

A new helper carries the extraction:

```go
// sfTenant returns the hub tenant key from a job URL, or "" when the URL carries none.
func sfTenant(loc string) string
```

Four of the 973 sitemap entries are `/job/<slug>/<id>/` with no tenant segment at all, so
the literal `job` must not be read as a tenant. The helper returns `""` for that shape and
the caller falls back.

### 3. The board file entry

`sources/successfactors.yml` gains:

```yaml
- company: Bertelsmann
  board: jobsearch.createyourowncareer.com
  hub: true
  tenants:
    ARVATO: Arvato
    Arvato_Systems: Arvato Systems
    Riverty: Riverty
    # …
```

### Which names go in the map

Only names the platform itself states, read from each tenant's landing page at
`/<tenant>/`. That covers 23 of the 40 tenants; their titles come in four different shapes
(`Riverty Job Search`, `Job search | Career at Arvato Systems`,
`Careers at Arvato - We're on it!`, `Penguin Random House | Careers USA`), which is why the
names are curated once into YAML rather than parsed at crawl time.

The other 17 stay OUT of the map on purpose. Fourteen serve a landing page whose title and
body are both empty (JS-rendered); `BCE` and `HayHouse` serve the platform's own boilerplate
`Create Your Own Career`; and `Bertelsmann_Corporate`'s title is `Explore our Corporate
Jobs`, a page heading rather than a company. Together they account for 112 of the 973
postings, and each falls back to `Bertelsmann` — visible in the catalogue, and true. They
can be added later from a source that actually names them.

## Testing

- `sfTenant` unit tests: a normal tenant URL, the tenant-less `/job/…` shape, a bare host,
  and a URL whose path is only the tenant.
- An adapter test over a fixture sitemap + two job pages under different tenants, asserting
  the mapped employer on one, the fallback on the other, and that a non-hub board still
  takes `e.Company` unchanged.

## Risks

**Crawl cost.** 973 postings each need their own detail page fetch, making this the largest
SuccessFactors board by an order of magnitude. `fetchDetails` bounds concurrency at
`defaultDetailWorkers`, so the run is throttled rather than abusive, but it will be
materially longer than any existing successfactors board. Watch the first run's duration
before adding a second hub of this size.

**Stale sitemap entries.** At least one sampled posting (`PRH_US/…/1414466433`) redirects to
the site's exception page. `detail` already returns `ok=false` on a failed fetch and the
pipeline skips just that posting, so this needs no new handling — but it means the ingested
count will sit below the sitemap's 973.

## Out of scope

- Resolving the 17 unnamed tenants (112 postings). They fall back to the parent brand
  until a source that
  names them turns up.
- Any change to how `Hub` behaves for huntflow.
- A second SuccessFactors hub. The one board is what this is for; a second one that fits the
  same shape only needs a YAML entry.
