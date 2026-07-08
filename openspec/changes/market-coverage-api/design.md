## Context

The market-coverage verdict (`internal/verdict` + `GetResumeVerdict`) already
scores a candidate's skills against the live market, but it is cookie-only and
sources its skills from a stored profile + parsed CV, defaulting the role from the
profile's specializations. `verdict.Compute` itself is pure and I/O-free: it takes
an `Input` (three facet-query results + declared/body/all skill sets) and returns
the scored `Verdict`. An agent driving `freehire-cli` has only a flat skill list
and an API key — it needs a stateless entry into the same computation.

(Full design narrative: `docs/superpowers/specs/2026-07-08-market-coverage-api-design.md`.)

## Goals / Non-Goals

**Goals:**
- A stateless `POST /api/v1/market/coverage` (API key or cookie) taking skills in
  the body and a market filter in the query, returning the coverage verdict.
- Reuse `verdict.Compute` unchanged; extract the shared three-query step so the
  new endpoint and the CV verdict share one implementation.
- Full facet-filter vocabulary via the existing `buildSearchFilter`.
- `freehire-cli`: `market-fit` command + generic `--facet key=value` filtering on
  `market-fit` and `search`.

**Non-Goals:**
- No coverage segmentation into per-facet-value buckets.
- No CV parsing / `coherence` / `hidden` status in this path.
- No new market data, LLM, DB migration, or reindex.

## Decisions

- **Reuse over re-implement.** `verdict.Compute` stays as-is. Extract
  `coverageFor(ctx, roleFilter, declared, body, all) (verdict.Verdict, error)` from
  `resume_verdict.go` — the three facet queries (role total; uncovered via
  `AndNotSkills`; skill-bearing total via `AndSkillsPresent`) plus `Compute`.
  `GetResumeVerdict` and the new handler both call it. This keeps the ranking /
  must-have / gap logic single-source.
- **Flat-list skill mapping.** New handler calls
  `coverageFor(ctx, filter, skills, nil, skills)` — `Declared = skills`,
  `Body = nil`, `All = skills`. Statuses collapse to `strong / adjacent / missing`;
  `coherence_percent` is meaningless (0) and is omitted from the response.
- **Transport.** Filter as query params (reuse `buildSearchFilter(c)` — the whole
  vocabulary for free); skills as a JSON body `{"skills": [...]}`. `POST` because
  the caller submits a body; the route sits with the other `keyAuth` job routes.
- **Response shape.** `{"data": verdict}` reusing the existing `verdict.Verdict`
  contract (already codegen'd to TS), with `coherence_percent` zeroed.
- **CLI filters.** A generic `--facet key=value` (repeatable) mapped straight into
  `url.Values` covers the full 22-facet + numeric vocabulary without 22 named
  flags; keep the existing convenience flags and add high-traffic ones.

## Risks / Trade-offs

- **Cost.** Each call runs three Meili facet queries; API-key gating (not public)
  bounds anonymous abuse.
- **Contract coupling.** The response reuses `verdict.Verdict`, so a future verdict
  field lands here too. Acceptable — both are the same coverage concept; the
  stateless path just omits the CV-section metric.
- **Cross-repo.** The CLI lives in the separate `freehire-cli` repo; its tasks run
  there, not in this repo's worktree.
