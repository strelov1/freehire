## Context

`/my/profile/+page.svelte` currently owns six tabs (Settings, Skills, Profile,
Experience, **Market coverage**, CV readiness) and, alongside them, a
`FilterStore` + verdict/counts/ats fetch cycle shared across tabs. The
**Market coverage** tab renders `VerdictView` against `getProfileVerdict`, and
exposes the filter sidebar/edge-tab/modal only while active.

`/my/market-pulse/+page.svelte` is a separate, single-purpose page rendering
`MarketPulseView.svelte` — a card grid of the caller's profile skills with
weekly demand sparklines, fed by `GET /me/market-pulse`. It owns its own
`<h1>`/subtitle and its own `isAuthenticated()` gate.

This change moves the Coverage view onto the Market Pulse page as a second
tab, and removes it from Profile — decided via a brainstorming session with
the user (approved design, see prior conversation; this document formalizes
it for the OpenSpec change).

## Goals / Non-Goals

**Goals:**
- One page for "how do I compare to the live market" (Coverage + Skill trend).
- Preserve CV readiness's ability to compare against a caller-chosen role —
  the filter UI moves tabs within Profile rather than disappearing.
- No behavior change to any endpoint; this is a component/route reshuffle.

**Non-Goals:**
- Deep-linking the active tab via a query param on either page (Profile's
  existing `TabRow` doesn't do this either — no new precedent needed).
- Restoring the originating tab when following `/my/market-pulse/[skill]`'s
  back link (it always lands on the page's default tab, Coverage).
- Renaming `accountNav`'s "Market Pulse" entry or touching `[skill]`.
- Any backend change — `getProfileVerdict`, `facetCounts`, `getATSReport`,
  `marketPulse` are called exactly as today, just from a different component.

## Decisions

### Coverage becomes the Market Pulse page's default tab, not Skill trend

Coverage is model-heavy (role/region/seniority comparison); Skill trend is a
lighter, glanceable trend view. Leading with Coverage matches how Profile
today leads with its most substantive market view. Considered leading with
Skill trend (preserves today's `/my/market-pulse` first-paint exactly) —
rejected because the user, presented with both mockups, chose Coverage-first.

### Filter sidebar/modal/edge-tab move from Profile's `coverage` tab gate to Profile's `readiness` tab gate — not removed

`internal/atscheck`'s Keyword Strength category (40/100 points) scores against
"the role's top in-demand skills," using the same `filters.applied` params
`reload()` already threads through to `getATSReport`. Two options were
weighed:
1. **Keep the sidebar, re-gate onto `readiness`** (chosen) — CV readiness
   keeps its existing ability to compare against a non-default role; the only
   change is which tab shows the controls.
2. **Drop the sidebar entirely** — simpler (one component that only exists on
   Market Pulse now), but silently regresses CV readiness to always scoring
   against the profile's default specializations, with no way to change it
   short of leaving Profile. Rejected — a silent capability loss is worse than
   carrying the sidebar one tab over.

### `/my/market-pulse/[skill]`'s back link stays a plain `href`, no tab restoration

Since Coverage is now the default, following "← Market pulse" back from a
skill's detail page lands on Coverage rather than Skill trend (where the user
came from). Considered threading `?tab=trend` through the back link and
seeding the page's initial tab from `page.url.searchParams` once (read-only,
not a live sync — so the shallow-routing staleness issue that affects
`FilterStore`'s two-way URL sync doesn't apply here). Rejected per explicit
user choice: the mis-landing is minor and not worth the added surface.

### `MarketPulseView.svelte` loses its own heading and auth gate

Both tabs now share one page-level `<h1>Market pulse</h1>` + a tab-neutral
subtitle, and one `isAuthenticated()` check above the `TabRow`. Keeping a
second `<h1>` inside the Skill trend tab body would double the heading and
read wrong under a tab strip that already labels the section.

## Risks / Trade-offs

- **[Risk]** `openspec/specs/web-frontend/spec.md`'s "Profile filters appear
  only on the Market coverage tab" requirement is written for the old
  layout and will read as violated in the delta view until archived. →
  Mitigation: this change ships a MODIFIED delta for that exact requirement
  (see `specs/web-frontend/spec.md` in this change).
- **[Risk]** `cv-ats-score`'s "Role keyword-match distinct from
  market-coverage" requirement parenthetically cites "the verdict page's
  filter" as the role source — stale once Profile has no verdict page. →
  Mitigation: MODIFIED delta updates the parenthetical to name the CV
  readiness tab's filter instead; the normative behavior (role from request
  facet params) is unchanged.
- **[Trade-off]** Skill-detail back-navigation can land on a different tab
  than the user started from (see Decisions above) — accepted, not mitigated.

## Migration Plan

Pure frontend change behind no feature flag — ships in one PR/deploy. No data
migration, no backend deploy ordering concerns. Rollback is a plain revert of
the frontend commit(s).

## Open Questions

None outstanding — scope, tab order, tab naming, filter placement, and the
back-link trade-off were all resolved in the preceding brainstorming session.
