## Why

`/my/profile` and `/my/market-pulse` are both "how do I compare to the market"
views on the candidate, but split across two unrelated pages. Consolidating the
comparison-role verdict (currently Profile's **Market coverage** tab) onto the
Market Pulse page — next to the existing per-skill demand trend — gives the
candidate one place for market intelligence, and lets Profile focus on
identity/settings/CV.

## What Changes

- `/my/market-pulse` becomes a tabbed page: **Coverage** (default) and **Skill
  trend**, using the same `TabRow` pattern Profile already uses.
- The Coverage tab carries everything the old Profile `coverage` tab had: the
  role/region/seniority filter (summary sidebar, mobile edge tab, two-pane
  modal), the verdict (coverage percent, gap skills with their unlock counts).
  When the caller has no profile yet, it shows an empty state pointing to
  `/my/profile` instead of computing anything.
- The Skill trend tab carries the existing `MarketPulseView` content unchanged
  (search, card grid, sparklines) — only its own page-level heading and
  auth-gate are removed, since the host page now owns both.
- **BREAKING (UI only, no API change)**: `/my/profile` loses its **Market
  coverage** tab. Five tabs remain: Settings, Skills, Profile, Experience, CV
  readiness.
- The filter sidebar/edge-tab/modal that used to gate on Profile's `coverage`
  tab now gates on the **CV readiness** tab instead of disappearing — CV
  readiness's Keyword Strength scoring already reads the same comparison-role
  filter the verdict did (`internal/atscheck`), so removing the only way to
  change that filter from Profile would silently freeze it at the profile's
  default specializations.

## Capabilities

### New Capabilities

- `market-pulse`: the signed-in user's personalized market page — a Coverage
  tab (role/region/seniority-filtered verdict against the live market) and a
  Skill trend tab (weekly demand history for the user's own profile skills).
  Coalesces the previously-unspecified `/my/market-pulse` page (shipped by the
  `market-pulse-skill-trend` change, never synced to `openspec/specs/`) with
  the coverage view moved in from Profile.

### Modified Capabilities

- `web-frontend`: the requirement "Profile filters appear only on the Market
  coverage tab" changes — Profile no longer has a Market coverage tab, and the
  filter controls now gate on the CV readiness tab (which becomes filterable,
  reversing the old "scored against the profile's default role" constraint).
- `cv-ats-score`: the "Role keyword-match distinct from market-coverage"
  requirement's parenthetical source of the role filter ("the verdict page's
  filter") is stale — it now comes from Profile's CV readiness tab filter.

## Impact

- Frontend only: `web/src/routes/my/market-pulse/+page.svelte`,
  `web/src/lib/components/MarketPulseView.svelte`,
  `web/src/routes/my/profile/+page.svelte`.
- No backend/API/database changes — reuses `getProfileVerdict`, `facetCounts`,
  `getATSReport`, and `marketPulse` exactly as they're called today.
- `/my/market-pulse/[skill]` and `accountNav` are untouched.
