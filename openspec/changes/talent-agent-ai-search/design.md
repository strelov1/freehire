## Context

The AI search today is `AiFilterDialog.svelte`: a small modal, opened from "Describe with AI" in the filter modal, that posts a description to `POST /api/v1/search/interpret` (`internal/search/searchintent`), shows the resolved values as preview chips and applies them through `FilterStore.apply`. `AiFilterButton` sends signed-out visitors to sign in. The interpretation endpoint is cookie-only, rate-limited 40/hour per user, and binds the model to the caller's own gateway credential under `plan.WithFreeAI`.

A spike on 2026-10-24 (`/Users/i_strelov/Projects/hire-talent-agent`, branch `spike/talent-agent`, uncommitted) rebuilt the modal as a describe → plan → results flow on our design system and against production data, and the owner accepted the layout. The spike proved the pipeline end to end — interpret → `filtersFromInterpretation` → `filtersToParams` → `/jobs/facets` + `/jobs/search` → `store.apply` — and left known gaps: companies counted from the first 100 hits, a hardcoded centroid table with no land shapes, a catalogue count on the request path, guests unable to reach either endpoint, no i18n or tests, and a dev-proxy cookie hack.

The owner decided: the entry point stays in the filter modal; guests get 10 interpretations per hour per IP; guests may attach a CV; attachments are not PII-masked (consistent with typed text).

## Goals / Non-Goals

**Goals:**
- Ship the spike's flow as the AI search, with the old dialog's honesty restored (preview chips, unresolved list, plan refusal link, start over on empty).
- Open interpretation and CV extraction to guests, bounded per IP, without ever minting a gateway credential for a non-account.
- Exact "companies hiring" and the top companies, with no Meilisearch settings change.
- A real map of where the jobs are.

**Non-Goals:**
- A "top match" ranked against the CV (the spike's blurred block was removed at the owner's request).
- Moving the entry point to the header.
- DOCX CVs (the existing extractor reads PDF only).
- A streaming, tool-calling agent: the agent is the existing one-shot interpreter, re-run per message with the previous plan.
- An expandable "agent thinking" trace: the interpreter produces no reasoning to show.

## Decisions

### D1. The modal is one component family under `filters/`, not one 400-line file
`AiFilterDialog.svelte` keeps the shell (portal, focus trap, Escape, stage indicator) and the conversation state. The overview moves to `AiSearchResults.svelte`, the map to `JobsWorldMap.svelte`, the plan card and chips to `AiSearchPlan.svelte`. The pure logic — building the CV context line, turning a plan into params, limiting countries to the requested ones, the preview-chip grouping (lifted from the old dialog) — lives in the existing AI-filter module `web/src/lib/aiFilter.ts` with unit tests, the pattern `web/AGENTS.md` sets (`matchAnalysis.ts`). The spike's review flagged the single file as Divergent Change; the split follows the seams it named.

Messages are a discriminated union on `kind` (`you` | `plan` | `found` | `note`) rather than three boolean flags.

### D2. The chat reuses the assistant's `Composer` and a shared `ThinkingIndicator`
The composer already carries dictation, sizing and submit handling; the spike used it unchanged. `Composer` gains a small exported `focus()` so the host does not reach into its DOM with `querySelector('form textarea')`. The thinking line moves out of `AssistantChat.svelte` into `assistant/ThinkingIndicator.svelte` (glyph, shimmering verb, elapsed seconds, timing itself from mount) and both use it — the spike already made this extraction and the assistant's tests stayed green.

### D3. Guests are interpreted on the service credential, never through `llmkey`
`/search/interpret` mounts `mw.optional` instead of `mw.cookie`. For a signed-in caller nothing changes; an API-key caller, refused before, now runs as that account on its own credential and per-user limit — the spend stays attributed to a person. A compressed request body is refused (415) on both guest-open routes, as `/cv/roast` already does, because the framework decompresses without a size cap. For a guest the handler builds the interpreter from the service client tagged `llm.Feature(tagSearchIntent)` directly — NOT via `llmBinding.bind(ctx, 0, …)`, because `llmkey.Resolver.For` would read, then **mint**, a gateway credential for account 0. A test asserts the guest path never calls the resolver.

Two limiters: the existing one keyed per user (40/h) applies when there is a user; a new guest limiter keyed per IP (10/h) applies when there is not. `ratelimit.KeyByUserOrIP` would collapse both into one budget and is not used for the split.

*Alternative considered:* a "guest" pseudo-account with its own credential — rejected: one shared key for every anonymous visitor is the service credential with extra steps, and it would show up as a user in spend attribution.

### D4. CV extraction opens the same way
`/me/resume/extract` mounts `mw.optional` with a per-IP guest limiter (20/h). It calls no model and stores nothing, so the cost is CPU for PDF parsing, already bounded by the global body limit. The route keeps its `/me/` path: renaming would break the onboarding wizard and profile callers for no behavioural gain.

### D5. `companies_total` is a second, concurrent Meili query
Measured during the spike (Meili 1.49, prod-equivalent settings): a search with `distinct: "company_slug"`, `page: 1`, `hitsPerPage: 0` returns an exact `totalHits` equal to the distinct company count — 2–6 ms for ~2k companies, 60–110 ms for ~29k, ~380 ms for the whole catalogue. Two traps, both covered by tests: `limit: 0` ignores `distinct` (use `page`/`hitsPerPage`), and `distinct` in the same request as `facets` collapses every facet count to 1 (two requests). The top 50 come from a `company_slug` facet in the facets query, already count-ordered (`SortFacetValuesBy {"*": count}`), so exact even when the 1,200 value cap truncates the tail. Display names are resolved for those 50 slugs from `companies` in one Postgres read. `company_slug` is already filterable, so no settings patch and no settings-before-binary rollout hazard. The result rides the existing one-minute facet cache.

*Alternatives considered:* raising `maxValuesPerFacet` (an index-wide setting that bloats every facet response); showing "1,200+" (honest but needless given D5 measures fine); a Postgres count (the filter lives in Meili).

### D6. Jobs searched reads `GET /api/v1/stats/catalog`
The spike called `/jobs/facets` with no filter to get a catalogue total — a count on the request path, which the catalogue-scale convention forbids. The overview reads the snapshot's `unique_open_jobs` (what `/jobs` itself shows unfiltered) and renders nothing for it when the snapshot is not `exact`.

### D7. The map draws real land shapes, precomputed once
`web/src/lib/worldMap.ts` holds the Natural Earth 110m land outline (public domain, via world-atlas) already projected equirectangularly into one SVG path in a 360x180 box (~56 KB), the label point of every country the `countries` facet can emit (Natural Earth 50m, the few small territories it omits filled in by hand), and `projectToMap`. A test walks the facet vocabulary so a code without a point fails rather than silently dropping off the map. No TopoJSON decoder or map library at runtime; the module is imported only by the map component, which renders only on the Results step. Bubbles are sized by √count.

*Alternatives considered:* shipping the TopoJSON and decoding it in the browser (a runtime dependency for data that never changes); a map library (MapLibre/Leaflet — a tile dependency for a static picture).

### D8. All copy goes through `.messages.ts`
The dialog and its pieces get `AiFilterDialog.messages.ts` etc., matching ~60 sibling components including `Composer`.

## Risks / Trade-offs

- [Bots spend the service credential through guest interpretations] → 10/h per IP, Cloudflare in front, and the call is a short one on the cheap `SEARCH_INTENT_MODEL`; spend is visible under the search-intent feature tag.
- [The results step makes three requests per run (facets, distinct count, search snapshot)] → the facets response is cached for a minute and the distinct query costs tens of milliseconds for typical plans; measured worst case ~380 ms for an unfiltered plan, which the interpreter never produces.
- [Overlap with PR #3510 in `AssistantChat.svelte`] → the ThinkingIndicator extraction is a few lines; whichever PR lands second rebases.
- [Interpretation quality: a summary that names a place the facets dropped] → the restored preview chips make the applied filters visible before Run, which is the guard the old dialog had.

## Migration Plan

1. Backend first (D3, D4, D5) behind no flag — every change is additive or widens access; the old dialog keeps working against them.
2. Web: the new modal replaces the old one in one deploy.
3. Rollback: revert the web deploy restores the old dialog; reverting the backend restores cookie-only access. No migrations, no Meili settings.

## Open Questions

None blocking. The guest limits (10/h interpret, 20/h extract) are the owner's and our first guesses; revisit with the spend figures after a week.
