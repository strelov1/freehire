## Context

The `huntflow` adapter (`internal/sources/huntflow.go`) crawls Huntflow-hosted career sites,
one board per `<board>.huntflow.io` subdomain, and stamps the board's configured `company` on
every vacancy. That assumption breaks for **community hubs** like AlumniHub, where each vacancy
belongs to a different partner company. The true employer is already in the vacancy's `division`
breadcrumb; we only need to read it — but solely for boards that opt in, because on an ordinary
single-company Huntflow site `division` is a department, not an employer.

This mirrors the existing `tecla` aggregator (per-job company), but differs in one structural
way: `tecla` is **boardless** (one global feed). Huntflow is **board-based** and *mixed* — most
boards are single companies and only some are hubs. So the existing boardless-`aggregator`
mechanism does not fit; the hub property must be **per board entry**, not per provider.

## Goals / Non-Goals

**Goals**
- Attribute AlumniHub vacancies to their real employer (Fluently, Mirai, Sparkland, …).
- Leave every non-hub board (Banki.ru, Flowwow, …) completely unchanged.
- No new source, schema, env var, migration, or dedup change.

**Non-Goals**
- Generalising the hub mechanism to non-huntflow providers (none need it yet).
- Making the `Partners` container folder name configurable (YAGNI — one hub today).
- Mapping salary or other new fields.

## Decisions

### Per-entry opt-in flag (`CompanyEntry.Hub bool`), not a provider marker or heuristic
- **Chosen**: add an optional `Hub bool` field (`yaml:"hub"`) to `CompanyEntry`, set only on the
  `alumnihub-career` entry. The adapter branches on it.
- **Why**: explicit, safe (untouched boards behave identically), and follows the existing
  precedent of `CompanyEntry.Region` — an optional per-entry field used by a single provider
  (Lever's EU host). Validation is unaffected (`company` stays required as the hub name +
  fallback).
- **Alternatives considered**:
  - *Pure heuristic* (always parse division when it ends in `Partners · Vacancies`): no config,
    but applies the `Partners` assumption implicitly to every huntflow board and would misfire
    if an ordinary company named a department `Partners`. Rejected — magic and unsafe.
  - *Provider-level `aggregator()` marker*: wrong granularity — huntflow is mixed, most boards
    are single companies. Rejected.
  - *String marker in config* (`hub_marker: "Partners"`): most general, but overengineered for
    the single hub we have. Deferred as the documented seam (see Risks).

### Employer = segment immediately before the `Partners` folder
- **Chosen**: split `division` on either separator (`·`/`•`), find the `Partners` segment, take
  the segment before it; fall back to the configured company otherwise.
- **Why**: robust against the real data. The naive "first segment" breaks on
  `Remote · Sparkland · Partners · Vacancies` (yields `Remote`, a sub-team). The breadcrumb is
  leaf-first and `Partners` is AlumniHub's stable container folder for all partners, so the
  segment before it is always the partner company; deeper segments are sub-teams.
- **Alternatives considered**: "second-to-last segment" — fails for the 3-segment case
  (`Mirai · Partners · Vacancies` → `Partners`). Rejected.

### Read `division` in `detail()` where the `Job` is already built
- The detail payload (already fetched per vacancy) carries `division`; extract it there
  alongside the existing fields and branch the `Company` assignment. No extra request.

## Risks / Trade-offs

- **Hard-coded `Partners` marker couples to AlumniHub's org tree** → if a future hub uses a
  differently named root folder, the parser returns the fallback (the hub name) rather than a
  wrong company — a safe degradation, not data corruption. Mitigation/seam: promote the bool
  flag to a string marker in config when a second hub appears.
- **Re-attribution churn**: existing AlumniHub rows change company on the next crawl. This is the
  intended correction; `UpsertJob` handles it (same `source`+`external_id`), and `company_slug`
  re-derives. No data loss (soft state only).
- **A partner with no sub-division but no `Partners` parent** would fall back to the hub name.
  Acceptable: better an honest "AlumniHub" than a wrong employer. Live data shows every partner
  vacancy carries the `Partners` folder, so this is the rare/own-roles case by design.

## Migration Plan

1. Ship code + `hub: true` on `alumnihub-career`.
2. Next scheduled `cmd/ingest sources/huntflow.yml` re-attributes AlumniHub vacancies to real
   employers via `UpsertJob`; a follow-up reindex (existing cron) refreshes the search facet.
3. **Rollback**: remove `hub: true` (or revert the adapter branch) — the next crawl re-stamps
   `AlumniHub`. No schema or data migration to undo.

## Open Questions

None.
