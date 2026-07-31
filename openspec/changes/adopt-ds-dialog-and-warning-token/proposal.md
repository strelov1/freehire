## Why

`extend-ds-verification-to-web` put two numbers on the board and deliberately moved
neither. This change spends them.

**Eleven of fifteen primitives have no consumer**, and `Dialog` is the most expensive of
them: `web/` hand-rolls nine `role="dialog"` surfaces, each with its own overlay, focus
handling and Escape key, while the package ships a native `<dialog>` with a counted scroll
lock and a test for the double-open case that every hand-rolled one gets wrong.

**564 token violations sit in `web/src`, and 106 of them are amber** — one hue doing the job
of a semantic colour that does not exist. `destructive` has a token; caution does not, so
every call site picks its own shade and its own dark-mode partner. Thirty-five of the 106
are `dark:` variants that exist only because there is no token for `.dark` to override.

## What Changes

- **A `warning` token family**, following the shape `brand` already established:
  `warning`, `warning-foreground`, `warning-muted`, `warning-strong`, in both the light and
  the dark file.
- **The 106 amber utilities across 21 files move onto it.** The 30-odd `light dark:`
  pairs collapse to one utility each — `text-amber-700 dark:text-amber-400` becomes
  `text-warning`, because that is the whole point of a token.
- **Five of the nine hand-rolled modals become `Dialog`**: `AuthDialog`, `ReportDialog`,
  `GmailConnectDialog`, `RequestReferralModal`, `DeleteAccountButton`. All five are the same
  shape today — `fixed inset-0 … flex items-center justify-center p-4` — which is exactly
  what `Dialog` is.
- **The other four are named, not forced.** `CookieConsent` is a bottom banner and not modal
  at all; `JobDrawer` is a full-height drawer; `FilterModalShell` and `OnboardingWizard` are
  responsive sheets — `items-stretch` on mobile, centred above `sm`. Bending them onto
  `Dialog` would mean re-adding by hand everything `Dialog` deliberately does not do. What
  they want is a **Sheet** primitive the system does not have, and that gap is worth
  recording rather than papering over.
- **Both baselines move down**, and the diff records by how much.

## Capabilities

### New Capabilities
- `design-system-warning-colour`: the semantic colour for caution — what it means, what it
  is named, and the rule that a caution surface may not pick a hue of its own.

### Modified Capabilities
- `design-system-verification`: the adoption baseline and the web token baseline both change,
  and the change is the point rather than an incidental edit.

## Impact

- `design-system/tokens/color.tokens.json` and `color-dark.tokens.json` — four new entries
  each. **`dist/` must be rebuilt in the same commit**, now that CI diffs it.
- `web/src` — 21 files for the colour, 5 for the modals.
- `design-system/scripts/*-baseline.json` — both rewritten with `--update`.
- No API change, no schema change. Every edit is presentational and must be **verified
  visually in both themes**, because a token that resolves to the wrong lightness compiles
  perfectly and simply looks wrong.

### Adjacent, and deliberately not in scope

The same audit found the rest of an informal palette with no tokens behind it: emerald and
green at 41 occurrences (success), red and rose at 32 (danger — where a `destructive` token
already exists and is not being used), blue and sky at 23 (informational). Each is the same
defect as amber. They are left for a following change so this one stays reviewable, and so
the `warning` family can prove the shape before three more copy it.
