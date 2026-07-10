# Phase 1 — deterministic ATS score (no LLM, no migration; independently shippable)

## 1. Core scorer (pure)

- [x] 1.1 RED: `internal/atscheck` tests — `Score(cvText, roleTopSkills)` returns `{Overall, Readability, KeywordMatch, Checks[]}`; scanned/near-empty text ⇒ `machine_readable` fail + low readability; a clean CV ⇒ structural checks pass + high readability; determinism (same input → same output).
- [x] 1.2 GREEN: implement the deterministic checks (machine_readable, contact email+phone, sections via a curated EN+RU heading dict, dates, length band, bullets) with per-check `{id,status,label,fix}`; Readability = weighted pass-rate; weights/thresholds as named constants. Pure, no I/O.
- [x] 1.3 RED+GREEN: keyword-match — `cvSkills = skilltag.Parse(cvText, WithResumeAcronyms())`, `matched = cvSkills ∩ roleTopSkills`, `KeywordMatch = round(len(matched)/N·100)`; the `keyword_match` check's fix names top missing role skills. Test: 2-of-3 role skills present → correct score + missing named; distinct from market-coverage (uses CV text, not profile skills).
- [x] 1.4 GREEN: `Overall = round(wR·Readability + wK·KeywordMatch)` (content-quality term absent in Phase 1); named weight constants. Tests for the blend + clamping to [0,100].

## 2. HTTP endpoint (GET, deterministic)

- [x] 2.1 RED: handler tests (fake facets + fake résumé store) — owner-scoped 404; 503 when facets nil; "no CV" 200 state when storage enabled but none stored; role from `?category=` param (reuse `roleValues`); happy path returns score + checks.
- [x] 2.2 GREEN: `internal/handler/ats_report.go` `GetATSReport` — resolve profile (owner-scoped), read stored CV text (`resume.Text`), build role filter + `FacetCounts(skills)` → top-N role skills, call `atscheck.Score`, return `{data: report}`. Wire `GET /me/profiles/:id/ats-report` in `handler.go`.

## 3. Contracts + frontend (deterministic section)

- [x] 3.1 Regen Go→TS contracts (`cmd/gen-contracts`) for the atscheck report shape; add to `$lib/types`; add `getATSReport(id, params)` to `$lib/api`.
- [x] 3.2 Web: a "CV readiness" section on `/my/profiles/[id]/verdict` — overall score, readability + keyword-match sub-scores, checklist (pass/warn/fail + fix), recomputed on filter change alongside coverage. Use "CV" wording.
- [x] 3.3 Web: re-introduce a CV upload/replace control on the verdict page (existing `extractResumeSkills`/résumé-storage path); after upload, the report recomputes. "CV" wording.

## 4. Verify Phase 1

- [x] 4.1 `go build ./... && go vet ./... && go test ./...`; `svelte-check` clean; confirm the deterministic report renders and recomputes on filter/CV change. Deterministic phase ships with NO LLM and NO migration (nil-LLM path = the same degraded score).

# Phase 2 — optional LLM qualitative layer

## 5. Server LLM client (re-added, nil-safe)

- [x] 5.1 Re-add `config.Config` LLM/Langfuse fields + `config.Load`; re-add `cmd/server` `llm.NewClient` construction (nil-safe, gated on `LLM_*`) + Langfuse flush on shutdown; pass the client into `handler.Config`. Tests: config load; nil when unset.

## 6. LLM analyzer + cache

- [x] 6.1 RED+GREEN: `atscheck.Analyzer` (mirrors old `coherence.go`) — `Analyze(ctx, cvText) → *Review{ContentQuality, Findings[]}` via `llm.Client` (nil ⇒ (nil,nil)); `GenerateJSON` + Sanitize (clamp 0-100, bound strings). Unit tests with a fake model.
- [x] 6.2 Migration `migrations/00NN_users_resume_ats_analysis.sql` — `ALTER TABLE users ADD COLUMN resume_ats_analysis JSONB;` (manual-apply BEFORE the Phase-2 binary; `SELECT *` users query). Add queries to set/clear/read it; `make sqlc`. Invalidate (clear) in `resume.Put`/`Delete`.
- [x] 6.3 GREEN: `POST /me/profiles/:id/ats-report` — read stored CV, `Analyze`, cache to `users.resume_ats_analysis`, return merged report (best-effort: LLM error ⇒ 200 deterministic). `GET` merges the cached review + folds `content_quality` into `Overall`.

## 7. Frontend (AI review)

- [x] 7.1 Web: "Run AI review" button (POST) when LLM is enabled; render content-quality + findings/fixes; hide cleanly when no LLM. Update contracts/types.

## 8. Verify + ops

- [x] 8.1 `go build ./... && go vet ./... && go test ./...`; `svelte-check`; validate the LLM path with a fake + (locally) a real CV. Ops: apply the `users` migration before deploy; ensure `LLM_*` present in the app env to enable the AI layer (else deterministic-only). No reindex.
