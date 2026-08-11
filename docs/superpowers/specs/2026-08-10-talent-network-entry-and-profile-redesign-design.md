# Talent Network — entry point + public profile page redesign

## Context

The talent-network-profile-visibility change (see
`docs/superpowers/specs/2026-08-10-talent-network-profile-visibility-design.md`
and `openspec/changes/talent-network-profile-visibility/`) shipped a working
slice: a tri-state visibility toggle buried inline on the Settings tab of
`my/profile` (`TalentNetworkSettings.svelte`), and a plain single-column
public profile page (`web/src/routes/talent-network/[publicId]/+page.svelte`).

After manually trying it locally, the candidate-facing UX needs two changes:
the entry point is too easy to miss (a small control inside a settings tab),
and the public page reads as a bare data dump rather than something a
candidate would be proud to share. This document redesigns both — no backend
change, no new API surface, and no change to the visibility/redaction rules
already shipped and reviewed. Purely presentational.

Reached by working sessions in a visual-companion brainstorming tool; every
decision below was picked from mocked-up options, not asserted.

## Goals / Non-Goals

**Goals:**
- A prominent, unmissable entry point on `my/profile` instead of a buried
  settings row.
- The entry point communicates current status at a glance once a candidate
  has already opted in — not just "click here to configure."
- The public profile page reads as a real profile (header identity block +
  skill chips + narrative work history) instead of a plain field dump.

**Non-Goals:**
- No change to `off`/`public`/`anonymous` semantics, the masking rule, or
  any backend endpoint — this is presentation only.
- No photo on the public page — still deferred (unchanged from the shipped
  design's Non-Goals).
- No employer-facing anything — still out of scope.
- No new design-system component library work beyond what a single overlay
  panel needs — this is one feature's UI, not a reusable primitive unless a
  second call site appears later.

## Decisions

**Entry point is an overlay panel, not a new tab.** Two options were
mocked: adding a "Talent Network" tab to the existing `TabRow` strip
(reuses existing tab machinery, cheapest to build), versus a button that
opens a focused overlay panel on top of whatever tab is currently active.
The panel was chosen — it reads as a deliberate, self-contained decision
("join this network") rather than one more settings row among six tabs,
which matches the weight the feature actually has (a candidate is choosing
to expose data publicly, not tweaking a preference).

**The button reflects current status, not a static label.** When
visibility is `off`, the button is a solid green CTA reading "Join Talent
Network." Once visibility is `public` or `anonymous`, the button becomes an
outlined pill showing the current mode with its icon (e.g. "🌐 Talent
Network: Public") — still clickable, still opens the same panel. This was
chosen over a button that always reads "Join Talent Network" regardless of
state: the header becomes a real status indicator a candidate can read at a
glance, at the cost of three button visual states instead of one (accepted
— it's a small, bounded set of states, not open-ended).

**The panel's public-link card is always visible at the top, before the
mode picker.** Two orderings were mocked: link/preview appearing only
after a mode is picked (decision-then-consequence ordering), versus a
link/status card pinned at the top regardless of current mode, showing
what the URL is (or would be) with a "View →" action even when visibility
is `off`. The always-visible version was chosen — a candidate can always
find and check their link without first having to remember or guess their
current mode.

**Each mode option in the picker carries an icon.** 🚫 Off, 🌐 Public, 🕶️
Anonymous — paired with the existing short description text already
shipped in `TalentNetworkSettings.svelte` (e.g. "Name & current employer
hidden" for Anonymous). Purely a scanability aid; no behavior change.

**Public profile page becomes a header-block-plus-single-column layout.**
Current shipped page is a flat list of fields. Redesign: a header block
(avatar-or-initials circle, name — omitted in anonymous mode per existing
rule, headline, location, one-line summary, skill chips inline) followed by
a single column of experience entries (each with a small placeholder/logo
box, title, company — masked per existing anonymous-mode rule, date range,
description) and education below that. No sidebar, no availability
badges, no "message me" CTA — none of that data exists yet (work
authorization, availability status) or the flow it would trigger exists yet
(intro requests). This is restyling of existing fields already returned by
`GET /api/v1/talent-network/:publicID` — no new field is introduced by this
redesign.

**No competitor or product names anywhere in shipped code, comments, commit
messages, or docs** — see `hire-no-competitor-names-in-code` project
convention. This document itself avoids naming the reference used during
brainstorming; describe layout choices generically (as done above).

## Risks / Trade-offs

- **[Risk]** A three-state button (solid CTA / three outlined status pills)
  is more UI surface than a single static button → **Mitigation**: the
  state set is small and closed (exactly the three visibility values that
  already exist), not a source of open-ended complexity.
- **[Risk]** An overlay panel is more component work than reusing the
  existing tab strip → **Mitigation**: accepted deliberately — the feature
  is a distinct, weighty decision for a candidate, and the tab-strip
  alternative was mocked and passed over specifically because it undersold
  that weight.
- **[Risk]** Redesigning the public page's visual layout while explicit
  fields (work-authorization, availability) remain unavailable could read
  as an unfinished profile compared to a fuller reference layout →
  **Mitigation**: deliberately scoped out; those fields don't exist in this
  product yet and inventing placeholder data for them would misrepresent
  the candidate.

## Migration Plan

Pure frontend change: modify `TalentNetworkSettings.svelte`'s trigger (a
new overlay-panel host component replacing the current inline settings
render) and the public page's template. No migration, no API change, no
data backfill. Existing shipped tests for the settings component and the
public page's server load function are unaffected by a template-only
change; new tests should cover the button's three visual states and that
the overlay's public-link card renders correctly across `off`/`public`/
`anonymous`.
