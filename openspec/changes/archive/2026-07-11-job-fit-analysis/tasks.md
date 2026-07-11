## 1. Data model & queries

- [x] 1.1 Add `migrations/0009_user_job_analysis.sql`: table `user_job_analysis (user_id BIGINT, job_id BIGINT, analysis JSONB NOT NULL, model TEXT NOT NULL, cv_uploaded_at TIMESTAMPTZ, job_content_hash TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (user_id, job_id))` with FKs to `users(id)` and `jobs(id)` ON DELETE CASCADE
- [x] 1.2 Add sqlc queries in `internal/db/queries/`: `GetUserJobAnalysis` (by user_id, job_id) and `UpsertUserJobAnalysis`; run `make sqlc` and commit generated code
- [x] 1.3 Confirm `internal/jobhash` exposes the job `content_hash` used at ingest, and identify the query returning it for a job by slug — `jobs.content_hash` is already on the `Job` row via `GetJobBySlug (SELECT *)`; no query change needed

## 2. Core fit engine (`internal/jobfit`) — three-stage chain

- [x] 2.1 RED: test the `Analysis` contract + `sanitize` — dimension scores clamp to 0–100, verdict coerces to the controlled set, requirement-match statuses coerce to {covered,synonym-only,missing-have,missing-gap}, strengths/gaps/recommendation trimmed and count/length bounded
- [x] 2.2 GREEN: define the wire structs — `Analysis` (five `Dimension` scores, `RequirementMatch []Requirement`, `OverallScore`, `Verdict`, `Strengths[]`, `Gaps[]`, `Recommendation`) + per-stage structs (`RequirementMatch`, recruiter `verdict`) + `sanitize`, mirroring `internal/atscheck`
- [x] 2.3 RED: test `scoreOverall` — weighted average (Title 25 / Experience 25 / Seniority 15 / Skills 20 / Company 15, named constants) and `verdictFor` threshold mapping to Strong/Good/Moderate/Weak/Poor
- [x] 2.4 GREEN: implement server-side `overall_score` + verdict derivation (model's own overall ignored)
- [x] 2.5 RED: test the three stage prompt builders — Stage 1 (extract requirements + classify vs CV) carries job title+description + CV + anchor; Stage 2 (recruiter verdict) carries Stage-1 match + company_info + CV + anchor; Stage 3 (adversarial audit) carries the Stage-2 verdict + evidence; all bound runes like `atscheck.maxCVRunes`
- [x] 2.6 RED: test the `Analyzer` chaining + degradation — Stage 3 parse-fail falls back to the Stage-2 verdict; Stage 1/2 fail ⇒ `(nil, err)`; nil client ⇒ `(nil, nil)`
- [x] 2.7 GREEN: implement `Analyzer.Analyze` over `*llm.Client` — run Stage 1 → Stage 2 → Stage 3 as sequential `GenerateJSON` calls (per-stage timeout), sanitize each, compute overall/verdict from the audited dimensions; log failures without CV/job text

## 3. HTTP endpoints (`internal/handler/job_fit.go`)

- [x] 3.1 RED: `job_fit_integration_test.go` (build tag `integration`) — GET on a never-analyzed job returns `has_cv` + null analysis and no LLM call; unknown slug 404; unauthenticated 401
- [x] 3.2 GREEN: implement `GetJobFit` — resolve user + job + profile + CV; run `jobmatch.Compute` anchor; read cache; compute `stale` by comparing stored `cv_uploaded_at`/`job_content_hash` to live values; respond `{data:{has_cv,stale,analysis}}`
- [x] 3.3 RED: test `PostJobFit` — computes via a fake analyzer, upserts the cache row with the current CV upload time + job content hash, returns fresh; LLM-unconfigured degrades to `200` no-analysis and no row; no CV ⇒ `has_cv:false`
- [x] 3.4 GREEN: implement `PostJobFit` (best-effort: log without CV/job text on failure, serve no-analysis)
- [x] 3.5 Wire routes in `internal/handler/handler.go` (`GET`/`POST /jobs/:slug/fit`, `RequireAuthOrKey`) and construct the `jobfit.Analyzer` + cache store in `Register` (reuse the existing `llm.Client`)

## 4. Wire contract & frontend

- [x] 4.1 Add `jobfit.Analysis` to `cmd/gen-contracts`; regenerate TS types into `web/src/lib/types.ts`
- [x] 4.2 Add `api.getJobFit(slug)` / `api.postJobFit(slug)` to `web/src/lib/api.ts`
- [x] 4.3 Extend `web/src/lib/components/JobMatch.svelte`: keep the deterministic bar on top; add an "AI fit analysis" expander that GETs on expand and POSTs on the explicit compute/recompute action; render the five dimensions, overall score + verdict, the ATS requirement-match table (covered/synonym/missing-have/missing-gap), strengths/gaps/recommendation; surface a stale banner offering recompute
- [x] 4.4 Extract any pure presentation logic (verdict→colour, dimension ordering) into `web/src/lib/jobFit.ts` and unit-test with vitest (mirrors `jobMatch.ts`)

## 5. Verify & finish

- [x] 5.1 `go build ./... && go vet ./... && go test ./...`; run the integration test with `-tags=integration` — all green (65 unit pkgs + handler integration incl. testcontainers)
- [x] 5.2 Frontend: `svelte-check` (0 errors) + `vitest` (122 pass) + eslint (clean) + production SSR build. NOTE: interactive browser-rendered expanded state not driven live (needs an auth session + backend + LLM); verified at type/logic/compile level only
- [x] 5.3 Document in `AGENT.md` the new job-fit convention (on-demand, cached per (user,job), staleness stamps, best-effort) and note the manual migration step for deploy
