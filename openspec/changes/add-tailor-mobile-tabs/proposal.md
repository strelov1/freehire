## Why

On a phone the tailoring workspace only ever showed the left panel (Editor /
Chat). The centre CV preview and the right context panel (Templates / Job
description / Verdict) are gated behind `lg:` and are simply absent below the
breakpoint — there is no control that reveals them. A mobile user tailoring a CV
can talk to the agent and edit fields but can never see the live preview, switch
a template, re-read the job description, or review the verdict.

## What Changes

- Below `lg`, the three columns collapse to a single full-screen view driven by
  one flat, horizontally-scrollable tab bar: **Chat · Editor · Preview ·
  Templates · Job · Verdict**. Tapping a tab shows that view full-width.
- At `lg` and up nothing changes: all three columns render side by side with
  their own splitters and per-column tab bars exactly as before.
- The per-column tab bars (Editor/Chat in the left panel, Templates/Job/Verdict
  in the context panel) become desktop-only; on mobile the single flat bar is
  the sole navigation, so tabs are never duplicated.
- The right context panel's active tab is lifted to the page so the mobile bar
  can drive it; its fixed pixel width moves to a CSS variable so it fills the
  screen on mobile instead of staying a narrow column.

No breaking changes; desktop behaviour is untouched. Purely a frontend layout
change.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `tailor-workspace`: gains a mobile navigation requirement — below `lg` the
  three columns collapse to one flat tab bar switching between all six views,
  while the existing three-column requirement continues to govern wide
  viewports.

## Impact

- **Frontend only:** `web/src/routes/tailor/[slug]/+page.svelte` (mobile tab
  bar, `mobileView` state + `pickMobile` sync, per-breakpoint region
  visibility) and `web/src/lib/tailor/ArtifactPanel.svelte` (`tab` lifted to a
  bindable prop, `mobileVisible` prop, width on a CSS var, desktop-only tab
  bar).
- **Known limitation:** the "Saving/Saved" indicator lives in the left panel's
  desktop-only header, so it is not shown on mobile. Autosave still runs; only
  the status text is absent below `lg`.
