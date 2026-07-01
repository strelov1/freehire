## Context

freehire already parses a résumé into canonical skill slugs (`internal/handler/resume.go` → `skilltag.Parse`, text discarded) and shows a per-profile "Market fit" ratio in the web UI (`web/src/lib/skillGap.ts` reading the `/jobs/facets` distribution). The verdict feature promotes that ratio into a full screen and adds an LLM "coherence" read of the résumé. The verdict UI (`web/src/lib/components/VerdictView.svelte`) is already built and styled to the app's flat monochrome theme, currently on mock data.

Key constraints:
- **Privacy invariant** (from `resume.go`): the raw résumé text is never persisted or logged.
- **Market facets** are the source of skill demand; the handler already has a `facetCounter` (`FacetCounts(ctx, search.FacetParams)`), the same source `/jobs/facets` uses.
- **LLM access** is provider-agnostic via `internal/llm` (`New`, `GenerateJSON`, `TruncateRunes`, `NewWithModel` test seam); the HTTP server does not build a client today — only `cmd/enrich` does.
- **Migrations** apply on fresh volume init only; existing/prod volumes need a manual apply (the open versioned-migration-runner seam).

## Goals / Non-Goals

**Goals:**
- Serve a per-profile verdict: deterministic market gap (stack match, must-haves, per-gap unlock) computed live from the profile, plus an optional LLM coherence score + gap advice.
- Persist only the derived AI layer (coherence + advice + `analyzed_at`) so it survives reload, never the résumé text.
- Degrade gracefully: deterministic verdict always renders; the AI layer is best-effort.
- One canonical gap computation shared in spirit with `skillGap.ts` (backend `internal/verdict.Compute` is the authority for this feature).

**Non-Goals:**
- Persisting or re-uploading the résumé text; a refreshed coherence requires a new upload.
- Classifying skills into stack/methodology (no data source) or humanizing skill slugs beyond what the UI already does.
- Rate-limiting the LLM endpoint beyond the existing global limiter and input truncation (noted as a follow-up).
- Changing the existing profile "Market fit" widget or `skillGap.ts`.

## Decisions

**1. Deterministic core in a pure `internal/verdict` package, computed live.**
`Compute(market MarketSkills, candidate []string) Verdict` is pure (no I/O, no LLM), mirroring `skillGap.ts` (top-20, count-desc/slug-asc sort, coverage, unlock = count/total). It is computed on every read from the profile's saved `skills` + `specializations`, so editing the profile refreshes the verdict without a re-upload. Alternative (persist a full verdict snapshot) rejected: it goes stale when the profile changes and duplicates market data.

**2. Must-have = demand-share threshold.** `MustHave = count/total ≥ MustHaveShare` (constant, start 0.40), self-explanatory in the UI and role-independent. Alternative (top-K by rank) rejected as arbitrary. The constant is calibrated against real facet data during verification.

**3. AI layer is a separate `Analyzer` over `internal/llm`, best-effort, persisted as derived data.** `Analyzer.Analyze(ctx, resumeText, gaps) (*Analysis, error)`; a nil client (LLM unconfigured) returns `(nil, nil)`. One JSON call returns `{coherence:int, advice:{slug:string}}`; the caller clamps coherence to 0-100, drops advice for non-gap slugs, and truncates advice length. On any error the handler swallows it (logs the error, never the text) and serves the deterministic verdict. The derived analysis is stored as `resume_analysis JSONB` on `search_profiles`; the résumé text is used only in-request.

**4. Two endpoints on the profile sub-resource.**
- `GET /api/v1/me/profiles/:id/verdict` — compute the deterministic verdict live and merge any stored `resume_analysis`.
- `POST /api/v1/me/profiles/:id/verdict` — parse the uploaded résumé (PDF multipart `file` or JSON `{text}`, reusing `resumeText`/`pdfText` from `resume.go`), run the analyzer, persist the derived analysis, return the full verdict.
Both `RequireAuth` (cookie-only, matching the other `/me/profiles` routes) and ownership-checked via the `searchprofile` service (missing/other-owner → 404).

**5. Server LLM wiring mirrors the Meili nil-guard.** Add `LLMBaseURL/LLMAPIKey/LLMModel` to the server config; in `cmd/server`, build an `*llm.Client` only when all three are set and pass it into `handler.Config`; `handler.Register` wraps it in `verdict.NewAnalyzer` (nil-safe). No vendor/model hard-coded.

**6. Contract via `cmd/gen-contracts`.** Add `internal/verdict` to the tygo packages so `Verdict`/`Skill` become the TS types the frontend consumes; `VerdictView.svelte` is refactored from mock data to a `verdict` prop of that type.

## Risks / Trade-offs

- **Must-have threshold may yield too few/many must-haves on sparse dictionary tagging** → make it a single tunable constant and calibrate against live facet counts in verification; UI legend states the rule.
- **LLM latency on an interactive request** (default 90s timeout in `internal/llm`) → frontend shows a busy state; the deterministic verdict is available without the AI call. A shorter analyzer timeout is a possible follow-up (would need an `llm` API addition, out of scope now).
- **Stored analysis goes stale relative to an edited profile** → acceptable; the coherence is about the résumé, not the market, and re-uploading refreshes it. `analyzed_at` is surfaced so staleness is visible.
- **User-influenced text amplifying token cost** → input bounded via `TruncateRunes` (mirrors enrich's `maxDescriptionRunes`); advice output bounded by the ≤20 must-have gap slugs and per-entry truncation.
- **Migration on prod** → the `resume_analysis` column must be applied manually before the new binary rolls (documented seam); the column is nullable with no default, so it is backward-compatible with old rows.

## Migration Plan

1. Add migration `migrations/00NN_search_profiles_resume_analysis.sql`: `ALTER TABLE search_profiles ADD COLUMN resume_analysis JSONB;` (nullable, no default).
2. `make sqlc` after adding the new queries; commit generated code.
3. Deploy order: apply the migration manually on the persistent/prod DB **before** rolling the new server binary (which references the column via the profile fetch query).
4. Rollback: `ALTER TABLE search_profiles DROP COLUMN resume_analysis;` after reverting the binary. No data loss beyond the derived analyses.

## Open Questions

- Exact `MustHaveShare` value — resolve by inspecting live facet distributions for a few categories during verification (start 0.40).
