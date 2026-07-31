## Context

`GhostLandingView.svelte` is 256 lines rendering eight stacked sections. It is built on a
good idea that this change keeps: the page renders from the product's own vocabulary rather
than from copy. `GHOST_SIGNALS` is derived from `CRITERIA` in `$lib/ghost`, and
`ghostSignals.test.ts` fails when a criterion joins the vocabulary without an explanation;
`GHOST_FAQ` feeds both the visible FAQ and `faqPageJsonLd`, so schema and page cannot
disagree. The two live previews already render the real `GhostBadge` and `GhostChecklist`
rather than screenshots or copied markup.

What it lacks is any graphic. Four criteria, two tiers, two gates and a prevalence range are
all stated in prose at a uniform `text-sm text-muted-foreground`, and the reader arriving
from the job page's "How this works" link has to mine ~2,500 words for the two facts they
came for.

Three existing constraints shape everything below:

- **`ghostBadge()` does not compute the level.** It reads `ghost.level` from the served
  payload. The gate rule lives in `internal/ghost` and has no frontend counterpart at all.
- **The escalation palette is partly migrated to semantic tokens.** `GhostChecklist` now
  uses `bg-warning` and `text-warning-strong` alongside raw ladder tones, and carries an
  explicit rule that an unfired criterion is never drawn in a reassuring colour.
- **`web/`'s vitest suite tests pure `.ts` modules.** There is one `.svelte.test.ts`, for
  runes, not component rendering.

## Goals / Non-Goals

**Goals:**

- Answer the arriving reader's two questions — which criteria fired, and why the level is
  not higher — above the fold and in pictures.
- Cut visible copy to roughly a fifth without deleting a single honesty caveat.
- Make the level rule checkable by a test instead of asserted in prose.
- Keep the page's existing anti-drift property: nothing on it may describe an interface or a
  vocabulary the product no longer has.

**Non-Goals:**

- No change to `internal/ghost`, any worker, the API, or the database.
- No new visual identity. The page stays in the `/features` language — `SectionLabel`,
  design-system tokens, hairline grids — and does not become a showcase.
- No chart library. These are mechanic diagrams, not plots.
- No refactor of `GhostChecklist` onto the new disclosure primitive.
- No strengthening of any claim the page makes about employers.

## Decisions

### `ghostLevel()` in `$lib/ghost.ts`, not a lookup table and not an endpoint

The sandbox needs a level for an arbitrary selection of criteria, and nothing in the
frontend can produce one. Three options were considered.

A **precomputed fixture table** mapping selections to payload literals introduces no
"logic", but the mapping *is* the logic, hidden where no test would think to check it. A
**preview endpoint** returning the real classifier's verdict eliminates drift entirely, at
the cost of a public route existing for a marketing page and a network round-trip behind a
checkbox — overengineering by this repo's own standard.

The chosen option is a three-line pure function beside `CRITERIA`, with the two gate
constants moved there from `ghostSignals.ts` and re-exported. The reasoning that settles it:
the frontend **already** mirrors `CONVERGENCE` and `WITNESS_GATE`, and the landing already
asserts the gate rule — in English, where no test can reach it. This does not add a mirror;
it moves an existing one out of prose into a tested function and consolidates it from two
modules into the one that declares itself the mirror of `internal/ghost`.

### Pure geometry modules for two diagrams, not six

`activityChart.ts` sets the precedent, with its rationale in the file: geometry lives in
`.ts` so the bug-prone scaling math is unit-testable, and `ActivityBars.svelte` is a dumb
renderer.

That applies to the prevalence waffle, whose 100 cell states derive from the published
range, and to the gate matrix, whose four cells derive from `ghostLevel`. Both genuinely
compute, and both would fail silently if wrong.

It does not apply to the four criterion diagrams. Those are drawings with literal
coordinates; routing them through a model would be ceremony for symmetry's sake, which the
project's working principles call out directly.

### The diagram registry is exhaustive by type, not by test

The original plan asserted "every criterion has a diagram" from a unit test. Probed before
any diagram was drawn, that turns out to be unreachable here: vitest in this project fails
on a `.svelte` import twice over — the `$lib` alias does not resolve, and with a relative
path the component reaches `vite:import-analysis` untransformed, because the svelte plugin
does not apply in the test environment. Making it work means adding jsdom, a Svelte testing
library and plugin configuration — a test-infrastructure change well outside a landing-page
redesign. The documented fallback (export the registry's keys for the test) fails for the
same reason: importing the registry pulls its `.svelte` imports along.

The replacement is stronger than what it replaces. `CRITERIA` gains
`as const satisfies readonly {...}[]`, which checks the shape without widening the inferred
type, and a `GhostCriterionCode` union is derived from it. The dispatcher's registry is
typed `Record<GhostCriterionCode, Component>`, so a criterion with no diagram is a
compile error — `TS2741`, caught by `pnpm run check` in CI, visible in the editor, and
impossible to leave unwritten. `.map` and `.flatMap` on the now-readonly array are
unaffected, so `ghostChecklist` and `ghostSignals` need no change.

`as const` alone would give the literal codes but lose the shape check, letting a
mistyped `tier` through silently. The existing explicit annotation checks the shape but
widens `code` to `string`, leaving nothing to build a union from. `satisfies` is what
does both.

### Native `<details>` for twelve disclosures

The page will hold eight FAQ disclosures and four criterion disclosures.
`GhostChecklist`'s hand-rolled pattern — button, `aria-expanded`, `aria-controls`, an
always-mounted panel toggled between `flex` and `hidden` — is justified there by a trigger
row containing a gauge, a chip and a chevron, and by two non-obvious traps its comments
record: the `aria-controls` IDREF must resolve in the collapsed state, and Tailwind's
`hidden` utility is required because the `flex` class beats the HTML attribute's
`display: none`.

None of that applies to a plain disclosure. `<details>/<summary>` gives the expanded state
and the trigger-content association from the platform, operates without client JS under
SSR, and keeps collapsed text in the DOM for `faqPageJsonLd`. A thin `Disclosure.svelte`
wraps it for styling only.

`GhostChecklist` is deliberately not migrated: it is a product component imported by the job
card, and rewriting it is not a side effect of editing a landing page. The seam is noted, not
taken.

### Diagrams are `aria-hidden`, following the gauge

Each criterion's name, example observations and summary sit beside its diagram as text
carrying the same information. The gauge in `GhostChecklist` is `aria-hidden` for exactly
this reason, with the reasoning written down: a screen reader gains nothing from unlabelled
shapes. The diagrams inherit the treatment and the justification.

### The evergreen diagram drops the time axis

The criterion's example text opens with "Open 240 days", which invites a timeline. Measured
on the live catalogue, none of the 16,461 open postings carrying a fresh stamp are older
than the 90-day threshold: the marked ones converge through repost and concurrent-copy
thresholds instead. A timeline would be the most legible element on the page and would name
the wrong cause to a reader who came to find out why their posting is marked. The diagram
shows the stack of concurrent copies and a repost count, and no axis.

### `gist` beside `why`, rather than a shortened `why`

`why` carries the caveats the feature's ethics rest on — that the board criterion counts
only where the board is crawled, that silence counts only behind a connected mailbox. Those
cannot be cut to fit a card. `SignalExplainer` gains a short `gist` for the always-visible
line, `why` moves under the disclosure unchanged, and `ghostSignals.test.ts` grows to
require both.

## Risks / Trade-offs

**A frontend mirror of the gate rule can drift from `internal/ghost`.** → The constants were
already mirrored, so this adds no new class of drift; it converts an unverifiable prose
claim into a tested function and puts it in the single module that announces itself as the
mirror. A threshold change in Go now has one frontend place to look instead of three.

**The "every criterion has a diagram" assertion cannot be a unit test here.** → Probed and
settled before any diagram was drawn: vitest cannot import a `.svelte` component in this
project without new test infrastructure. The invariant moved to the type system instead, as
a `Record<GhostCriterionCode, Component>` the compiler must see satisfied. A duplicated list
of criterion codes was rejected outright — a list that can drift from the registry defeats
the assertion it exists to make.

**Diagrams built from semantic tokens can collapse into one tone in dark mode**, taking
hatching and hollow outlines with them. → Both themes are verified visually before the change
is called done; the states that carry meaning by texture rather than hue are the ones to
check.

**Cutting ~2,000 visible words costs organic search on a page that currently ranks on
long-form text.** → Accepted deliberately: the primary audience is the in-product reader
arriving from the checklist link. The mitigation is that almost nothing is deleted — the text
moves behind disclosures and stays in the served HTML, and `GHOST_FAQ` and its structured
data are untouched.

**`GhostLandingView` could simply grow to 600 lines.** → The six diagrams, the sandbox and
the disclosure wrapper move into `components/ghost/`, following the existing subdirectory
convention. `GhostBadge` and `GhostChecklist` stay put; they belong to the product, not the
landing.

## Open Questions

None. The one unknown — whether vitest resolves a registry importing `.svelte` — was
probed first and answered no; see the registry decision above for what replaced it.
