## Context

freehire ingests jobs through single-responsibility `Source` adapters registered in
`sources.All`, each a read-only reader over a public feed. A spike confirmed CleverStaff
exposes every tenant's open vacancies at
`GET https://cleverstaff.net/hr/public/getAllOpenVacancy?alias=<tenant>` as a keyless JSON
document `{"status":"ok","orgId":…,"objects":[…]}`. Each object carries a stable 32-char hex
`vacancyId`, a short `localId` used for the public URL, `position`, full HTML `descr`,
`employmentType`, `workCondition`, `status`, `currency` (no salary amounts), `dc`/`dm` epoch-ms
timestamps, `clientName` (the real employer), and `industry`. There is no anti-bot wall from a
residential IP.

Two facts shape the design. First, CleverStaff is a genuine **ATS** (a company's own hiring
pipeline), not an aggregator that mirrors other boards — so its postings are first-party and
must not be reindex-suppressed. Second, some tenants are **staffing agencies** whose single
`alias` posts for many client companies; the real employer is the per-vacancy `clientName`,
not the tenant. The codebase already models exactly this with the `CompanyEntry.Hub` marker
(huntflow's `AlumniHub`), so no new mechanism is needed.

## Goals / Non-Goals

**Goals:**
- A `cleverstaff` adapter mapping one tenant's JSON `objects` to normalized `Job`s, keyed by
  the tenant `alias` as `board`.
- Correct employer attribution: honor `Hub` so an agency tenant's vacancies carry their
  `clientName` employer.
- Keep the adapter a pure HTTP reader, unit-testable against a saved JSON fixture.
- A pre-wired proxy egress path so a prod-IP block is an ops toggle, not a code change.

**Non-Goals:**
- No salary extraction (the feed carries only a currency, never amounts) — enrichment owns it.
- No geo parsing beyond the structured `workCondition` hint — the geography dictionary derives
  country/region downstream as for every source.
- No seniority/category mapping from CleverStaff's unverified `role`/`experience` enums — left
  empty so the title dictionaries decide (see the mapping decision).
- No automated tenant discovery inside the adapter; harvesting aliases is a separate, manual
  step that seeds the board file.
- No per-vacancy detail hydration (the list already carries the full `descr`).

## Decisions

### Decision: Per-tenant ATS keyed by `board = alias`, not boardless/aggregator

Each `getAllOpenVacancy` call is scoped to one tenant `alias`, so the natural board id is that
alias, exactly like greenhouse/lever. The adapter therefore takes no `boardless`/`aggregator`
marker: it requires a board, stays in the source facet, and its postings are first-party.

- **Why not aggregator (like djinni):** the `aggregator` marker enrolls a source in the reindex
  ATS-suppression pass, which hides a posting when a first-party ATS twin exists. CleverStaff
  *is* the first-party ATS; suppressing it would be backwards. A CleverStaff posting should win,
  not be suppressed.

### Decision: Honor `Hub` for agency tenants; ordinary tenants keep configured company

Mirror huntflow: when `e.Hub` is set, each `Job.Company` is the object's `clientName`
(falling back to the configured company when `clientName` is blank); when it is not set, every
job keeps the configured company. The board file flags agency tenants with `hub: true`.

- **Why over always using `clientName`:** for a company self-hosting its own alias, the
  curated `company:` name in the board file is the authoritative, deduplicated brand; blindly
  taking `clientName` risks a slightly different string fragmenting the company facet. Keeping
  it opt-in puts the curator in control and reuses the exact semantics already specced for
  huntflow, so there is no new concept to learn or test twice.
- **Alternative considered — a `clientName`-always mapping:** simpler in code but removes the
  curator's control over brand naming and diverges from the established `Hub` contract.

### Decision: Map only cleanly-structured facets; leave the rest to dictionaries

Per the `Job` struct contract, an adapter sets a structured facet only when the platform states
it unambiguously. CleverStaff gives two clean signals:
- `workCondition` → `WorkMode` via the existing `workplaceTypeMode` helper (remote/hybrid/onsite).
- `employmentType` → `EmploymentType` via a small explicit map (`fullEmployment`→`full_time`,
  `partEmployment`→`part_time`, `contract`/`freelance`→`contract`, `internship`→`internship`),
  emitting only recognized values.

`role` ("Senior") and `experience` ("e5_5years") are left unmapped: their value sets were not
verified in the spike and a wrong guess would mis-tag seniority. The classify/skilltag
dictionaries derive those from the title as they do for every source. This is a noted seam — a
future change can add a verified `role`→seniority map.

### Decision: Single JSON request per board; empty/error payload is a board failure

Unlike djinni there is no pagination — one request returns all open vacancies. A non-`ok`
`status` or a transport error surfaces as a board failure so `board_health` cools it, rather
than silently ingesting nothing. Objects missing a `vacancyId`, `position`, or `localId` are
dropped individually (one bad object never aborts the board). `status` other than open (e.g.
not `inwork`) is filtered out; a vacancy that leaves the feed is closed by the standard unseen
sweep.

### Decision: Pre-enroll in `proxiedProviders`

The spike ran from a residential IP. freehire's prod egresses from a datacenter IP that several
ATS edges block (djinni, eightfold). Adding `cleverstaff` to `proxiedProviders` now means that
if the prod IP is blocked, setting `SOURCES_PROXY_URL` routes only this provider through the
proxy — no code change under incident pressure. It is a no-op while the proxy is unset.

## Risks / Trade-offs

- **Prod datacenter IP may be blocked.** → Pre-wired proxy egress; keep the ingest timer
  disabled until a live prod smoke confirms the IP is served (or `SOURCES_PROXY_URL` is set),
  exactly as djinni rolled out.
- **One tenant = one board, single request.** A failing tenant cools only itself; there is no
  cross-tenant blast radius. Acceptable and consistent with every per-tenant ATS.
- **JSON shape drift** (CleverStaff renames a field). → A fixture-based unit test pins the
  current shape; a parse/`status` failure surfaces as a loud board failure, not silent garbage.
- **Currency-only, geo-less feed.** → Matches the data CleverStaff exposes; salary and geo are
  enrichment/dictionary responsibilities for every source.
- **Tenant list goes stale** (a tenant leaves CleverStaff). → The board's crawl fails or
  empties; `board_health` cools it and the unseen sweep ages out its rows. The board file is
  edited on the next harvest.

## Migration Plan

- Pure addition. Deploy the binary, then add a `cmd/ingest sources/cleverstaff.yml` cron
  schedule in freehire-ops. Run a live smoke crawl from prod first; if the IP is blocked, set
  `SOURCES_PROXY_URL` (or hold the timer) before enabling. No DB migration, no API change.
- **Rollback:** remove the cron schedule and the `sources.All` registration; existing rows age
  out via the standard unseen sweep. No data migration to undo.

## Open Questions

- None blocking. A verified `role`/`experience` → seniority/experience-years map and automated
  alias discovery are deferred until there is a concrete need.
