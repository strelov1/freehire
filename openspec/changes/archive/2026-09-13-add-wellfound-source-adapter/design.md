## Context

See `proposal.md` for motivation. Relevant existing machinery this design reuses rather than
duplicates:

- `internal/ingest/sources.Source` / the `aggregator`, `boardless`... marker interfaces
  (`registry.go`) that let an adapter declare structural facts about itself (company-per-
  posting, whether it lists a board to completion, etc.) without changing the shared
  pipeline.
- `internal/platform/firecrawl.Client` + `internal/ingest/sources/firecrawltier.go`'s
  `firecrawlProviders` map, which already rewires `bayt`/`gulftalent` onto the hosted
  scraping API without either adapter's own code knowing about it — a `build` function
  receives the hosted client and returns the same `Source`.
- `internal/dict/classify.ConfirmedNonTech`, the generic catalogue-admission gate every
  crawled adapter already goes through in `internal/ingest/pipeline`. Measured healthy for
  Wellfound (1.5% rejection on a 266-title sample) — no Wellfound-specific vocabulary needed.

Verified live (2026-09-13, this change's own spike): Wellfound's listing/search pages
(`/jobs`, `/browse/*`, `/company/<slug>/jobs`, `/remote`, and the role-taxonomy pages this
adapter actually crawls, e.g. `/role/r/software-engineer`) answer a Cloudflare JS challenge
to every address this repository can egress from, including our own proxied
headless-browser tier. The hosted Firecrawl tier passes the same challenge and returns the
real page. Each such page is server-rendered by Next.js and embeds a
`<script id="__NEXT_DATA__">` tag whose JSON body is `props.pageProps.apolloState.data` — a
normalized Apollo GraphQL cache keyed `"<Type>:<id>"`.

On a role-search page (the board type this adapter uses), job entries are typed
`JobListingSearchResult` and carry every scalar field this adapter needs directly (`title`,
`description` as full HTML, `compensation`, `remote`, `locationNames`, `atsSource`, `id`,
`slug`) — **except the hiring company, which is a reverse-only reference.** A
`JobListingSearchResult` entry carries no field naming its startup at all; the link exists
only as `StartupResult:<id>.highlightedJobListings[].__ref`, an array of refs the STARTUP
holds pointing at its jobs. Resolving a job's company therefore means building a reverse
index once per page (every `StartupResult:*` entry → the job ids it lists) before mapping
entries, not reading a field off the job itself.

(A `/company/<slug>/jobs` page uses a different type pair — `JobListing`/`Startup` — where
the job DOES carry a forward `startup.__ref`. That shape was examined during the spike for
comparison but is not what a role-taxonomy board actually is, and this adapter does not
crawl company pages; it is noted here only so the asymmetry is not rediscovered by surprise
later.)

## Goals / Non-Goals

**Goals:**
- Ingest Wellfound's technical postings into the catalogue at a healthy technical yield,
  through a transport that actually reaches the content.
- Keep the adapter's own code transport-agnostic, exactly like `bayt`/`gulftalent`, so it
  can be exercised in tests against a plain fixture and wired onto the hosted client only at
  the registry layer.

**Non-Goals:**
- Resolving a listing's `atsSource` into a second, first-party copy of the same posting via
  the actual ATS it names. Real future value (a first-party posting should suppress an
  aggregator copy, per the existing cross-source dedup pass), but a distinct piece of work
  with its own verification burden — not blocking this adapter's own value.
- A Wellfound-specific non-technical title vocabulary. The generic gate already measures
  healthy; adding one without a demonstrated gap would be exactly the kind of
  unaccountable-to-measurement mechanism `profession.hu`'s AGENTS.md entry warns against.
- Enumerating or crawling every role-taxonomy slice Wellfound exposes. This change lands the
  adapter and its parsing; which and how many role boards to add is a curator decision made
  afterward through the ordinary `cmd/add-board` flow, the same as any other provider.
- Any auto-apply capability. Wellfound's own apply flow (`directApply: true` on every
  listing observed) closes on Wellfound itself, not on an external ATS form this
  repository's `atsapply` package could drive — a materially different, much larger piece of
  work involving a candidate's own Wellfound session, explicitly out of scope here.

## Decisions

**Board identity is a role-taxonomy URL path, not a company or a keyword id.** Unlike a
per-tenant ATS board, Wellfound's crawl unit here is a *search slice* — the same shape `hh`
(professional_role) and `schoolspring` (keyword) already use for "a general-population site
where only a facet the site itself defines picks a workable technical slice." The board
field carries the role slug (e.g. `software-engineer`), and the adapter builds the full URL
from it; this keeps the catalog row human-readable and stable if the URL's own query
structure changes shape later.

*Alternative considered*: crawl `/remote` or `/jobs` unscoped and rely entirely on
`ConfirmedNonTech` to filter. Rejected — at 18.8% confirmed-technical over the unscoped
slice, roughly 8 in 10 fetched pages would be spent on postings the catalogue was never
going to keep, and Firecrawl is metered per page. Scoping to role slices up front is
free to do (the taxonomy already exists) and does not preclude adding more slices later.

**Parse `__NEXT_DATA__`'s embedded Apollo cache, not the rendered DOM.** The data is already
fully structured JSON, typed and complete (including the full HTML body) — scanning
rendered markup for it would be strictly more fragile for zero benefit, the same reasoning
`remote.com`'s adapter already applies to its own embedded RSC flight rows.

*Alternative considered*: call Wellfound's GraphQL endpoint directly with a captured
persisted-query id (`operationId`), as the initial spike did by hand. Rejected for this
adapter: it requires a POST with vendor-specific headers this repository's `firecrawlClient`
does not send (it only exposes `HTMLGetter`/`XMLGetter`-shaped GETs, mirroring the two other
firecrawl-tier adapters), and it is the more fragile surface — Wellfound can invalidate a
persisted-query hash without changing what a page renders. The embedded payload is fetched
as a side effect of asking for the page a person would see, which degrades gracefully to
"page still renders, script tag still present" against far more of Wellfound's own possible
changes.

**Not a `HydratingSource`.** The role-search page's embedded payload already carries each
posting's complete HTML description, unlike `bayt`/`gulftalent` (JSON-LD only on the detail
page). Fetching a second page per posting would multiply the Firecrawl page cost by the
average slice size for no additional information.

**Pagination trusts the payload's own stated total.** `ROOT_QUERY.talent` carries one field
per page fetched, named `seoLandingPageJobSearchResults({"page":N,"remote":true,"role":"<slug>"})`
— Apollo's normalized-cache convention of encoding call arguments into the field key, one
level under `talent` rather than directly on `ROOT_QUERY` — whose value states `pageCount`,
`totalJobCount`, `totalStartupCount` and the page's own `startups` list. Verified live on
`/role/r/software-engineer`: page 1 states `pageCount: 46, totalJobCount: 1903`; page 2
(`?page=2`, confirming the same query parameter this repo's other paginated adapters use)
states `pageCount: 46, totalJobCount: 1905` — the count moves slightly between two live
requests, as a real-time marketplace listing would, but the page count agrees. The adapter
reads `pageCount` from page 1's response and pages through it, rather than treating an empty
`startups` list as the only proof pagination has ended — following Dayforce's/UKG Ready's
precedent (an exact vendor-stated count is trustworthy) rather than SEEK's (`totalCount` is a
function of page size and cannot be trusted). Locating the field by prefix match on its name
(rather than an exact key) is required because the JSON-encoded arguments are part of the
key itself.

**Registered as `aggregator`, not `boardless`.** Each posting names its own hiring startup
(via the reverse index above), matching `bayt`/`gulftalent`'s shape exactly (a board selects
a *slice* of a shared catalogue, not one tenant's postings) — the adapter reads the company
from the page's own data, never from `CompanyEntry.Company`.

## Risks / Trade-offs

**[Cross-role-slice duplication]** The same posting can appear under more than one role
slice (a role that is both "Backend Engineer" and "Full-Stack Engineer" facing, say) →
Mitigation: accepted, not fixed here — the existing duplicate-marker/company-scoped-sweep
machinery already collapses exact-posting duplicates the same way it does for
`schoolspring`'s overlapping keyword boards. Not a correctness bug, only a minor efficiency
one (a small number of postings fetched more than once across boards).

**[Firecrawl cost scales with how many role slices get added]** Every page costs a credit
regardless of yield → Mitigation: `FIRECRAWL_MAX_PAGES_PER_RUN` already bounds a run's total
spend across every hosted-tier provider; onboarding slices is a curator decision made one at
a time via `cmd/add-board`, the same throttle every other provider's board list already goes
through, so nothing about this change removes anyone's chance to watch spend before it grows.

**[The embedded-payload shape is unversioned, vendor-controlled markup]** Wellfound could
change its Next.js build, the Apollo type names, or drop server-side rendering entirely, and
nothing announces it in advance → Mitigation: the "unparseable page fails loudly" requirement
(see spec) turns that into a visible per-board crawl failure rather than a silent empty
result, the same posture `gulftalent`'s sitemap-index-failure requirement already takes for
its own vendor-controlled surface.

**[Reverse-index resolution needed a deterministic tie-break]** Found in code review:
`wellfoundCompanyIndex` originally ranged the Apollo cache's `StartupResult:*` entries directly
(Go map iteration order is randomized per range), so if a job were ever referenced by more than
one startup — not expected in real Wellfound data, but not something the JSON schema itself
forbids — its resolved company could flip between runs of the identical input, churning
`content_hash` and misattributing the employer at random → Mitigation: `StartupResult` keys are
now visited in sorted order with first-mapping-wins, so resolution is stable regardless of
whether the conflict is ever real; `TestParseWellfoundPage_CompanyResolutionIsDeterministic`
pins it by asserting a deliberately-conflicting fixture resolves identically across repeated
parses.

**[`atsSource` correlation is left on the table]** A posting whose `atsSource` names a real
ATS could in principle already exist in the catalogue as a first-party posting from that ATS
→ Mitigation: explicitly deferred (see Non-Goals); the existing cross-source dedup pass is
title/fuzzy-based and will likely catch a meaningful share of the true duplicates without
adapter-specific work, and the remainder is a bounded, separately-measurable follow-up.
