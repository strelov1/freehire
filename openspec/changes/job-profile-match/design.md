## Context

The SPA already exposes both inputs needed for a per-job match on the job page: `job.skills[]` (canonical skilltag slugs, arriving with the SSR-loaded job) and the user's `profile.skills[]` (loaded once via `profileStore.ensureLoaded()` from `/api/v1/me/profile`). The existing profile "market coverage" (`internal/verdict`, `/me/profile/verdict`) is backend-computed because it needs Meilisearch market distributions — irrelevant here. For a single job the match is a set operation over two skill lists, plus the curated adjacency dictionary in `internal/verdict/adjacent.go` (react↔vue↔angular, postgresql↔mysql, aws↔gcp, pytorch↔tensorflow, …).

Auth and profile state are already first-class in the SPA: `isAuthenticated()` (from `page.data.user`, SSR-safe) and the `profileStore` singleton.

## Goals / Non-Goals

**Goals:**
- Answer "how much of this job do I cover, and what's missing?" at the top of the job sidebar.
- Deterministic, no LLM; reuse the canonical adjacency dictionary rather than duplicating it.
- Four clear UI states (real match / guest teaser / no-profile teaser / not-enough-data) with no redundant network calls.

**Non-Goals:**
- Caching, match history, or persistence of match results.
- Any influence on search ranking or facets.
- LLM-based similarity or "coherence" scoring.
- Multi-profile matching (profile is a per-user singleton).

## Decisions

**Compute on the backend, not the client.** Although both skill lists are already in the browser, the canonical skill vocabulary and the adjacency dictionary live in Go. Duplicating the adjacency map into TypeScript would create a second source of truth that drifts. A new `GET /api/v1/jobs/:slug/match` keeps the logic in one place and returns a typed, ready-to-render shape. *Alternative considered:* pure client-side intersection — rejected because it can't do adjacency without re-implementing the dictionary, and adjacency is a chosen feature.

**Mirror the `user_jobs.go` per-user endpoint shape.** The endpoint sits behind `RequireAuthOrKey`, is addressed by `public_slug` (resolved to the internal id before the read), and returns `{"data": ...}`. This matches the existing per-user job endpoints exactly, so route wiring, auth, and error handling follow established patterns (`pgx.ErrNoRows` → 404 via the central `ErrorHandler`).

**Add `adjacentVia`, keep `adjacentHeld`.** The current `adjacentHeld(required, held) bool` answers "is any neighbour held?" but not "which one". The UI hint "· у вас GCP" needs the specific neighbour, so add `adjacentVia(required string, held map[string]struct{}) (string, bool)` returning the first held neighbour. `adjacentHeld` can be expressed in terms of it or left as-is; the classification path uses `adjacentVia`. Pure and deterministic. *Alternative:* return all neighbours — rejected as YAGNI; one representative via is enough for the chip.

**Percent weights adjacent at one half.** `round((exact + 0.5×adjacent)/total×100)`. Exact skills are full credit; a transferable-but-not-exact skill is partial. This drives both the headline number and the two-segment progress bar (green = exact width, amber = half-weight adjacent width). *Alternative:* adjacent as full or zero credit — half is the honest middle and matches the validated design.

**Frontend gates the state; the endpoint only computes.** `JobMatch.svelte` decides which of the four states to render from data it already has — `job.skills` (SSR), `isAuthenticated()`, and `profileStore` — and only calls the endpoint in the real-match state. The guest and no-profile teasers use static, non-real figures behind a light blur, so no data leaks and no wasted request. This keeps the endpoint single-purpose (assume an authenticated caller with a profile) and the UI logic co-located.

**Generate the TS type via `cmd/gen-contracts`.** The Go response struct is the source of truth; the SPA consumes the generated `contracts.ts` type, consistent with the rest of the codebase.

## Risks / Trade-offs

- **Adjacency dictionary is curated and incomplete** → the amber "Близкие" group only reflects known neighbour pairs; unknown transferable skills fall to "Не хватает". Acceptable: the same curated-dictionary trade-off as the rest of the skill tooling; it never guesses.
- **Job skills vs profile skills vocabulary mismatch** → both are canonical skilltag slugs produced by the same `internal/skilltag` dictionary, so they compare directly; no normalization step needed. Guard with a test that compares real slugs.
- **Guest teaser shows a static 76%** → could be mistaken for a real score. Mitigation: the light blur + lock affordance and CTA make the "sign in to see yours" intent clear; the figure is fixed demo content.
- **Extra `/me/profile` fetch on the job page** → the match component triggers `profileStore.ensureLoaded()`; this is a one-time cached fetch already used elsewhere, and only for authenticated viewers.

## Migration Plan

Additive only — a new endpoint, a new component, one insertion into `JobView.svelte`, and a regenerated contract. No schema changes, no data migration. Rollback is removing the route wiring and the component insertion. Deploy backend (endpoint) before or with the frontend; the SPA degrades gracefully if the endpoint 404s (the real-match state simply fails to load, other states are unaffected).
