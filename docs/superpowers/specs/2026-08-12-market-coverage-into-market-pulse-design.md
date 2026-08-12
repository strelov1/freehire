# Move "Market coverage" from Profile into Market Pulse

## Context

`/my/profile` currently has a `coverage` tab rendering `VerdictView` (a verdict of how
the candidate's profile compares against a chosen role/region/seniority, driven by a
`FilterStore` + a right-hand filter sidebar/modal/edge-tab). `/my/market-pulse` is a
separate account-nav section (`MarketPulseView.svelte`) showing per-skill demand
sparklines for the candidate's own profile skills.

Both are "how do I compare to the market" views on the candidate. This change merges
them onto one page — `/my/market-pulse` gains two tabs, Profile loses one.

## Scope

- `/my/market-pulse`: becomes a tabbed page — **Coverage** (default) and **Skill
  trend** — reusing the same `TabRow` pattern already used on `/my/profile`.
- `/my/profile`: the `coverage` tab and `VerdictView` usage are removed. Five tabs
  remain: Settings, Skills, Profile, Experience, CV readiness.
- No change to `/my/market-pulse/[skill]`, to `accountNav`, or to any backend/API
  surface — this is a pure frontend reshuffle of existing components and existing
  endpoints (`getProfileVerdict`, `facetCounts`, `marketPulse`).

## Key finding: filters aren't Coverage-only

`internal/atscheck`'s Keyword Strength category (40 of 100 ATS-readiness points) scores
the CV against "the role's top in-demand skills" — the same comparison role/region/
seniority the Coverage filters set. Profile's `reload()` already fetches `ats` using the
same `filters.applied` params used for `verdict`. So the filter sidebar/modal/edge-tab
must **stay in Profile**, just re-gated onto the `readiness` tab instead of `coverage` —
otherwise CV readiness's keyword comparison silently freezes on the profile's default
specializations with no way to change it from Profile.

## Component changes

### `web/src/routes/my/market-pulse/+page.svelte`

Becomes the owner of:
- Page header: `<h1>Market pulse</h1>` + a subtitle general enough for both tabs (not
  the current skill-trend-only copy).
- `TabRow` with two tabs, ids `coverage` (default) and `trend`, labelled "Coverage" /
  "Skill trend".
- The filter/verdict state and logic moved from `profile/+page.svelte`: `FilterStore`
  construction (`buildFilters`, seeded from `profileStore.profile.specializations`),
  `verdict`, `counts`, `modalOpen`, `previewCount`, `gapHref`, `loadError`, and a
  `reload()` that calls `getProfileVerdict` + `facetCounts` (no `ats` — that stays in
  Profile).
- `profileStore.ensureLoaded()` on mount (same effect shape as Profile's).
- Coverage tab body: `VerdictView` when a profile exists; when `profileStore.profile`
  is `null`, an empty state ("Complete your profile to see market coverage" + CTA to
  `/my/profile`), matching the tone of Skill trend's existing empty state.
- The filter aside + `FilterEdgeTab` + `FilterModal`, gated on `tab === 'coverage'`
  (moved wholesale from Profile, same gating condition, just now the page's own tab).
- Skill trend tab body: `<MarketPulseView />`.
- The `isAuthenticated()` gate currently inside `MarketPulseView` moves up to this page
  (shared by both tabs).

### `web/src/lib/components/MarketPulseView.svelte`

Drops its own `<h1>`/subtitle and the `isAuthenticated()` check (now the caller's
job). Keeps everything else: the skill search input, the card grid, loading/error/
empty states.

### `web/src/routes/my/profile/+page.svelte`

- `TABS` drops the `coverage` entry; the `tab` union type drops `'coverage'`.
- `verdict` state and the `VerdictView` import/usage are removed.
- `reload()` drops the `getProfileVerdict` call; `loadError` is now driven by whether
  `ats`/`counts` fetches settle rather than `verdict`.
- `gapHref` is removed (it only served the coverage tab's gap-skill links).
- The filter aside + `FilterEdgeTab` + `FilterModal` block's gate changes from
  `tab === 'coverage'` to `tab === 'readiness'`.
- `FilterSummary`'s description copy is reworded to describe its readiness purpose
  (comparing CV keyword strength against a chosen role/region/seniority) instead of
  the old coverage framing.

## Explicitly out of scope / accepted trade-offs

- `/my/market-pulse/[skill]`'s "← Market pulse" back link is untouched. Since Coverage
  is now the default tab, following it back from a skill-detail page lands on Coverage
  rather than Skill trend (where the user came from). Accepted as-is — no query-param
  plumbing to restore the originating tab.
- No URL/query-param sync for the new page's active tab, consistent with how Profile's
  own `TabRow` already works (plain `$state`, not deep-linkable).
- `accountNav`'s "Market Pulse" label and beta pill are unchanged.

## Testing

- `go build ./...`, `go vet ./...` are unaffected (frontend-only change).
- Manual verification in a running dev server: Profile page shows 5 tabs with no
  Coverage; CV readiness's filter sidebar still reads/writes the comparison role;
  `/my/market-pulse` shows Coverage by default with working filters, and Skill trend
  tab renders the existing card grid unchanged; `/my/market-pulse/[skill]` still loads
  and its back link returns to the page.
