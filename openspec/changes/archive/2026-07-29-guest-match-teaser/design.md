## Context

Two surfaces render a profile match today, from the same state machine
(`resolveMatchState` in `web/src/lib/jobMatch.ts`) but with different fallbacks for
the locked states:

- `JobMatchBar` (inside `JobRow`) renders `null` when the owning card passes
  `match: null` — so a guest sees nothing.
- `JobMatch` (job-detail sidebar, also used by `JobDrawer` and `SwipeDeck`) renders a
  module-level constant `{percent: 76, have: ['React','Docker','SQL'], missing: ['Kafka']}`
  under `blur-[1.5px]`.

The job itself is server-rendered; only the viewer's profile hydrates client-side via
`profileStore`. That asymmetry is what forces the teaser to be a pure function of the
job: it has to produce the same output in the SSR pass, before any profile is known,
as it does after hydration.

## Goals / Non-Goals

**Goals:**

- One teaser derivation, shared by the card and the sidebar, so the same job cannot
  show 73% in the feed and 68% on its own page.
- Teaser figures that survive SSR → hydration and arbitrary re-renders unchanged.
- Teaser chips that name the job's real skills.
- No new network traffic: the locked states still never call `GET /jobs/:slug/match`.

**Non-Goals:**

- Computing a genuine match for a guest. There is no profile to match against; the
  teaser is an invitation, not an estimate.
- Touching the backend match endpoint, its formula, or the adjacency dictionary.
- Changing what a viewer with a profile sees, on either surface.

## Decisions

### Deterministic hash over `Math.random()`

The teaser is seeded from `job.public_slug` through FNV-1a (32-bit) and a `mulberry32`
PRNG, both a handful of lines and dependency-free.

`Math.random()` was the obvious alternative and is wrong here for three separate
reasons, each of which is a visible defect rather than a theoretical concern:

1. **SSR divergence.** The server renders one number, the client another; Svelte
   replaces the text on hydration, so the percentage visibly flips on first paint.
2. **Re-render churn.** The feed re-renders on filter changes and pagination. A
   `$derived` over `Math.random()` re-rolls, so a card's score changes while the user
   scrolls past it.
3. **Cross-surface disagreement.** `JobDrawer` shows the sidebar block over the same
   feed that shows the card. Two independent rolls put two different scores for one
   job on screen at once.

A slug-seeded hash costs about fifteen lines and removes all three.

### `matchTeaser()` returns a `missing` set, not a chip list

The helper's shape is deliberate:

```ts
export type MatchTeaser = {
  percent: number;      // 60..90
  matched: number;      // round(percent/100 × total)
  total: number;        // the job's real skill count
  missing: Set<string>; // which of the job's real skills read as "missing"
};
```

`{percent, matched, total}` is structurally the existing `ClientMatch`, so
`JobMatchBar` needs no new shape to render a teaser — only a `blurred` flag. Handing
back a `Set` of names rather than a pre-sliced chip list keeps the decision of *how
many* chips to show with each surface (the card shows five, the sidebar three) while
the *tint* of any given skill stays one shared answer. A pre-built list would have
forced the sidebar's cap onto the card.

### Coherence beats pure randomness

`matched` is derived from `percent`, not rolled independently, and the missing set is
sized as `total - matched`. Rolling them apart produces cards that read as bugs — a
73% bar over four green chips, or "8 of 11" beside 61%. The randomness is spent on
*which* skills are marked missing (a `mulberry32` shuffle), not on the arithmetic, so
red chips scatter through the row instead of always trailing it.

Two clamps keep the visual honest at the edges: at least one held skill (never an
all-red row under a 60–90% bar) and at least one missing (never an all-green row under
anything short of 100%).

### A one-skill job gets no teaser

Rendering the teaser against live data showed the one case where a viewer can catch it
out: a job with a single skill reads "1 of 1 skills" beside an 87% bar and a lone green
chip. The fraction says everything matched, the bar says it didn't, and there is no
have/missing contrast to show in the first place.

The alternative — suppressing just the fraction for that case — trades one visible
inconsistency for a special case in two components. Dropping the teaser below two
skills instead *removes* a special case: the "at least one missing" clamp becomes
unconditional and the percent ceiling loses its `total === 1` branch. On a sampled feed
page, 57% of jobs carry no skills at all (already the not-enough-data state) and 11%
carry exactly one, so the teaser covers the 32% where it has something to say.

### A capped chip row borrows a missing skill

The sidebar fits three chips at a legible width. Cutting the job's first three skills
can miss the only missing one — a job at "5 of 6 skills" often renders a uniformly green
row, which is precisely the contrast the teaser exists to show. `teaserChips` therefore
trades the row's last chip for the first missing skill when the window would otherwise
be all-held, preserving the job's own order among the rest.

The card's five-chip window is left alone: its chip order is real information shared
with the signed-in state, and a wider window rarely misses every missing skill.

### Blur the chips container, not the row

On the card, the skills and the salary share one flex row. The blur goes on the chips
container so the salary — real information a guest should be able to read — stays
crisp. The existing `Badge` variants `brand` and `missing` already carry the two
tones (`design-system/src/Badge.svelte`), so no new styling is introduced.

### `aria-hidden` plus an `sr-only` invitation

The blurred strip gets `aria-hidden="true"`, and the card exposes a visually-hidden
"Sign in to see your profile match" in its place. Today `JobMatchBar` builds an
`aria-label` announcing the percentage; leaving that in place for a teaser would read
a fabricated score to a screen-reader user as fact, which the blur does not
communicate to them at all.

## Risks / Trade-offs

- **A guest reads the teaser as a real score.** → It is blurred and hidden from
  assistive technology, which is offered the sign-in invitation in its place. In the
  sidebar the copy names it as not-yet-computed ("Sign in to see your match"); on the
  card the blur is the only signal a sighted viewer gets, since the card has no room
  for copy and the spec asks for none. Accepted: the same trade the existing static
  teaser already makes, now less contradictory because the skills on screen belong to
  the job. If the blur alone proves too subtle in the feed, a lock glyph in the strip
  is the cheap next step.
- **The card's skill names become harder to read for guests.** → `blur-[1.5px]` is
  light enough to leave short dictionary slugs legible, and the text stays in the DOM,
  so crawlers and SEO are unaffected. This was an explicit product call in favour of
  the stronger nudge.
- **A slug-derived percent is stable enough to be mistaken for meaningful.** → It is
  by construction never at either extreme (60–90) and never 100%, so it cannot be
  read as a verdict.
