## Context

getmatch.ru is a Next.js IT job marketplace. Its public listing (`/vacancies`) is backed by a
keyless JSON API. A short reconnaissance established the surface:

- `GET /api/offers?offset=N&limit=M` → `{ meta: {total, offset, limit}, offers: [...] }`.
  `limit` defaults to 20; `total` was ~760 at survey time (755 `vacancy` + 5 `one_day_offer`).
- `GET /api/offers/{id}` → the same offer object plus richer fields, notably `description`
  (full HTML, ~1155 chars in the sample) vs the list's shorter `offer_description` (~459).
- `GET /api/vacancies` → `401 {"detail":"Login required"}` — a personalized surface, not used.

Each offer carries its own employer (`company.name`), so getmatch is a **marketplace
aggregator**, not a single-company board. This matches the existing `tecla`/`jobstash`
adapters: boardless, `aggregator()`-marked, with a placeholder company in the config entry.

`location_items[]` gives a per-location `format` from the vocabulary observed across the full
catalogue: `remote` (447), `hybrid` (609), `office` (124), `relocation_company` (77),
`relocation_candidate` (6). 614/760 offers have a single distinct format; 146 mix two or three.

## Goals / Non-Goals

**Goals:**

- Ingest getmatch.ru into the catalogue under `source = "getmatch"` via the existing pipeline,
  with full HTML descriptions and a clean structured work mode where unambiguous.
- Follow the established boardless-aggregator adapter pattern (tecla) so the change is one new
  adapter file + one registry line + one board file, with no pipeline changes.

**Non-Goals:**

- Salary ingestion (no field in the raw `Job` shape; enrichment owns compensation).
- Skill extraction from `skills_objects` (the project derives skills via `internal/skilltag`).
- `StreamingSource` / incremental persistence (noted seam; the fan-out is moderate today).
- Mapping department/specialization/seniority/team-size detail fields (classification is
  derived deterministically downstream).

## Decisions

**1. Boardless aggregator, mirroring `tecla`.** getmatch lists many employers behind one feed,
so the adapter implements `boardless()` (config entry needs no `board`) and `aggregator()` (it
stays in the source facet, unlike a single-company boardless adapter). The `company` comes from
each offer, not the config. *Alternative considered:* a board-per-company model — rejected, the
API has no per-company board id and the feed is global.

**2. Full description via per-offer detail fetch.** The list's `offer_description` is a short
summary; the detail endpoint's `description` is the full HTML body. We fetch
`/api/offers/{id}` per offer for the full text, since description quality drives both search and
AI enrichment. *Alternative considered:* list-only (1 request/page, ~8 total) — rejected for
the project's quality bar (tecla explicitly flags its truncated description as a downside). The
cost is ~755 extra requests per crawl, which is acceptable for a once-and-exit cron worker.

**3. Detail-description fallback.** Event cards (`one_day_offer`) carry their copy in
`offer_description` and may have an empty detail `description`. The adapter prefers the detail
`description` and falls back to the list `offer_description` when it is empty, so no posting is
dropped and event cards still carry usable text. A failed *detail request* for one offer also
falls back to the list summary rather than dropping the offer.

**4. Structured work mode only when unambiguous.** `Job.WorkMode` carries structured signal
only (never the location heuristic — that is the pipeline's job). We map the work-mode formats
`remote→remote`, `hybrid→hybrid`, `office→onsite` and ignore the `relocation_*` flags (they are
relocation support, orthogonal to work mode). We emit `WorkMode` only when the offer's locations
resolve to a **single** distinct work mode; a mixed offer leaves it empty so the pipeline's
location-string parser decides. *Alternative considered:* collapsing mixed remote+office to
`hybrid` — rejected as an inference that would muddy the structured field's provenance.

**5. Pagination by `meta.total` with a safety bound.** Page with `limit=100` and stop when
`offset >= meta.total`, bounded by a `maxPages` constant so a bad/absent total cannot loop —
the same guard `tecla` uses. A failed first page is a board error; a failed later page ends
enumeration with what was gathered.

## Risks / Trade-offs

- **Per-crawl request volume (~755 detail fetches)** → Mitigated by the shared HTTP client's
  rate limiting and the run-once-and-exit cron model; the `StreamingSource` seam is noted if it
  later needs incremental persistence.
- **API shape drift** (undocumented endpoint) → The adapter unmarshals only the fields it needs
  and fails the board loudly on a list-decode error rather than silently emptying the catalogue;
  the test pins the mapping against inline real-shaped JSON fixtures (mirroring `tecla_test.go`).
- **HTML descriptions** → Run through the shared `sanitizeHTML` (active content stripped,
  structure kept), as other HTML-yielding adapters do.
- **Mixed-format offers lose a structured work mode (146/760)** → Accepted; the location-string
  parser still derives a hint, and provenance stays clean.

## Migration Plan

Additive only — no schema or pipeline change. Deploy is a full image rebuild (new adapter
compiled in) plus one cron schedule for `sources/getmatch.yml`. Rollback is removing the cron
line and the registry entry; ingested rows are inert (`source = "getmatch"`) and soft-close
naturally via the existing sweep if the crawl stops.

## Open Questions

None — the two scope decisions (full-description detail-fetch; include event cards) are settled.
