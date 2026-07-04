## Why

A signed-in user browsing a job has no immediate signal of how well it fits their own skills — they must eyeball the job's skill tags against what they know. The profile already has a "market coverage" view, but nothing answers the concrete question on the job page: *"how much of THIS job do I already cover, and what am I missing?"* A per-job match block turns the sidebar into a personal fit signal and a nudge to complete the profile.

## What Changes

- **New backend endpoint** `GET /api/v1/jobs/:slug/match` (behind `RequireAuthOrKey`, slug-addressed, mirroring the per-user endpoints in `user_jobs.go`). It classifies each of the job's skills against the caller's profile skills as **exact**, **adjacent**, or **missing**, and returns a coverage percent. Deterministic — **no LLM**.
- **Adjacency helper** — add `adjacentVia(required, held) → (via, ok)` to `internal/verdict/adjacent.go` alongside the existing `adjacentHeld`, so the response can name *which* held skill made a job skill count as adjacent (e.g. job wants `aws`, you have `gcp`).
- **New TS contract type** for the match response, generated via `cmd/gen-contracts`.
- **New frontend component** `JobMatch.svelte`, placed at the **top of the `JobView.svelte` sidebar**, with four states: not-enough-data, guest teaser (blurred + "Войти"), no-profile teaser (blurred + "Загрузить CV"), and the real match block (percent + two-colour progress bar + three chip groups: Есть / Близкие / Не хватает).

## Capabilities

### New Capabilities
- `job-profile-match`: The per-job skill-coverage match between an open job and the signed-in user's profile — endpoint, classification rules (exact/adjacent/missing), coverage-percent formula, and the sidebar block with its authed/guest/no-profile/empty states.

### Modified Capabilities
<!-- None: the adjacency helper and gen-contracts changes are implementation details of the new capability; no existing spec's requirements change. -->

## Impact

- **API**: new `GET /api/v1/jobs/:slug/match`.
- **Backend code**: new handler (`internal/handler/`), a small addition to `internal/verdict/adjacent.go`, route wiring in `handler.Register`, and a generated contract type.
- **Frontend**: new `web/src/lib/components/JobMatch.svelte`, one insertion into `JobView.svelte`, a new `api.ts` call, and the regenerated `contracts.ts`. Reuses existing `isAuthenticated()` and `profileStore`.
- **Out of scope (YAGNI)**: caching, match history, any influence on search/facets, and any LLM involvement.
