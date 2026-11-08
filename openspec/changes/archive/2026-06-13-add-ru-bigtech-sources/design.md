## Context

The existing source adapters (greenhouse, lever, ashby, workable, recruitee,
smartrecruiters, personio, pinpoint, rippling, bamboohr, workday, huntflow) are all
**multi-tenant ATS platforms**: one adapter serves many companies, selected by a
per-company `board` slug. Russian bigtech does not use these platforms — each company
runs its own bespoke career backend on its own domain. Their listings are reachable via
public, unauthenticated JSON APIs, live-verified on 2026-06-13 with our exact ingest
client header (`User-Agent: freehire/0.1 (+https://freehire.dev)` + `Accept:
application/json`).

This document records the architectural decisions; the spec delta records the requirements.

Constraints carried from the codebase: sqlc is the only DB layer and this change adds no
schema (every adapter reuses `UpsertJob`); the shared `HTTPClient` is the single
transport; `fetchDetails` is the shared bounded list→detail fan-out; `sanitizeHTML`
(bluemonday) is the single description sanitizer.

## Goals / Non-Goals

**Goals:**

- Register 14 single-company RU bigtech adapters behind the existing `Source` interface
  with the established one-file-plus-one-registration-line ergonomics.
- Reshape the board model so single-company adapters are a clean fit, not an awkward
  special case, without changing behavior for the 12 existing adapters.
- Add the two small, reusable transport seams the harder sources need (per-request
  header; HTML-derived description) without coupling them to those adapters.

**Non-Goals:**

- `tochka` — deferred (fragile multi-row RSC flight-payload for ~48 vacancies; see
  Decision 6).
- Region/country normalization at ingest — tracked separately as `ingest-job-geography`.
- hh.ru-backed sources (legally off-limits), headless-browser sources
  (Cian-direct, SberHealth), and HTML/JSON-LD-scrape sources (Avito, Kontur, 2GIS).
- Any DB schema change or new third-party dependency.

## Decisions

### 1. `board` optionality via an in-package `boardless` marker interface

Single-company APIs have no tenant/board concept, but `Config.Validate` requires a
non-empty `board`. Rather than make every adapter declare board-need (interface churn on
12 adapters) or relax the check globally (loses fail-fast for real boards), add a marker:

```go
type boardless interface{ boardless() }
```

`Validate` skips the empty-board check only when `registry[provider]` implements it. The
two boardless adapters opt in with a one-line method; Yandex does **not** (its `board` ∈
{`ru`,`com`} selects host+language and stays required). *Alternative considered:* a
`RequiresBoard() bool` on the `Source` interface — rejected because it forces edits to all
12 existing adapters for a property only 14 new ones care about.

### 2. Per-request headers as additive `HTTPClient` variants

MTS requires a non-secret `x-api-key`. Add `GetJSONWithHeaders` /
`PostJSONWithHeaders`; the existing `GetJSON`/`PostJSON` delegate with `nil` headers.
*Alternative considered:* a variadic functional-option on the existing methods — rejected
because it still changes the interface signature (breaking the test fakes) for no gain
over two explicit methods that read clearly at the call site. Custom headers are layered
on top of the existing `User-Agent`/`Accept` and the unchanged retry/backoff.

### 3. MTS key is harvested at runtime, not configured

The `x-api-key` is a public JWT baked into the MTS SPA's Nuxt runtime config, served to
every visitor — not a secret. The adapter harvests it from the public config at `Fetch`
time. *Alternative considered:* an `MTS_API_KEY` env var — rejected as operational
overhead for a value that MTS publishes and rotates; a harvest failure is board-isolated
(fails only MTS, per the existing isolation requirement).

### 4. VK description by HTML extraction, isolated to `vk.go`

VK exposes no description in JSON; the vacancy page renders schema.org `JobPosting`
microdata, so VK reuses the existing `itempropHTML` extraction (from the SuccessFactors
adapter) over `golang.org/x/net/html` (already a direct dependency), then `sanitizeHTML`.
*Alternative considered:* skipping VK — rejected (248 live roles). The markup coupling is
contained to one adapter and a parse failure drops only that posting.

### 5. Pagination is per-adapter; reuse `fetchDetails` for the fan-out

The APIs paginate several ways — cursor (Yandex, VK), page-number (Ozon, Kuper),
offset/`skip` (RWB, Sber, Alfa, Lamoda, MTS, T-Bank), and single-shot
(Aviasales, Dodo, DomClick, MTS Link). Each adapter owns its loop; where the list omits
the description, the shared bounded `fetchDetails` helper performs the detail fan-out, so
isolation and concurrency bounds stay identical across platforms.

### 6. Tochka deferred

Tochka's detail is only reachable as a fragile multi-row Next.js RSC flight-payload (with
`$NN` reference resolution) for ~48 vacancies — too much coupling for the volume. Deferred
as a known seam rather than shipped as a brittle parser; the other 14 adapters land.

## Risks / Trade-offs

- **MTS key rotation / harvest path changes** → board-isolated failure; could later move
  to a config env var if harvest proves brittle.
- **VK page markup is version-coupled** (reuses the schema.org `itempropHTML` extraction)
  → isolated per adapter; a parse failure drops the posting, not the board.
- **Unconfirmed public vacancy URLs (Kuper, Dodo, T-Bank)** → synthesized best-effort;
  must be live-confirmed during the verification task before merge.
- **Sber rate-limits (503 on rapid paging) and is high-volume (~3788)** → throttle between
  pages and lean on the client's 429/5xx backoff.
- **prodradar reference is unlicensed** → endpoints/field maps were independently
  live-verified, never copied; prodradar is reference-only.

## Migration Plan

Additive only — no DB migration, no config breakage. Existing `sources/*.yml` and the 12
existing adapters are untouched; new `sources/<provider>.yml` files are added.
Rollback = drop the new adapter files, registrations, and config files. Roll out by
enabling providers incrementally in cron if desired (each board is independent).

## Open Questions

- Confirm the best-effort public URLs for Kuper, Dodo, and T-Bank live before merge; if a
  pattern cannot be confirmed, store the API/source URL and flag for follow-up.
