## Context

See proposal.md - Why. All of the following was verified live on 2026-09-06 against three
tenants — `etrade.gr8people.com`, `ardene.gr8people.com`, `batesville.workgr8.com` — and is
written up for `internal/ingest/sources/AGENTS.md` as part of this change (task 3).

**Vendor and hosts.** Both `apply...gr8people.com` and `...workgr8.com` career pages serve the
identical Next.js "app-career-site" bundle (same asset host, `assets.gr8people.com`, same chunk
names, same GraphQL schema) — confirmed by minting a token and running the same query against a
tenant on each domain and getting the same shape back. This is the Factorial/FactorialHR case
(`internal/ingest/sources/factorial.go`, `internal/ingest/atsboard/board.go`'s `factorial`/
`factorialhr` rows): one vendor, two marketing domains, one adapter, one provider key.

**Minting the session.** A GET of the tenant's own `https://<host>/jobs` page is a Next.js
server-rendered page whose `__NEXT_DATA__` script tag embeds `"token":"eyJ..."` — a JWT (ES256,
`iss: auth-service`) good for 5 hours (`iat`/`exp` measured 18000s apart), scoped to that ONE
tenant: decoding two tenants' tokens shows different `org` claims (`db63c8` for etrade, `0f3be7`
for ardene, `392f45` for batesville) — the token is NOT portable across hosts, so it must be
minted fresh per board, from that board's own host. No cookie or session state is needed beyond
the bearer token itself.

**The search call.** `POST https://<host>/graphql`, `Authorization: Bearer <token>`, GraphQL
operation `searchJobs` aliasing the schema's `searchJobPostings(query, start, first, after, sort,
filters)` field, returning `results.nodes` (the `jobPostingFields` fragment), `results.pageInfo`
(Relay-style `hasNextPage`/`endCursor`) and `results.totalCount`. Confirmed live: the DEFAULT,
filterless query already answers only what a public visitor's search shows — open, external
postings — on every tenant tried (5-, 12-, 173-, and 20-posting boards), so no extra filter
clause is needed the way `jobappnetwork`'s shared ES proxy required one. Cursor pagination was
exercised end to end (page 2 continued cleanly from page 1's `endCursor`, no gaps, no overlap on
the sampled boards).

**Posting shape** (fields actually observed, `jobPostingFields` fragment; irrelevant
`@include(if: $hasXxx)`-gated custom fields omitted — those exist for a tenant's own
custom-field configuration and are out of scope, matching how other adapters ignore vendor custom
fields they cannot generically interpret):
```json
{
  "id": "Sm9iUG9zdGluZy80NzA5", "key": "4709", "number": "JR019381", "status": "OPEN",
  "title": "FSR - Chicago, IL", "descriptionHTML": "<p>...</p>",
  "workplaceType": "ON_SITE", "postType": "EXTERNAL", "postedOn": "2026-03-18T19:59:49.013Z",
  "positionType": {"id": "...", "name": "Full Time"},
  "primaryPlace": {"id": "...", "name": "Saint-Jérôme, QC, Canada"},
  "places": {"nodes": [{"id": "...", "name": "Saint-Jérôme, QC, Canada"}]},
  "payRangeLow": null, "payRangeMid": null, "payRangeHigh": null, "currencyCode": null,
  "numPositionsOpen": 1, "applyOnExternalATS": false
}
```
`workplaceType` is a closed three-value enum (`ON_SITE`/`REMOTE`/`HYBRID` — confirmed by a
validator function in the platform's own bundle), a genuine structured work-mode signal.
`places.nodes[].name` is already a formatted "City, Region[, Postal], Country" string per place
(no separate structured country code field exists on the fragment), so it is passed through as
free text the same way `apploi`/`hrmdirect` do, and `Countries` is left for the location dictionary
to resolve downstream. `positionType.name` is free-typed by the tenant but overwhelmingly
"Full Time"/"Part Time" on the sampled tenants — mapped the same low-risk way `edjoin`/`ashby`/
others map an equivalent field, everything else left unmapped.

## Goals / Non-Goals

**Goals:**
- One adapter, one provider key, covering both `gr8people.com` and `workgr8.com` tenants.
- Board = the tenant's whole host, matching the existing `modeHost` convention (Factorial,
  Teamtailor, Zoho).
- Mint a fresh token per crawl (per board), never cache one across boards (they are tenant-scoped)
  or across runs (a 5h token comfortably outlives one crawl but crawls are infrequent enough that
  caching would add complexity for no measured benefit).

**Non-Goals:**
- Custom fields (`userDefinedN`, `classificationType`, `gradeLevel`, etc.) gated behind the
  `hasXxx` GraphQL flags — tenant-specific configuration with no generic mapping, and reading them
  would require first querying each tenant's field configuration, doubling the request cost for a
  facet no other adapter in this catalogue publishes either.
- Reusing the `searchGoogleJobDiscovery` variant seen alongside `searchJobPostings` in the
  bundle — it exists for tenants the platform's own `google-job-discovery` feature flag opts into
  (`false` on every tenant sampled); crawling it unconditionally would silently return nothing on
  the common case and add a second code path for a variant not yet observed live.
- `werecruit.io`, the issue's remaining platform.

## Decisions

**One `gr8people` provider key for both domains, mirroring `factorial`.** The alternative —
`gr8people` and `workgr8` as separate providers — is exactly the mistake the issue itself calls
out and `internal/atsboard/AGENTS.md` warns against: a board this repo already crawls under one
name would look brand new under the other the day someone pastes the same tenant's link on its
other-branded domain (unlikely for one tenant to run both, but the provider-identity risk is
independent of that — it is about the CRAWL LOGIC being identical, which it verifiably is).

**Board = whole host, not the subdomain label.** A bare label ("etrade") does not say which
brand domain it resolves under, and nothing about a tenant's name predicts which of the two it
was onboarded on. `modeHost` already exists for exactly this shape (Factorial's TLD, Teamtailor's
apex) — reusing it costs nothing new in `atsboard` and keeps this board id self-describing the
way Workday's `host+site` and UKG's `host+tenant+guid` are.

**Mint the token from `/jobs`, not a lighter-weight endpoint.** The `_next/data/<buildId>/jobs.json`
route returns the same token in ~2KB versus the full page's ~85KB, but `buildId` is a per-deploy
hash embedded in the HTML itself — hardcoding it would break on gr8people's next frontend
release, and discovering it would still require fetching the HTML page first, at which point the
lighter route buys nothing. Fetching `/jobs` directly needs no such assumption: the
`"token":"eyJ..."` pattern lives in a `<script id="__NEXT_DATA__">` shape that is standard Next.js
SSR output, far more stable across frontend redeploys than one build's asset hash.

**No explicit visibility filter, unlike `jobappnetwork`.** There the shared Elasticsearch proxy
had to be told `internalOrExternal: externalOnly` because an unscoped query reads across the
WHOLE platform. Here the token itself is minted per-tenant by the platform's own auth service and
the default query already answered only public/external/open postings on every tenant sampled —
there is no equivalent unscoped path to guard against, so adding a filter clause would be
defending against a risk that was checked and does not exist for this platform.

## Risks / Trade-offs

- **[Undetermined rate limiting]** Only a handful of manual probes were sent; no adapter-scale
  volume was measured. → Ship without a pacer (the default for an unmeasured provider, matching
  jobappnetwork's own note) and watch the first real boards' `board_health` rows.
- **[Token minted per crawl, not cached]** A board with many concurrent in-flight boards during
  one `cmd/ingest` run mints one token each — cheap (a single GET) and avoids any cross-board
  state, at the cost of one extra request per board versus a hypothetical shared cache keyed by
  host. Not worth the complexity at today's expected board count.
- **[`positionType`/`workplaceType` are tenant-configurable, not platform-fixed]** `workplaceType`
  was confirmed as a closed enum by the platform's own client-side validator; `positionType.name`
  is free text a tenant could rename. Only the two overwhelmingly common values are mapped;
  anything else is left to the pipeline's own dictionaries, the same posture `edjoin`/`ashby` take
  on their own free-typed fields.
