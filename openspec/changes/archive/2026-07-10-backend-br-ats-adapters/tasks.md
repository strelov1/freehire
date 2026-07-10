## 1. Quickin adapter (account-slug, inline listing)

- [x] 1.1 Add `internal/sources/quickin.go`: `NewQuickin(JSONGetter)`, `Provider() "quickin"`, resolve board slug → account id via `GET /public/accounts/<slug>`, then page `GET /public/<accountId>/jobs`.
- [x] 1.2 Map each `published` posting inline (title, description+requirements, city/region/country, `workplace_type` → work mode, `career_url`, `created_at`); drop non-`published`.
- [x] 1.3 `internal/sources/quickin_test.go`: offline `routedHTTP` test covering account resolve, pagination, publish filter, sanitization, work mode, URL fallback.
- [x] 1.4 Register `NewQuickin(c)` in `sources.All`; add `sources/quickin.yml` (BotCity, Igma — live-validated); regenerate contracts.
- [x] 1.5 Live end-to-end fetch of BotCity + Igma returns postings with mapped fields.

## 2. Mindsight adapter (Next.js `__NEXT_DATA__`, detail body)

- [x] 2.1 Add `internal/sources/mindsight.go`: `NewMindsight(TextGetter)`, `Provider() "mindsight"`, fetch `oportunidades.mindsight.com.br/<slug>`, `bracketSlice` `__NEXT_DATA__`, decode `publicJobPostings`.
- [x] 2.2 Enrich each `IN_PROGRESS` posting with the description from its detail page's `jobPosting.description`; map `work_model` → work mode, country/state/city → location, `external_publication_start_at`|`created_at` → posted.
- [x] 2.3 `internal/sources/mindsight_test.go`: offline test covering listing decode, closed-post drop, detail body enrichment, sanitization, posted-date preference.
- [x] 2.4 Register `NewMindsight(c)` in `sources.All`; add `sources/mindsight.yml` (Grupo MNGT — live-validated); regenerate contracts.
- [x] 2.5 Live end-to-end fetch of the seeded board returns postings with mapped fields.

## 3. Enlizt adapter (JSON-LD)

- [x] 3.1 Reverse-engineer enlizt's public listing + detail (`<tenant>.enlizt.me`): confirm keyless list endpoint and `JobPosting` JSON-LD shape.
- [x] 3.2 Add `internal/sources/enlizt.go` implementing `Source` over the resolved transport; map to `Job` (title, URL, external id, location, work mode, posted, sanitized body).
- [x] 3.3 `internal/sources/enlizt_test.go`: offline fixture test of the mapping and any drop/fallback paths.
- [x] 3.4 Register in `sources.All`; add `sources/enlizt.yml` (live-validated tenant); regenerate contracts.
- [x] 3.5 Live end-to-end fetch returns postings with mapped fields.

## 4. Descoped — Workfully & Strider (not keyless)

Both platforms referenced by `backend-br/vagas` were reverse-engineered and found to sit
behind authentication, failing the adapter bar (`AGENT.md`: adapters are read-only over
**public** ATS APIs). Descoped by decision; recorded as a known seam in the proposal.

- **Workfully** (`hiring.workfully.com`): the SPA calls `api.hiring.workfully.com`; every
  job endpoint (`/jobs/{id}`, `/jobs/{id}/public`, `/recruiters/{id}/jobs`) returns
  `403 ForbiddenError "You don't have rights"` without a session token, and the page is
  RSC-rendered (no ld+json / `__NEXT_DATA__`). Covers 1 company (Stefanini).
- **Strider** (`app.onstrider.com`): a Clerk-authenticated CRA SPA; `.../api/*` returns
  `401 "User not authenticated"`. A US talent marketplace (not a company ATS), 1 recruiter.

- [x] 4.1 Confirm neither exposes a keyless public jobs API; document the block and descope.

## 5. Cross-cutting verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./internal/sources/` clean with the three adapters registered.
- [x] 5.2 Each new provider (quickin/mindsight/enlizt) appears in `FilterableProviders()` / `SOURCE_VALUES`; config validation requires a board for each (none boardless).
- [x] 5.3 A dry `cmd/ingest sources/<provider>.yml` config-validation pass loads each board file without error.
