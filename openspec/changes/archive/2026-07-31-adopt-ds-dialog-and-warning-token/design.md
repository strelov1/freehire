## Context

`extend-ds-verification-to-web` made two facts measurable: 11 of 15 primitives have no
consumer, and `web/src` carries 564 token violations across 108 files. This change moves both
numbers for the first time, so it is also the first test of whether the ratchet is workable
in practice rather than only correct.

Two shapes of work, and they are independent: the colour is 21 files of mechanical
substitution whose risk is entirely visual; the modals are 5 rewrites whose risk is
behavioural. They share a change because they share a justification, not a mechanism.

## Goals / Non-Goals

**Goals:**

- One semantic colour for caution, and no call site picking a hue.
- `Dialog` used where the app already means a dialog, with the platform doing the modal work.
- Both baselines demonstrably lower.

**Non-Goals:**

- The other three informal palettes (success, danger, informational). Named in the proposal
  with their counts; deliberately left so this diff stays reviewable.
- A `Sheet` primitive. Four of the nine surfaces want one; this change records that and builds
  nothing.
- Any change to what a surface *does*. Every edit here is how it looks or which element it
  mounts on.

## Decisions

### The family copies `brand`, exactly

`warning`, `warning-foreground`, `warning-strong`, `warning-muted` — the same four roles
`brand` already defines, so a reader who knows one knows the other.

The shape is forced by the usage rather than chosen: `bg-amber-500` (21 occurrences) is a
solid fill and `text-amber-600/700` (32) is text on the page background. Those cannot be the
same value — amber-500 as text on white is about 2.9:1. So a fill (`warning`) with something
legible on it (`warning-foreground`), a text-and-border colour (`warning-strong`), and a soft
tint (`warning-muted`).

*Alternative considered:* one `warning` used for both, as `destructive` does. Rejected —
`destructive` gets away with it because oklch(0.577) is dark enough to read as text, and the
amber the app actually uses is not.

*Alternative considered:* naming the text role `warning` and the fill `warning-solid`.
Rejected: it reads better in isolation and worse in the system, and consistency with `brand`
is worth more than one better name.

Authored in oklch, matching `destructive` and both dark files. `brand`'s light values are hex
— an inconsistency this change does not widen.

### The mapping is a table, and the `dark:` half disappears

| Today | Becomes |
|---|---|
| `text-amber-600/700` + `dark:text-amber-400/500` | `text-warning-strong` |
| `bg-amber-500` + `dark:bg-amber-400` | `bg-warning` |
| `border-amber-500/400` | `border-warning` |
| `bg-amber-100` + `dark:bg-amber-950` | `bg-warning-muted` |
| `text-amber-800` | `text-warning-strong` |

Thirty-five of the 106 occurrences are `dark:` variants, and every one of them exists only
because there was no token for `.dark` to override. They are not translated — they are
deleted. That is the clearest evidence the token is the right answer: a third of the problem
was the workaround.

### Five modals, not nine, and the four are a finding

All five migrating surfaces are `fixed inset-0 … flex items-center justify-center p-4`
today — a centred modal, which is what `Dialog` is. Each keeps its own content and loses its
overlay, its Escape handler, its focus handling and its z-index.

The four that stay are a different component:

| Surface | Shape | Why not `Dialog` |
|---|---|---|
| `CookieConsent` | `inset-x-4 bottom-4` | a banner, and not modal — `showModal()` would trap focus in a cookie notice |
| `JobDrawer` | `inset-0 flex flex-col` | full-height drawer |
| `FilterModalShell` | `items-stretch` → `sm:items-center` | responsive sheet |
| `OnboardingWizard` | same | responsive sheet |

Forcing them would mean re-adding by hand the positioning `Dialog` deliberately does not
offer. What three of them want is a **Sheet** — a primitive the system does not have. This
change records the gap; building it on the strength of three call sites, mid-migration, is
the kind of infrastructure that should wait for the need to be stated on its own.

`CookieConsent` is a separate observation: it carries `role="dialog"` while being a
non-modal banner. That is an accessibility bug, not a migration target, and it is not fixed
here — flagged instead, so it is not smuggled into a colour-and-modals diff.

### Verification is visual, and the compiler cannot help

A token that resolves to the wrong lightness compiles perfectly and simply looks wrong. Every
touched surface is checked in **both themes** before its task is ticked. The ratchet proves
the utilities are gone; it says nothing about whether the result is legible.

`Dialog`'s own behaviour is already tested in the package (`dialog.test.ts`, including the
counted scroll lock). The migration does not re-test it — it tests that each call site still
opens, closes and submits.

## Risks / Trade-offs

- **A migration that lands mid-review goes stale against both baselines.** This branch already
  hit it once: three PRs merged during the previous change and moved 14 violations. → Rebase
  and re-run `--update` immediately before merge, and treat a red baseline on rebase as
  expected rather than as a defect.
- **`Dialog` is `max-w-lg`; some of the five are wider.** → `class` overrides it; the Button
  tests in the previous change pin exactly this `cn` precedence, so it is a tested path.
- **Escape and backdrop-click become the platform's, not the component's.** Behaviour changes
  subtly — a hand-rolled Escape handler that also reset form state no longer runs on its own
  terms. → Read each modal's close path before replacing it, not after.
- **`warning` gets four values chosen by eye.** Nothing verifies contrast. → Chosen against
  the shades the app already ships, so the result is at worst what it looks like today; a
  contrast check across the whole scale is its own piece of work, not a rider on this one.
