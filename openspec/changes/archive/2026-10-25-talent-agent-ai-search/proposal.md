## Why

The AI search today is a small "Describe your search" dialog that only signed-in users can open: it turns a sentence into filter chips and stops there. It never shows what the search will find before the filters are applied, and it closes its door on exactly the visitors who have not yet been given a reason to sign up. A spike (2026-10-24, branch `spike/talent-agent`) validated a richer flow on our own design system and real data: a conversation that builds a search plan, then an overview of what that plan finds (how many jobs, where they are, who is hiring), and only then applies it to the list.

## What Changes

- Replace the content of the AI search modal (`AiFilterDialog`, opened from "Describe with AI" in All filters — the entry point does not move) with a three-step agent flow:
  1. **Describe** — a free-text description, an optional CV (PDF) and example searches.
  2. **Search plan** — a chat in which the agent states the plan it built, the filters it will apply (the applied-filter preview chips the current dialog shows, restored), what it could not place, and a card offering **Run search** or **Keep chatting**. Follow-up messages refine the plan.
  3. **Results** — jobs searched, jobs found, companies hiring, a world map of where the jobs are, a grid of the top hiring companies' logos, the chat beside it for further refinement, and **Show N jobs**, which applies the plan to the job list exactly as the current dialog's Apply does.
- The "agent is working" state reuses the assistant's thinking animation, extracted into a shared `ThinkingIndicator` (the assistant switches to it too).
- **Guests may use the AI search.** `POST /api/v1/search/interpret` admits signed-out callers on the service LLM credential, limited to 10 interpretations per hour per IP; signed-in callers keep their own credential and 40 per hour.
- **Guests may attach a CV.** `POST /api/v1/me/resume/extract` — deterministic, stores nothing, calls no model — admits signed-out callers under a per-IP limit. **BREAKING** for the spec only: the "unauthenticated request is rejected" scenario no longer holds.
- `GET /api/v1/jobs/facets` can report the exact number of distinct companies matching a filter, plus the top companies by job count, when asked.
- "Jobs searched" reads the published catalogue snapshot, never a count on the request path.

## Capabilities

### New Capabilities
- `ai-search-agent`: the AI search modal's describe → plan → results flow, its applied-filter preview, refinement in chat, the results overview, and applying the plan to the job list — including guest access to search interpretation.
- `job-facet-company-count`: an opt-in exact distinct-company count and top-companies distribution on the job facets endpoint.

### Modified Capabilities
- `resume-skill-extraction`: the extraction endpoint admits signed-out callers under a per-IP limit instead of rejecting them.

## Impact

- **Web**: `web/src/lib/components/filters/AiFilterDialog.svelte` (rewritten), `AiFilterButton.svelte` (no sign-in gate), new results/map pieces beside it, `web/src/lib/assistant/ThinkingIndicator.svelte` (new, shared) and `AssistantChat.svelte` (uses it), `api.ts`/`types.ts` for the new facet fields. A static world land-shape asset for the map.
- **API**: `internal/api/handler/search_intent.go` (optional auth, per-IP limiter for guests), `internal/api/handler/resume.go` (optional auth on `/me/resume/extract`, per-IP limiter), `internal/api/handler/facets.go` and `internal/search/search/facets.go` (company count + top companies), `web/static/openapi.yaml`.
- **Cost**: guest interpretations spend the service LLM credential; bounded by the per-IP limit.
- **No Meilisearch settings change**: `company_slug` is already filterable, and facet values are already ordered by count.
- Overlaps PR #3510 (PDF attachments in the assistant) in `AssistantChat.svelte`; whichever lands second rebases.
