## Context

The `internal/sources` package is a registry of one adapter per ATS/job platform, all
implementing `Source.Fetch(ctx, CompanyEntry) ([]Job, error)`. Single-company adapters
(`uber`, `amazon`) implement the `boardless` marker so their board-file entries may omit
`board`. `app.tecla.io` is a React SPA backed by a public JSON API (`api.tecla.io`); its
`getPublicJobs` endpoint returns a paginated, structured list of postings — but it is a
**marketplace**, so each posting names its own employer, and the description it returns is
truncated (the full text sits behind login).

## Goals / Non-Goals

**Goals:**
- A `tecla` adapter that crawls the whole marketplace through `getPublicJobs` pagination and
  yields normalized `Job`s with the correct per-posting company.
- Mark every job remote (the platform is remote-only) and date it from `createdAt`.
- Follow the established boardless-adapter shape so the feature fits without restructuring.

**Non-Goals:**
- Salary (no field on `Job`), skills/seniority parsing (ingest dictionaries own that).
- Authenticated full-description fetch — out of scope; we accept the truncated public text.
- Cron scheduling — lives in the `freehire-ops` repo.

## Decisions

- **Adapter over `internal/sources`, not `linksource`.** Tecla exposes a whole board (a paginated
  list), which is what `Source` adapts; `linksource` resolves a single detail URL. The shape
  matches `uber`/`amazon` (boardless single endpoint), with the one twist below.
- **Per-job company from the payload.** Unlike every existing adapter, `Job.Company` comes from
  `posting.company.name`, not `e.Company`. `e.Company` ("Tecla") is only a validation placeholder.
  This is the one genuinely new behavior and is captured as a spec requirement.
- **JSON over the shared `HTTPClient`**, decoded into a typed `teclaResponse` (`data.jobs[]` +
  `data.pagination.countPages`). Paginate `?page=N` from 1 through `countPages`, bounded by a
  defensive `teclaMaxPages` cap (mirrors `gpMaxPages`) so a misbehaving `countPages` can't loop.
- **Remote is a source property, not a guess.** `Remote=true` / `WorkMode="remote"` for all jobs
  because the marketplace is remote-only — the same justification a "remote jobs" board would use.
  Location is left empty (`city`/`state` are blank in the payload).
- **`createdAt` layout.** The API emits `2026-05-25T17:02:00.864421` (no zone), which `parseRFC3339`
  rejects; a dedicated `time.Parse` layout `2006-01-02T15:04:05.999999` handles it, funneled through
  the shared `NotFuture` guard.
- **Description taken as-is, sanitized** with the shared `sanitizeHTML`, consistent with other adapters.

## Risks / Trade-offs

- [Truncated descriptions weaken the catalogue entry and LLM enrichment] → Accepted per product
  decision; the alternative (auth-gated fetch + credential storage) was judged not worth the
  complexity for an MVP daily ingest. The seam (an authenticated `jobDetail` fetch) is noted, not built.
- [`getPublicJobs` is an undocumented endpoint that could change shape] → The typed decode fails
  loudly and the per-board isolation means a tecla break is counted, not fatal to the ingest run.
- [Marketplace reposts a vacancy that also exists on its real ATS] → No cross-source dedup (dedup key
  is `(source, external_id)`); a duplicate under `source="tecla"` is acceptable and consistent with
  how every other source behaves.

## Open Questions

None — scope and behavior are settled.
