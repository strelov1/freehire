## 1. Guest access to search interpretation (backend)

- [x] 1.1 Test: a signed-out `POST /search/interpret` is answered on the service client tagged with the search-intent feature, and the `llmkey` resolver is never called (no credential read or minted for account 0)
- [x] 1.2 Test: signed-out callers are limited to 10/hour per IP (11th → 429); signed-in callers keep 40/hour per user, and the two budgets do not share a key
- [x] 1.3 Mount `/search/interpret` on `mw.optional`; build the guest interpreter from the service client directly (design D3); add the per-IP guest limiter beside the per-user one
- [x] 1.4 Update the handler's register comment and `internal/api/handler/AGENTS.md` for the guest path (not added to `web/static/openapi.yaml`: that file is the public read-only search API imported as the custom GPT's Action, and listing interpret there would let the GPT spend the service credential)

## 2. Guest access to CV extraction (backend)

- [x] 2.1 Test: a signed-out multipart PDF to `/me/resume/extract` returns 200 with skills and persists nothing; a signed-in call is unchanged
- [x] 2.2 Test: signed-out callers are limited to 20/hour per IP (21st → 429)
- [x] 2.3 Mount `/me/resume/extract` on `mw.optional` with the per-IP guest limiter (likewise kept out of the public `openapi.yaml`)

## 3. Exact distinct-company count on job facets (backend)

- [x] 3.1 Test (integration, Meili): `DistinctCount` with `distinct: company_slug`, `page: 1`, `hitsPerPage: 0` equals the true distinct count, including above the 1,200 facet cap; a `limit: 0` request is NOT used
- [x] 3.2 Implement `search.Client.DistinctCount(ctx, query, filter, attr)` in `internal/search/search/facets.go`; add `Companies` to `FacetResult` so the one-minute facet cache covers it
- [x] 3.3 Test (handler): `facets=company_slug` returns `companies_total` and a top-50, count-ordered `company_slug` distribution with display names; absent → no extra search; `disjunctive=1&facets=company_slug` → 400
- [x] 3.4 Accept `company_slug` in `requestedFacets` (not in the default set), run the facet query and the distinct count concurrently (two requests, never `distinct` beside `facets`), trim to 50, resolve display names for those slugs from `companies` in one read
- [x] 3.5 Add `companies_total` and the named distribution to `facetPayload`/`facetView` and `openapi.yaml`; extend web `FacetCounts` type and `api.facetCounts`

## 4. Shared thinking indicator (web)

- [x] 4.1 Add `web/src/lib/assistant/ThinkingIndicator.svelte` (glyph, shimmering verb, elapsed seconds; `label` replaces the verb and hides the counter) and switch `AssistantChat.svelte`'s turn indicator to it; assistant vitest stays green
- [x] 4.2 Expose a `focus()` on `Composer.svelte` for hosts that need to focus the field

## 5. AI search logic module (web)

- [x] 5.1 Add tested pure helpers to the existing AI-filter module `web/src/lib/aiFilter.ts`: `planParams` (interpretation → job-list params), `cvContext` (CV profile → appended description line), `limitToRequestedCountries`, and the preview-chip grouping lifted from the old dialog (included, excluded struck, query, salary, posted, experience, visa — shared labels)
- [x] 5.2 Unit tests for each helper in `aiFilter.test.ts`, including "include wins over exclude" and chips keyed `param:value`

## 6. AI search modal (web)

- [x] 6.1 Rewrite `AiFilterDialog.svelte` as the shell: portal, focus trap, Escape, backdrop, Describe / Search plan / Results stage indicator, conversation state with a `kind`-discriminated message union; keep the `store` / `onclose` / `onApplied` contract
- [x] 6.2 Describe step: textarea, example searches, PDF CV attachment through `api.extractResumeProfile` (file chip with seniority + skill count, remove, error); Enter and the send button share one submit path with the "Find jobs that fit my CV" fallback
- [x] 6.3 `AiSearchPlan.svelte`: summary, preview chips, not-recognised list, Run search / Keep chatting card that stays on the latest plan after Keep chatting; Keep chatting adds the agent's prompt and focuses the composer
- [x] 6.4 Honest failures: empty interpretation → explanation + Start over (no plan card); plan refusal → message with `PlanLimitLink`; 429 for a guest → sign-in invitation; any other error → chat message, working state cleared
- [x] 6.5 Chat: `Composer` bound to a host-cleared draft, `ThinkingIndicator` while working, refinement sends the previous plan, refinement from Results re-runs the overview
- [x] 6.6 `AiSearchResults.svelte`: jobs searched from `GET /stats/catalog` (`unique_open_jobs`, nothing when not exact), jobs found, `companies_total` and `top_companies` from one `facetCounts` call with `facets=countries,company_slug`, top-company logo grid via `EntityLogo` + `companyLogoUrl`, Show N jobs → `store.apply(planParams(plan))`, then close and `onApplied`
- [x] 6.7 `JobsWorldMap.svelte`: land outline and country points precomputed in `web/src/lib/worldMap.ts` (tested against the countries facet vocabulary), equirectangular SVG, √count bubbles, top countries as chips, limited to the plan's countries when it names any; loaded only on the Results step
- [x] 6.8 Layout: the Results chat is reachable at every width the Results step renders (side panel on wide screens, a toggleable panel below that); check mobile
- [x] 6.9 `AiFilterButton.svelte`: open for everyone, correct its header comment
- [x] 6.10 `.messages.ts` for the dialog and each new component; no hardcoded copy
- [x] 6.11 Component specs for the dialog: guest opens it; plan shows chips; Keep chatting keeps Run; empty interpretation shows Start over; Show N jobs calls `store.apply` and `onApplied`

## 7. Verification and rollout

- [x] 7.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`; integration tests for the touched handlers and the Meili distinct count
- [x] 7.2 `web`: svelte-check 0 errors, eslint/oxlint clean, vitest green; `pnpm check:dead` (knip) clean for new exports
- [ ] 7.3 Drive the flow in a browser as a guest and as a signed-in user: describe → plan → refine → run → Show N jobs; CV attach; map limited to the requested country; companies_total matches
- [ ] 7.4 PR → CI → merge → deploy → verify on prod as a guest; watch search-intent spend and 429s for a day
