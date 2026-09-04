## Why

The header search box does not tell visitors what they can ask it for. An empty
box suggests nothing, a typo returns nothing, and the phrase people actually type
— `java developer`, `nodejs developer` — names no role in any dictionary we ship,
so it produces no suggestion at all even though the catalogue answers it well
(`q=java developer` returns 37,533 postings, `q=nodejs developer` returns 9,386).

The existing role dropdown was built against the right measurement — 37.6% of
searches name a role while the `role` facet appeared in 1.1% of requests — but it
can only ever offer roles, matched client-side against a shipped dictionary. The
gap it was built to close is wider than roles.

## What Changes

- A new Meilisearch index `suggestions` holding one document per offerable
  suggestion: mined job titles, roles, skills, categories and companies, each with
  its open-posting count.
- A new cron worker `cmd/build-suggestions` that mines titles from the catalogue,
  joins the dictionary vocabularies and their live facet counts, and writes the
  index through the existing rebuild swap.
- A new endpoint `GET /api/v1/suggest` serving completions, including
  **progressive phrase completion**: `senior software engineer go` is parsed into a
  recognised role part plus the fragment `go`, and completes to Google.
- Choosing a suggestion applies **every** part it names at once — a composed row
  sets the role facet and `company_slug` together, not one of the two.
- The header dropdown gains the launcher's job and company sections, with company
  logos, so typing `google` shows Google's actual postings rather than only the
  word.
- Search runs on Enter or on choosing a row. Typing no longer pushes into the list
  filter on every keystroke.
- **BREAKING (internal):** `web/src/lib/roleSuggest.ts` and its client-side
  matcher are retired. Meilisearch already does prefix matching, typo tolerance
  and ranking; keeping both would leave two matchers to drift apart.
- **BREAKING (behaviour):** choosing a suggestion no longer clears the typed text.
  Progressive completion requires the recognised prefix to stay in the box.
- A typo-only match IS now offered. The previous rule dropped it because ranking
  that tier by vacancy count put Marketing Specialist above Backend Engineer for
  `backedn`; the index ranks by relevance, so the reason no longer holds.
- A new table `search_queries` records what visitors ask for, and the builder
  ranks suggestions by that demand.

## Capabilities

### New Capabilities

- `search-suggestions`: the suggestions index and its contents, the offline
  builder that fills it, the `/api/v1/suggest` endpoint, progressive phrase
  parsing, and how a composed suggestion is applied.
- `search-query-frequency`: recording what visitors search for and using it to
  rank suggestions by real demand.

### Modified Capabilities

- `role-search-suggestions`: matching moves from the client dictionary to the
  suggestions endpoint; suggestions cover more than roles; a typo-only match is
  offered rather than dropped; choosing a suggestion keeps the typed text instead
  of clearing it; the dropdown gains job and company sections.

## Impact

- **New:** `internal/search/suggest` (block `search`, layer 6 — must be added to
  `internal/platform/arch/layering/blocks.go`), `cmd/build-suggestions`, migration
  `0125` for `search_queries`, a systemd unit and timer under `deploy/`.
- **Modified:** `internal/search/search` (the second index and its settings),
  `internal/api/handler` (the endpoint, its rate-limit bucket, the `q=` write
  path), `web/src/lib/components/HeaderListSearch.svelte`,
  `web/src/lib/components/JobsView.svelte` (the `roleSuggest` bridge),
  `web/static/openapi.yaml`.
- **Removed:** `web/src/lib/roleSuggest.ts` and its tests.
- **Operational:** Meilisearch runs one serial task queue, so the builder's timer
  must not overlap `freehire-reindexw`. `/suggest` is called per keystroke and
  needs its own rate-limit bucket — the incident where `c.IP()` returned empty and
  put the whole site in one bucket is the failure mode to avoid.
- **No change** to the `jobs` index settings, so none of the "declare the
  filterable attribute before the binary flips" hazard applies.
