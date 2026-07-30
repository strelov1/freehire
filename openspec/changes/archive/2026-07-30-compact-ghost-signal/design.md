## Context

`web/src/lib/ghost.ts` is the whole presentation layer for the signal: `ghostBadge` projects the
served `Ghost` into a chip, `ghostChecklist` into four rows, `supersedesReality` decides whether the
reality badge yields. Three components consume it — `GhostBadge.svelte` (the chip, used by `JobRow`
and by `JobView`'s header), `GhostChecklist.svelte` (the bordered panel before the description), and
`JobView.svelte`, which renders both.

The served payload carries only the criteria that *fired* (`criteria: string[]`, `criteria_total`,
plus `contributors?` and `ats_checked_at?`). Nothing in it separates "we checked this criterion and it
was clear" from "we have no data on this criterion" — `detailFor` collapsed both into the string
`No data`, which was false for `evergreen_posting` (derived from a reality class computed for every
job) and for an `ats_absent` that did not fire because the role IS on the employer's board. That gap
is the binding constraint on both the gauge's colour rules and the summary line's wording.

`web/src/lib/ghost.test.ts` covers the projections as plain functions. The web workspace has no
component-render harness (no `@testing-library/svelte`), so component behaviour is verified visually
against a running dev server; logic that deserves a test has to live in `ghost.ts`.

## Goals / Non-Goals

**Goals:**

- One statement of the signal on the job page, one line tall when collapsed.
- The scale readable pre-attentively — two-of-four distinguishable from four-of-four without reading.
- The full justification still reachable, one click away, with its hedging intact and the caveat one
  further click away on the page that explains it.
- The gauge's colour vocabulary honest about what the payload does and does not know.

**Non-Goals:**

- No backend, contract, or `Ghost` shape change. This is presentation only.
- No change to list cards. `GhostBadge` and `JobRow` are untouched.
- No change to `RealityBadge` or the supersede rule.
- No new dependency, and no promotion of the gauge into `design-system` — one call site does not
  justify a shared primitive yet.

## Decisions

**The gauge fills with risk, not health.** Segments colour as criteria fire; the tone escalates with
the count. The alternative — a battery draining from full green — is the more familiar metaphor and
was rejected twice over. First, green segments would assert that the remaining criteria were checked
and found clear, which the payload cannot support. Second, the "healthy" end of that scale is
unreachable: at level `none` the field is absent and nothing renders at all, so a full green battery
would be a state the interface can never show.

**The tone escalates on the fired count, not on `level`.** `level` has two values, and the gauge has
four states worth distinguishing; keying tone to `level` would render one-of-four and two-of-four
identically. The projection therefore derives tone from `criteria.length`, and `level` continues to
drive only the wording. This is a deliberate second reading of the same data, and the wording stays
authoritative: a three-of-four gauge in a strong tone still says "Possibly inactive" if that is the
served level.

**Uncoloured segments use a neutral grey.** Not green, not a lighter shade of the fired colour. The
segment reads as "nothing observed here", which is exactly what the payload means. It is
`muted-foreground/30` rather than the border tone: on the dark theme the border is nearly the
background, and a gauge whose unfilled segments are invisible reads as "one of one" — losing the very
denominator that caps the claim.

**Disclosure is a `<button>` toggling local state, not `<details>`.** `<details>`/`<summary>` would
work without JS, but the row's content — gauge, wording, scale, chevron — has to sit inside
`<summary>`, whose default marker and flex behaviour need overriding in every browser, and the
project's Svelte 5 components already reach for `$state` for this shape. The page is SSR'd but the
signal is a secondary affordance; a collapsed row that needs hydration to expand is acceptable where
a mis-styled marker in Safari is not. `aria-expanded` plus `aria-controls` carry the semantics.

**The projection lands in `ghost.ts` as `ghostGauge`.** A third exported function beside the existing
two, returning `{ segments, filled, tone }`. Keeping the arithmetic and the tone thresholds out of
the component is what makes them testable at all, given the absent component harness — and it keeps
`GhostChecklist.svelte` a template.

**`GhostChecklist.svelte` is rewritten in place, not replaced by a new file.** It already owns the
checklist markup and the caveat; the change adds a gauge row above and a toggle around it. A new
`GhostSignal.svelte` alongside would leave the old component as dead code or force a second edit to
delete it. The name stays slightly narrow for what it now renders, which is cheaper than the churn of
renaming a component with one call site.

**The expanded view names the missing criteria once, and links out for the caveat.** Four rows each
reading "No data" said the same thing four times — and said it wrongly, so the line reads
"Not observed: …" instead. That wording lives in `ghostUnobserved` rather than the template, which
puts the doctrine under test: one line naming those criteria says it once and still tells the reader
why the level is not higher — which is the whole reason unfired criteria are
shown at all. A fired criterion's fact moves onto its own line as a lower-case continuation instead
of a second line of type, and `detailFor` returns an empty string where firing IS the fact (the tick
already said "yes"). The standing caveat becomes a link to `/features/ghost-jobs`, which states it
and devotes a whole section to where each criterion is blind — more than the sentence it replaces,
and it does not cost four lines on every job page.

**The landing renders the components instead of copying their markup.** `GhostLandingView` hand-built
a chip and the checklist panel from illustrative data, with a comment explaining that previews are
markup rather than screenshots. The reasoning was right about screenshots and wrong about copies:
this very change made that panel obsolete, so the page would have described an interface that no
longer exists — to a reader who arrived because they did not understand what they saw. It now renders
`GhostBadge` and `GhostChecklist` against fixture `Ghost` payloads, and the caveat sentence moves out
of the copied panel and onto the page as the section's own line.

**The row moves into `JobView`'s header slot.** It replaces `GhostBadge` there and the section before
the description is deleted. The reader meets the hedge earlier than before — under the title rather
than above the description — and `supersedesReality` keeps deciding between ghost and reality exactly
as it does today.

## Risks / Trade-offs

- **A four-segment gauge is a small target on mobile** → the gauge is decorative (`aria-hidden`); the
  wording and scale beside it carry the same information as text, and the whole row is the hit area
  for the toggle.
- **Tone keyed to count can outrun the wording** — a three-of-four `possible` job renders a
  near-red gauge under hedged text → acceptable, and arguably the point: three fired criteria *is*
  worse than one. The wording never escalates beyond what the classifier served.
- **Collapsed by default means fewer readers see the unfired criteria**, which is what stops the
  signal reading as an accusation → the collapsed row itself carries the hedged wording and the `2/4`
  scale, so the ceiling on the claim is visible without expanding. The caveat sentence moves out of
  the component entirely, to the page the expanded view links to.
- **A gauge is a new visual idiom on the job page** → confined to one row on one page; if it earns
  reuse (company pages, the tracking board), it graduates to `design-system` then, not now.

## Migration Plan

Not applicable — a client-side presentation change, shipped with the web bundle. Rollback is a
revert. Note that the signal is gated off in production behind the calibration gate, so this lands
unobserved by users until that gate opens.

## Open Questions

None.
