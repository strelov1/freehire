# Collections hub: sections, section pages, more collections

## What shipped in this change

- `/collections` is a grid of section cards (15), each listing its top five
  collections by open jobs, with a search box that filters rows in place. Every row
  is rendered (hidden past five), so all collection links stay in the server HTML.
- Each section has its own page, `/collections/group/:slug`, listing every
  collection in it. `group/` keeps section slugs out of the `/collections/:slug`
  namespace. Section pages are in the sitemap.
- A filter collection names its section with an explicit `group` field
  (`web/src/lib/collections.ts`); a company collection's section follows from its
  `kind`. `collectionSections.test.ts` fails if a collection lands in no section.
- 55 new filter collections in five new sections — Industries (`domains`), Cities
  (`cities`), Job types (`employment_type`, `relocation`), Company types
  (`company_type`), AI careers (`ai_archetype`). Each was checked against prod to
  narrow the catalogue and not be reported in `meta.ignored_params`.
- Rows draw a stack instead of a generic icon. A company collection shows its four
  leading companies (`/companies?collections=<slug>`). A field, AI-career,
  seniority, role, industry, job-type or company-type collection shows the four
  most frequent employers among its 24 latest postings at notable companies
  (`collections=` Big Tech, unicorns, Fortune 500, AI, YC, a16z…) — the head of an
  unrestricted feed is staffing agencies (DevOps: Manpower, Michael Page). A
  regional remote collection shows its four leading countries' flags. Languages,
  frameworks, infrastructure and cities keep their own logo or flag. Stacks are a
  committed snapshot (`web/src/lib/data/collectionStacks.json`), rebuilt by hand
  with `pnpm gen:collection-stacks` — who leads a collection moves over weeks.
  Count calls run at most eight at a time.

Two traps found on the way:

- **Location facets combine with OR.** `cities=Austin&countries=us` is every US
  job, not Austin's. A city collection pins `cities` alone; its flag rides in a
  display-only `flag` field.
- **`company_size` is a backend facet but not a UI one**, so the registry test
  refuses it; size-based collections were dropped rather than widening that test.

## Follow-ups (not in this change)

### 1. Investor collections

**Done 2026-10-21 for Sequoia, Accel, Greylock, USV, BOND and the a16z upgrade** (see
`internal/job/collections/AGENTS.md`). Still open: Lightspeed, Index, SoftBank Vision Fund,
Founders Fund; Benchmark publishes no portfolio.

Ten firms researched 2026-10-20; full notes in
[2026-10-20-vc-portfolio-sources.md](2026-10-20-vc-portfolio-sources.md). Each is one `backer` entry in `internal/job/collections/collections.go`;
`cmd/import-collections` needs no change. The web app needs a brand mark per firm
(`web/src/lib/backers.ts`).

| Firm | Source | Companies | Domains |
|---|---|---|---|
| Greylock | public Sanity query API | 162 | yes |
| Union Square Ventures | public Sanity query API | 262 | most |
| Accel | public Sanity query API | ~800 | ~770 |
| BOND | static JSON on bondcap.com (exclude legacy KP fund) | 85 | yes |
| Sequoia | Framer search-index JSON (file name changes per publish) | 431 | yes |
| Lightspeed | static HTML | 684 | detail pages |
| Index Ventures | static HTML | 330 | detail pages |
| SoftBank Vision Fund | `__NEXT_DATA__` | 340 | detail pages |
| Founders Fund | WordPress JSON, notable companies only | 64 | no |
| Benchmark | publishes no portfolio | — | — |

Order: the first five (one request each, with domains), then Lightspeed, Index,
SoftBank. Also move `a16z-portfolio` to `a16z.com/portfolio` (876 companies against
the 280 the talent-network API gives today); keep `a16z-speedrun` where it is.

Open decisions: drop exited/acquired companies; whether `import-collections` gets a
monthly timer (today it runs by hand, so no list ever refreshes).

### 2. Match by domain, not only by name

**Done 2026-10-21** — `sameSite` in `internal/job/collections/site.go`; YC also drops its
Inactive/Acquired entries now.

Membership is matched on `normalize.CompanySlug(name)` alone, which already
produces false members: the YC collection's leading companies on prod include
`benchmark`, `Meta`, `pennant` and `arrow`. Short common-word names in the new
portfolios (Founders Fund's "Found", "Enter", "River"; Index's "Bird") would add
more. Every good portfolio source carries a website; matching on it should land
before or with the investor collections.

### 3. Better "leading companies" for facet collections

The stack is the most frequent employer in a 24-posting sample, not the true top
by open jobs, and the snapshot only changes when someone reruns the script: `/jobs/facets` refuses `company_slug`, and `/companies` does not
filter by job facets. A backend top-N per collection would be exact.

### 4. Hub request count

The hub renders ~140 count calls per request. Each is a cheap Meilisearch query and SSR reaches the API
directly, but it grows with every collection added. If it shows in latency, one
facet-distribution call per facet could replace most of the per-collection counts.
