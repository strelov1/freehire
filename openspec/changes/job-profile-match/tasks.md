## 1. Adjacency helper

- [x] 1.1 Add `AdjacentVia(required string, held map[string]bool) (string, bool)` to `internal/verdict/adjacent.go`, returning the first held neighbour of `required` per the curated dictionary; `adjacentHeld` left as-is (neighbour-order semantics differ). Unit tests: known pair returns via, no neighbour returns `("", false)`, no-adjacency-entry case, first-listed-neighbour-wins.

## 2. Match computation (pure)

- [x] 2.1 Add a pure `Compute(jobSkills, profileSkills []string) Result` in a small package (`internal/jobmatch`) that classifies each job skill as exact / adjacent (with `via`) / missing and computes `coverage_percent = round((exact + 0.5×adjacent)/total×100)`. Exact takes precedence over adjacent. Unit tests: the 5-skill worked example (2 exact, 1 adjacent, 50%), precedence, half-weight rounding, `total=0` → zeroed result with empty lists.

## 3. Endpoint

- [x] 3.1 Define the response struct (`total`, `exact_count`, `adjacent_count`, `coverage_percent`, `matched []string`, `adjacent []{name,via}`, `missing []string`) and add a handler `JobMatch` in `internal/handler/` that resolves the job by `public_slug`, loads the caller's profile skills, runs `jobmatch.Compute`, and returns `{"data": ...}`. Unknown slug → `pgx.ErrNoRows` (central handler → 404).
- [x] 3.2 Wire `GET /api/v1/jobs/:slug/match` behind `RequireAuthOrKey` in `handler.Register`.
- [x] 3.3 Handler integration test (`//go:build integration`): authed caller gets the expected classification/percent; unknown slug → 404. (Auth-missing → 401 is covered by the middleware.)

## 4. Contract generation

- [x] 4.1 Run `cmd/gen-contracts` and commit the regenerated `web/src/lib/generated/contracts.ts` so the match response type is available to the SPA.

## 5. Frontend data access

- [x] 5.1 Add a `getJobMatch(slug)` call in `web/src/lib/api.ts` returning the generated match type.

## 6. Sidebar component

- [x] 6.1 Create `web/src/lib/components/JobMatch.svelte` implementing the four states: not-enough-data (empty `job.skills`), guest teaser (blurred, static figures, footer "Войти", no API call), no-profile teaser (blurred, footer "Загрузить CV", no API call), and real match (fetch via `getJobMatch`, render percent + two-colour progress bar + three chip groups Есть/Близкие/Не хватает with `via` hint). State chosen from `job.skills`, `isAuthenticated()`, and `profileStore.ensureLoaded()` without redundant calls. Use inline SVG for lock/document icons and the app's colour tokens.
- [x] 6.2 Extract and unit-test (vitest) any pure formatting/state-selection logic used by the component (e.g. state resolver given auth/profile/skills, progress-bar segment widths).

## 7. Integration into job page

- [x] 7.1 Insert `<JobMatch>` at the top of the `JobView.svelte` sidebar (above salary/metadata), passing `job` (and slug). Verify with `svelte-check` and a visual check across the four states.

## 8. Verify

- [x] 8.1 `go build ./... && go vet ./... && go test ./...`; run the handler integration test; `svelte-check` clean. Confirm the four states render correctly against a running app.
