## 1. Stage 1 — the dropdown's behaviour (frontend only, no Go)

- [x] 1.1 Stop the header input from filtering the list on every keystroke: hold the typed text locally in `HeaderListSearch.svelte` and commit it to the list store only on Enter or on choosing a row
- [x] 1.2 Open the dropdown on focus with an empty query, showing the category suggestions in `CATEGORY_GROUP_ORDER`; drop the two-character minimum
- [x] 1.3 Add the postings and companies sections to the dropdown, reusing `HeaderSearch.svelte`'s data calls and its `EntityLogo`/`companyLogoUrl` row rendering rather than writing a second one
- [x] 1.4 Make the keyboard highlight run continuously across all three sections, and cap each section (5 completions, 5 postings, 3 companies)
- [x] 1.5 Verify stage 1 in the browser on `/`: empty focus, typing without refetch, Enter, arrow-through, Escape, click-away

## 2. Stage 2 — the suggestions package

- [x] 2.1 Create `internal/search/suggest` and register it in the `search` block in `internal/platform/arch/layering/blocks.go`; assert `go vet ./...` and the layering test pass on the empty package
- [x] 2.2 Implement title normalisation (lowercase, collapse whitespace, cut at the first separator) as a pure exported function, with the mined-title and typed-query cases that must land on the same key
- [x] 2.3 Implement the drop rule for titles reducing to a bare grade or bare generic, driven by `vocab.SeniorityValues` plus the generic list
- [x] 2.4 Implement the suggestion document type and the builder's assembly: mined titles above the floor, roles, skills, categories, companies — each with its open-posting count
- [x] 2.5 Implement the category-vs-role de-duplication (a category sharing a role's slug is not emitted) and the one-row-per-base-role collapse
- [x] 2.6 Implement the index settings and the rebuild-and-swap write, reusing `search.Rebuild` rather than a second swap implementation

## 3. Stage 2 — the builder worker

- [x] 3.1 Add `cmd/build-suggestions` on the `worker.Main`/`worker.Bootstrap` shape of `cmd/rollup-facets`, taking the floors as env with documented defaults
- [x] 3.2 Run it against production data, inspect the resulting dictionary, and set the title floor and company floor from what it shows
- [x] 3.3 Add the systemd unit and timer under `deploy/`, scheduled so it cannot overlap `freehire-reindexw`, and record that it must be copied to the host

## 4. Stage 2 — the endpoint

- [x] 4.1 Implement the in-process phrase set: load the normalised phrases from the index at startup, refresh on a ticker, expose the exact longest-prefix parse
- [x] 4.2 Implement the fragment query against the index, excluding the kinds the prefix already filled (roles and companies once, skills unbounded)
- [x] 4.3 Implement ranking — relevance, then recorded demand, then open-posting count, then shorter text — and withhold any suggestion whose count is zero
- [x] 4.4 Implement the empty-`q` response: the curated category order, never the highest-count values
- [x] 4.5 Wire `GET /api/v1/suggest` in `internal/api/handler` with the standard list response shape, and give it its own rate-limit bucket
- [x] 4.6 Document the endpoint in `web/static/openapi.yaml`

## 5. Stage 2 — the client moves onto the endpoint

- [x] 5.1 Point the dropdown's completions at `/api/v1/suggest` with a debounce and a stale-response token; cache the empty-state response for the session
- [x] 5.2 Apply every part a chosen suggestion names — role plus `company_slug` together — and keep the typed text instead of clearing it. **Two halves that must flip together**: `facetModel.filtersWithRole` sets `q: ''`, and `HeaderListSearch.choose` clears the draft to match it. Either one alone leaves the box and the list disagreeing about what is being searched
- [x] 5.3 Apply a `title` suggestion as the free-text query rather than a facet
- [x] 5.4 Delete `web/src/lib/roleSuggest.ts`, its tests, and the now-unused `roleSuggest` bridge in `JobsView.svelte`/`listSearch.svelte.ts`; confirm `pnpm check:dead` stays clean
- [x] 5.5 Verify in the browser: `java developer`, `nodejs developer`, `backedn`, `senior software engineer go`, `google`

## 6. Stage 3 — search frequency

- [x] 6.1 Add migration `0125` creating `search_queries` (normalised text key, count, last seen) and the sqlc query; run `make sqlc`
- [x] 6.2 Upsert the normalised query on every search carrying a non-empty `q`, failing open so a write error never delays or fails the response, and storing no identifier
- [x] 6.3 Join the recorded counts into the builder's documents, writing zero for a suggestion nobody has searched
- [x] 6.4 Confirm the endpoint's demand-first ranking now reorders suggestions, and that an unsearched suggestion is still offered

## 7. Finish

- [x] 7.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...`
- [x] 7.2 Update `internal/search/AGENTS.md` with the new package, and the root `CLAUDE.md` worker list with `cmd/build-suggestions` and its env
- [x] 7.3 Verify the whole flow on production data, then finish the branch

## Shipped

On production 2026-09-03. Dictionary: **77,203 suggestions** at the default floors
(title 25, company 5), which is the size the mining sample predicted.

Six follow-up fixes, every one found by running against production rather than by a
test — recorded because each names a thing tests structurally could not see:

1. **#2362** — the endpoint answered 500 before the first build. A dictionary that
   does not exist yet is "no suggestions", not a fault, and the box asks once per
   keystroke.
2. **#2368** — the builder asked for facet `role`; the index attribute is `roles`.
   The names now come from `search.StringFacets`, the table the filter builder
   translates through.
3. **#2370** — `company:01-tech` is an illegal document id. Writing the test found
   more than production did: `node.js`, `c++` and `ci-cd` all carry characters an id
   may not, so no prefix-plus-value scheme survives this vocabulary.
4. **#2372** — hex made ids legal but PROPORTIONAL, and a 340-byte transliterated
   company slug doubled past the engine's 511-byte ceiling. Fixed-width hash. Bounding
   what we mine could never have covered a slug that arrives from a feed.
5. **#2375, #2379** — the parse consumed the word still being typed. `senior software
   engineer go` ate `go` as the skill; then `java dev` ate `java` as a title. Two
   rules: a word is recognisable only once a space follows it, and a prefix of a
   longer known phrase is still being typed.
6. **#2382** — the index inherited Meilisearch's default `maxTotalHits` of **1000**,
   so the recognition set held 1,000 phrases of 77,203. The symptom read as a parsing
   bug (`java dev` worked, `nodejs dev` did not) because the cut fell along posting
   counts. The ceiling and the request are now one constant.

Verified on production: `product own` → Product Owner (2,738); `java dev` → Java
Developer (5,473); `nodejs dev` → Nodejs Developer; `backedn` → Backend Engineer
(26,519); `senior software engineer go` → Senior Software Engineer · Go;
`product owner goo` → **Product Owner · Google** (3,400), applying the title and the
company together.

Two operational notes worth keeping:

- `deploy/bin/release.sh` does not deploy itself. The new worker was in the repo copy
  and not in `/opt/freehire/bin/release.sh`, so the binary was never built.
- A dictionary rebuild does not reach a running API for up to an hour (the refresh
  ticker). After a build that changes index SETTINGS, restart the live colour.
