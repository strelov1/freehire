## Context

`web/src/lib/components/JobView.svelte` lays its tab row out as
`flex items-end justify-between`: the content `TabStrip` takes `min-w-0 flex-1` on the
left, and the `actionStrip` snippet sits `shrink-0` on the right. The strip is the only
half that cannot give ground, so every control added to it has been taken out of the tabs.
It now holds six: a Discussion link, Report, Save, Add-to-list, Auto-apply and Apply. On a
desktop column that leaves the TabStrip a scrolling sliver — the component handles the
overflow correctly (it scrolls and fades rather than wrapping), so the failure is silent
and reads as tabs sliding under the actions.

The rank on that row is also wrong. `autoApplyCta` renders `variant="secondary"` and the
external `applyCta` renders `variant="primary"`, so the page's loudest button is the one
that hands the reader off to somebody else's site, and the Pro feature this page exists to
sell is the quiet grey one beside it.

`autoApplyButtonState` (`web/src/lib/autoApplyButton.ts`) already reduces the posting's
source and the caller's attempt status to a six-way `kind`. It is a pure function with its
own unit test and no Svelte dependency — the convention that file states for itself.

## Goals / Non-Goals

**Goals:**

- Give the content tabs their width back by moving the two CTAs off the tab row.
- Make auto-apply the page's primary CTA wherever it can actually be started, and say in
  the button that it needs Pro.
- Never put two primary CTAs on the page, and keep one in every state where the reader
  still has an action left — including the states where auto-apply will not act and they
  must apply by hand.

**Non-Goals:**

- Client-side Pro eligibility. It is still not known without new plumbing; the `Pro` marker
  names a requirement, it does not gate the click. The backend's 402 remains the answer.
- Any change to auto-apply's behaviour: the POST, the statuses, the worker, the error
  surface. Only the button's rank, label and place move.
- The sub-`lg` layout. The sticky bottom bar and the under-title strip stay as they are.
- ~~Bringing auto-apply to mobile. It is desktop-only today and stays that way here.~~
  **Reversed during the change.** Leaving the phone without an auto-apply button meant the
  feature the desktop page now leads with was invisible to most of this page's traffic, and
  the sticky bottom bar is the phone's one call to action — so it carries whichever control
  the plan made primary. See "Every bar carries the plan" below. Struck rather than
  deleted: a non-goal that turned out to be wrong is worth more as a record than as a
  tidy list.

## Decisions

### The six-state CTA rank lives in `autoApplyButton.ts`, not in the template

A second exported pure function, `jobCtaPlan(state: AutoApplyButtonState)`, maps the
existing `kind` to what the two buttons look like:

```ts
export type JobCtaPlan = {
  autoApply: { label: string; primary: boolean; pro: boolean; disabled: boolean } | null;
  external: { label: 'Apply' | 'Show origin'; primary: boolean };
};
```

`autoApply` is nullable rather than carrying a `shown` flag: `hidden` renders no button, so
a label, a loudness and a disabled state for it would be four fields describing nothing.

| `kind`     | auto-apply                              | external                 |
| ---------- | --------------------------------------- | ------------------------ |
| `hidden`   | not rendered                            | `Apply`, primary         |
| `idle`     | `Auto-apply` + `Pro`, primary, enabled  | `Show origin`, outline   |
| `queued`   | `Auto-apply queued`, quiet, disabled    | `Show origin`, outline   |
| `applied`  | `Already applied`, quiet, disabled      | `Apply`, primary         |
| `declined` | `Auto-apply declined`, quiet, disabled  | `Apply`, primary         |
| `failed`   | `Auto-apply couldn't complete`, quiet, disabled | `Apply`, primary |

**Why a function and not `{#if}` chains.** The interesting part of this change is the
table, not the markup: six states, two buttons, and a rule about how loud they may be that
is easy to break silently in a template. A pure function unit-tests the whole table without
mounting Svelte, which is precisely the seam `autoApplyButtonState` was extracted for.
Putting it in the same file keeps one place that knows what the auto-apply button is. The
rule is worth writing down twice — the invariant tests are what caught that `queued` falls
out of it, below.

**Why `queued` has no primary CTA at all.** The submission is in flight and the reader has
nothing to do but wait. The obvious invariant ("exactly one primary, always") would force a
loud button there, which reads as an invitation to apply a second time. The rule is
therefore two-sided — never two primaries, and one wherever an action remains — with
`queued` the only place it yields none.

**Why `applied` does NOT demote.** It looks like the same case, and it is not. `applied`
comes from `alreadyApplied` — the "Did you apply?" prompt after a manual click-through —
which is true of a posting from any source, while this table only runs on the ones
auto-apply can drive. Demoting on it would make a Greenhouse posting read differently from
an identical Lever one for a reader in the identical situation: an artefact of routing a
source-independent question through the auto-apply state machine, not a decision anybody
made. Whether a page should quiet down for a reader who already applied is a real question,
and it belongs to every source at once — not to this change.

**Why `declined`/`failed` re-promote the external button.** The naive reading of the ask —
"Apply becomes Show origin whenever auto-apply is shown" — leaves a page with no primary
CTA in the two states where the robot has given up and applying by hand is the reader's
only way forward. Demoting there would hide the one action still available. The rule is
therefore "demote while an attempt stands or can be started", not "demote whenever the
button exists".

**Alternative considered:** keeping the mapping inline in `JobView.svelte`. Rejected — the
component is already 800 lines, and the rule that matters would be untestable without a
mount.

### Every bar carries the plan — including the phone's

Four places render the external link: the title row, the pinned desktop header, the mobile
sticky bar, and the quiet strip. All four read `jobCtaPlan`. There is no exception.

The pinned header IS the title row for most of a several-screen description, so a
brand-filled `Apply` there for the link the title row calls a quiet `Show origin` would put
one link at two ranks on one page — and the argument against a loud button beside a queued
attempt applies with full force to the bar the reader actually has in front of them. It
therefore gains an auto-apply button it did not have before.

**This section had an exception, and it was removed.** An `undemotedExternalCta` constant
held the apply link at full rank for the mobile sticky bar, on the argument that auto-apply
had no button below `lg`, so a demoted label there would step aside for nothing the reader
could reach. That argument was sound and it stopped being true within this change: once the
phone's sticky bar could carry auto-apply itself (see below), the constant described a
world that no longer existed. It is deleted rather than kept — an exception nobody can
trigger is worse than no exception, because it reads as a live rule.

### CTAs move into the existing `<header>` block, right-aligned against the title

The header block (`lg:col-start-2 lg:row-start-2`) already holds the title and the
`Applied` chip. The CTA group first joined that row with `ml-auto`, which made its position
a function of how long the title happened to be — a two-word role left the buttons adrift
mid-page, and a long one wrapped them onto a second line anyway. They now always take that
second line, right-aligned, so they land above the quiet strip's own right edge on the tab
row below. It renders `hidden lg:flex`; the phone reaches the same two controls through the
sticky bar and the quiet strip instead.

**Alternative considered:** a separate full-width action row above the tabs. Rejected —
it leaves a wide, near-empty band on the page and still separates the primary action from
the thing it acts on.

### The `Pro` marker is a span inside the button, not the `Badge` primitive

`Badge`'s variants carry their own background and foreground colours, none of which read as
a plan marker on `bg-brand`. The marker is a small inline pill taking the PAGE's own
`foreground` for its fill and `background` for its text — near-black on white in the light
theme, near-white on black in the dark one — so it is the highest-contrast thing on a
brand-green button in either. A tint of the button's own foreground
(`bg-brand-foreground/15`) was the first attempt and dissolved into the fill.

### The demoted button uses `outline`, not `ghost`

`Show origin` is still a real action and still needs a hit target that reads as a button
beside the filled one. `ghost` would flatten it into the quiet strip's vocabulary, which is
where Save and Report live.

## Risks / Trade-offs

- **A non-Pro reader now meets a loud green button that refuses them.** → Deliberate: the
  refusal is a 402 carrying the upgrade path, and this is the page where that offer belongs.
  The `Pro` marker in the button is what keeps it from being a surprise.
- **A SIGNED-OUT reader meets the same loud green button, and it only opens the sign-in
  prompt** — while the link that does what they came for reads `Show origin`, a phrase that
  does not say "apply". Signed-out traffic is most of the job page's traffic, so this is the
  common case rather than an edge one. → Kept, because the page is where the Pro feature is
  advertised and a visitor who cannot see it will not sign up for it; but it is the change's
  weakest point and the first thing to measure. Making the plan fall back to `hidden` while
  `!isAuthenticated()` is a one-line change if the funnel says so.
- **`Show origin` is a weaker word than `Apply` and may cost external-apply clicks on
  Greenhouse postings.** → Accepted, and bounded: Greenhouse is a small slice of the
  catalogue, and the tracking on that click is unchanged, so the effect is measurable after
  the fact rather than guessed at now.
- **The apply-intent event and the "Did you apply?" prompt still fire under a button that
  no longer says "Apply".** → Kept on purpose: clicking through to the posting is still the
  funnel step, and splitting the event would break the comparison with every other posting.
  Noted in the spec so it is a decision rather than an oversight.
- **The header row gets crowded on a long job title.** → It is already `flex-wrap`; the CTA
  group wraps to its own line rather than truncating the title.

## Open Questions

None.
