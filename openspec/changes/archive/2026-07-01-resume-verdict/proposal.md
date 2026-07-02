## Why

A search profile already tells a user which skills they have and a one-line "Market fit" ratio, but it does not tell them *how they stack up against the live market for their target role* or *what to add first for the biggest gain*. Users want a verdict: a clear read on their coverage of the most in-demand skills, the highest-leverage gaps, and whether their résumé actually backs up the skills it claims.

## What Changes

- **New verdict screen** at `/my/profiles/[id]/verdict`: Stack Match %, must-haves covered X/Y, a top-20 market-skill breakdown (each Covered or a Gap carrying `+N% roles`), "Biggest wins" (top gaps by unlock), and "Your superpowers" chips. Reuses the already-built `VerdictView.svelte` (currently mock data → refactored to a real `Verdict` prop).
- **Deterministic core computed live** from the profile's saved `skills` + `specializations` against the `/jobs` facet distribution (mirrors `web/src/lib/skillGap.ts`). No résumé needed for this part.
- **Optional AI layer**: the user uploads a résumé (PDF or text) on the verdict page; the server reads the text *once, in that request* and asks the LLM for a Coherence Score (0-100 — are the claimed skills backed by Experience?) plus short advice for each must-have gap.
- **Privacy invariant preserved**: the raw résumé text is NEVER persisted (same as `internal/handler/resume.go`). Only the derived AI layer (coherence + advice + `analyzed_at`) is stored, so it survives reload.
- **Graceful degradation**: the LLM client is built on the HTTP server only when `LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL` are set (mirrors the Meilisearch nil-guard). When absent or on LLM error, the verdict still renders deterministically with the coherence card hidden.
- **Cleanup**: remove the throwaway preview route `web/src/routes/verdict-preview/+page.svelte`.

## Capabilities

### New Capabilities
- `resume-verdict`: computing and serving a per-profile résumé verdict — the deterministic market skill-gap scoring (stack match, must-haves, per-gap unlock) plus the optional, privacy-preserving LLM coherence score and gap advice, persisted as a derived analysis on the profile.

### Modified Capabilities
<!-- None: the verdict is a new capability. The search_profiles table gains a derived
     resume_analysis column, but the search-profiles capability's own requirements
     (create/list/update/delete a profile) are unchanged. -->

## Impact

- **New backend package** `internal/verdict` (pure `Compute` + LLM `Analyzer`).
- **Handler**: `GET`/`POST /api/v1/me/profiles/:id/verdict` in `internal/handler` (RequireAuth, ownership-checked); reuses résumé parsing from `internal/handler/resume.go`.
- **Config + `cmd/server`**: build an `internal/llm` client when the `LLM_*` env is set; pass into `handler.Config` (nil disables the AI layer).
- **DB**: migration adds `resume_analysis JSONB` (nullable) to `search_profiles`; new sqlc queries (fetch profile by id+user, set analysis).
- **Contracts**: `cmd/gen-contracts` emits the `Verdict`/`Skill` TS types.
- **Frontend**: new route `/my/profiles/[id]/verdict`, `api.ts` calls, `VerdictView.svelte` prop refactor, a "Verdict" link on the profile card. No web test runner — verify via `svelte-check`.
- **Ops**: the migration needs a manual apply on existing/prod volumes (the open versioned-migration-runner seam), applied before the new server binary rolls.
