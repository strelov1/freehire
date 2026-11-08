## Context

Three more public ATS APIs, researched live:

- **Workable** `GET apply.workable.com/api/v1/widget/accounts/{board}?details=true` → `{jobs:[...]}`. Job: `title`, `shortcode`, `url`, `description` (HTML, inline), `published_on`/`created_at`, `city`/`state`/`country`, `telecommuting` (bool). One request.
- **Recruitee** `GET {board}.recruitee.com/api/offers/` → `{offers:[...]}`. Offer: `title`, `careers_url`, `location`, `description` (HTML), `requirements` (HTML, separate), `created_at`, `id`, `remote` (bool). One request.
- **SmartRecruiters** `GET api.smartrecruiters.com/v1/companies/{board}/postings?limit=100&offset=N` → `{content:[...], totalFound}`, paginated; list items have **no description** (only `id`, `name`, `location{city,region,country,remote}`, `releasedDate`). Description comes from `GET .../postings/{id}` → `jobAd.sections{companyDescription, jobDescription, qualifications, additionalInformation}` (HTML).

All adapters implement the existing `Source` interface and reuse the shared `HTTPClient.GetJSON`. The established description convention (sanitized HTML via `sanitizeHTML`) is mandatory.

## Goals / Non-Goals

**Goals:**

- Three registered adapters, each yielding sanitized HTML descriptions.
- SmartRecruiters fully crawled (pagination + per-posting detail) with bounded concurrency.
- TDD with `fakeHTTP`-style tests like the existing adapters.

**Non-Goals:**

- No schema/migration change; no jobview/pipeline change.
- No auto-detect-provider (separate change).
- The OpenJobs harvest + `sources.yml` growth is a data follow-up, not adapter code.

## Decisions

**1. Reuse `GetJSON`; extend the client only if forced.** Workable and Recruitee are a single `GetJSON` each. SmartRecruiters loops `GetJSON` over offset pages and over posting ids — still just `GetJSON`. No client change expected.

**2. SmartRecruiters relaxes "no detail request".** The list has no description, so the adapter MUST fetch detail per posting. This is the spec relaxation. Alternative — store list-only data without description — rejected: it violates the description convention and defeats the point. Detail fetches run with bounded concurrency (a small worker pool, e.g. 8) so one board cannot fan out unboundedly; ordering of the returned slice is not significant.

**3. Description assembly per platform.**

- workable: `sanitizeHTML(description)`.
- recruitee: `sanitizeHTML(description + requirements)` (skip empties), mirroring the lever multi-field assembly.
- smartrecruiters: `sanitizeHTML(jobDescription + qualifications + additionalInformation)` (and optionally `companyDescription`); skip empty sections, wrap none in extra headings (the sections are already self-contained HTML).

**4. Location + remote per platform.** workable: join non-empty `city,state,country`, remote = `telecommuting`. recruitee: `location`, remote = `remote` bool. smartrecruiters: join `location.city/region/country`, remote = `location.remote` bool.

**5. external_id.** workable = `shortcode`; recruitee = `id`; smartrecruiters = posting `id`. The pipeline namespaces by board, as today.

## Risks / Trade-offs

- **SmartRecruiters N+1 latency** → bounded-concurrency detail fetches; one slow/failed posting must not abort the board (skip that posting, keep the rest). Per-board failure is already isolated by the pipeline.
- **Pagination correctness** (missing/duplicating postings) → loop by offset until `len(content) == 0` or collected ≥ `totalFound`; test with a two-page fixture.
- **Rate limiting** on SmartRecruiters detail bursts → modest concurrency cap; acceptable for a cron worker.
- **Workable `?details=true` payload size** → fine; one request, descriptions inline.

## Migration Plan

1. Land adapters + tests (TDD), register in `sources.All`.
2. `go build/vet/test` green.
3. Data follow-up: re-harvest OpenJobs for the three providers, validate slugs live, append working boards to `sources.yml`, re-ingest (idempotent).
4. Rollback: revert the commit; unregistered providers simply fail config validation if still listed in `sources.yml`, so remove those entries too.

## Open Questions

- Whether to include `companyDescription` in the SmartRecruiters assembly (boilerplate vs. completeness) — decide during implementation; lean toward excluding it.
