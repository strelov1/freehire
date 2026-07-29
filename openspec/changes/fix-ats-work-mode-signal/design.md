## Context

`workModeFromRemote(bool)` is a narrowing helper: it accepts one bit where an ATS reports
three states. Its own doc comment warns that a false flag implies nothing, but its
signature invites any boolean, and 22 adapters pass one. For four providers that boolean
is not the best signal the API offers.

Evidence gathered by probing the live APIs before writing code:

| Provider | sampled | finding |
|---|---|---|
| ashby | 1097 postings / 7 boards | `(Hybrid, isRemote=true)` 701, `(Remote, true)` 80, `(OnSite, false)` 33, `(absent, absent)` 283 — `isRemote` is "not onsite" |
| recruitee | 1984 offers / 120 boards | `(remote,hybrid)` = (F,F) 904, (F,T) 602, (T,F) 442, (T,T) 36 |
| smartrecruiters | 2485 postings / 120 boards | `location` carries both flags: (T,F) 273, (F,T) 615, (F,F) 1597 |
| bamboohr | 999 postings / 150 boards | `isRemote` null on all 999; `locationType` `0` 438, `1` 268, `2` 293 |

## Goals / Non-Goals

**Goals:** read each ATS's authoritative work-mode field; stop emitting `remote` for hybrid
Ashby postings; keep the ingest and link-resolution paths identical.

**Non-Goals:** auditing the other 18 `workModeFromRemote` callers (workable and breezy were
checked and carry no richer field); backfilling historic rows by any path other than
re-ingest; changing the `Job.WorkMode` contract itself, which already says structured
signal only.

## Decisions

**Ashby: `workplaceType` first, `isRemote` as fallback.** `firstNonEmpty(workplaceTypeMode(…),
workModeFromRemote(…))` rather than replacing the flag outright. The 283 postings with
neither field keep today's behaviour, and a board that someday sets only `isRemote` still
resolves. Alternative — dropping `isRemote` entirely — was rejected as it removes a working
signal for no gain.

**`Remote` is derived from the resolved mode, not from the raw flag.** For Ashby that is
`mode == "remote" || isRemote(location)`: the mode decides, and the location heuristic is
preserved because it is pre-existing behaviour (the greenhouse convention, where a "Remote"
location text alone marks a job remote — it can still leave `WorkMode=onsite` with
`Remote=true` on the four sampled postings whose location reads "San Francisco or Remote").
BambooHR takes `mode == "remote"` alone: it never had the heuristic, and adding it would
widen this change for no evidenced gain.

**Recruitee/SmartRecruiters: a new `workModeFromRemoteHybrid(remote, hybrid)` helper.** Two
callers justify it, and the shared helper is where the "(false, false) is not onsite" rule
is documented once. Alternative — inlining a switch in each adapter — would duplicate that
reasoning in two places and invite a third adapter to guess `onsite`. Both flags true is a
live case, not a hypothetical (36 of 1984 Recruitee offers); remote wins there because
Recruitee renders every one of those offers as "Remote job".

**BambooHR: `locationType` `0`=onsite, `1`=remote, `2`=hybrid.** BambooHR publishes no enum
documentation, so the mapping was established by two independent signals: postings with
`locationType=1` never carry a physical address (46/46), and work-mode words in job titles
match the enum (title~remote → `1` in 10/11, title~onsite → `0` in 15/16, title~hybrid →
`2` in 1/1). Alternative — parsing the public posting page — was rejected: those pages are
client-rendered and expose nothing in HTML.

## Risks / Trade-offs

- **BambooHR enum is inferred, not documented** → two independent signals agree, and a wrong
  guess degrades to a wrong facet on one provider, not a crash. If BambooHR renumbers, the
  `default: ""` arm keeps unknown values silent.
- **Ashby hybrid jobs leave the remote filter** → intended, and it is the bug being fixed;
  the volume is large (most Ashby postings), so the change is visible in remote counts and
  in company `remote_regions` rollups.
- **Stale search results until reindex** → Meilisearch keeps the old `work_mode` until the
  index is rebuilt; the migration plan sequences it.

## Migration Plan

1. Merge; no schema change, no migration file.
2. Re-ingest the four providers — `UpsertJob` refreshes `work_mode` from the adapter, so
   crawled boards self-correct: `go run ./cmd/ingest sources/{ashby,recruitee,smartrecruiters,bamboohr}.yml`.
3. `make reindex` afterwards so search stops serving the stale facet. Never stack it with
   `reindex-companies` — Meilisearch deadlocks.
4. Rollback: revert the commit and re-ingest; the adapters are stateless.

## Open Questions

- Do the remaining 18 `workModeFromRemote` callers hide the same defect? Workable and breezy
  were cleared; the rest are unaudited and out of scope here.
- BambooHR returns an all-null `location` for `locationType=1` and puts the real office in
  `atsLocation`, which the adapter does not read. Those postings (~27%) therefore land with
  an empty location and no geography — newly visible now that they carry `remote`. Worth a
  follow-up change, not this one.
