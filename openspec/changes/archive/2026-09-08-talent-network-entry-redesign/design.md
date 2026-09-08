## Context

Full background and every option that was mocked/compared during
brainstorming lives in
`docs/superpowers/specs/2026-08-10-talent-network-entry-and-profile-redesign-design.md`
(approved). This document restates the decisions in OpenSpec's
Context/Goals/Decisions/Risks shape; that file remains the fuller record.

The talent-network-profile-visibility change (PR #1727, not yet merged)
shipped: a tri-state visibility toggle inline on `my/profile`'s Settings
tab (`TalentNetworkSettings.svelte`), and a flat-field-list public page
(`web/src/routes/talent-network/[publicId]/+page.svelte`). Manual local
testing surfaced that both are functionally correct but under-designed —
easy to miss, doesn't read as something worth sharing. This change is a
presentation-only redesign of both, with zero backend/API/data changes.

## Goals / Non-Goals

**Goals:**
- Replace the buried settings-tab toggle with an unmissable, status-aware
  entry point on `my/profile`.
- Let a candidate read their current Talent Network status at a glance
  without opening anything.
- Make the public profile page read as a real profile, not a field dump.

**Non-Goals:**
- No change to visibility semantics, masking rules, or any API/DB —
  `talent-network-profile`'s existing behavior is reused as-is.
- No photo — still deferred, unchanged from the shipped design.
- No employer-facing anything.
- No reusable design-system component extraction — this is one feature's
  UI; a second call site would be the trigger to generalize, not this one.

## Decisions

**The entry point is gated behind `users.beta_tester`, hidden entirely for
a non-beta account.** REVISED after the shipped feature was manually
tried: the feature isn't ready for a general audience. Hidden, not merely
disabled — a visible-but-locked control would announce a feature exists
before it's meant to. `currentUser()?.beta_tester` (already exposed by
`$lib/auth.svelte`, the same field `AccountNavRail`/`my/+layout` already
gate other beta surfaces on) guards both the entry button and the
visibility fetch it would otherwise trigger — a non-beta page load makes
no `/me/talent-network` call at all. Frontend-only: the backend routes and
the public profile page are unchanged and still reachable by anyone who
already has a link — this gate is about discovery, not enforcement, and no
backend enforcement was asked for.

**Entry point is a dedicated page (`/my/talent-network`), not an overlay or
a tab-strip addition.** Originally built as an overlay panel (mocked
against a `TabRow` tab and chosen over it — opting into a public profile
is a deliberate, weighty decision, and a dedicated surface communicates
that better than a settings-tab row). REVISED after live review: the panel
was itself replaced with a plain route. The entry button now links to it
directly rather than opening anything in place — simpler, gets its own
URL/back-button/reload behavior for free, and avoids the extra
component-in-component nesting an overlay needs.

**The entry button reflects current status, not a static label.** `off` →
solid green "Join Talent Network" CTA. `public`/`anonymous` → an outlined
status pill with the mode's icon (e.g. "Talent Network: Public"), still
clickable, links to the settings page. Chosen over an always-static label
because the header becomes a real status indicator — worth three visual
states (a small, closed set) for that.

**No public-link-and-copy card.** Originally designed as a card pinned
above the mode picker, always visible (including when `off`), showing the
raw URL plus "Copy link"/"View" actions. REVISED after live review: cut
entirely as unnecessary — a browser's own address bar already covers
copying, so the only thing worth keeping is a way to open the page. That
surfaces instead as a single "View your public page" button in the page's
own header (top-right, solid/primary styling — visible only once
`public`/`anonymous` is selected, since there's nothing to view from
`off`), not a persistent card.

**Each mode option carries a real icon component, not an emoji.**
`EyeOff` (Off), `Globe` (Public), `VenetianMask` (Anonymous) from
`@lucide/svelte` — REVISED from the original emoji-based mockup after live
review found emoji render inconsistently and read as less deliberate than
the rest of this codebase's icon usage (every other icon in this feature,
and the app generally, is already a Lucide component).

**Public page becomes header-block + single-column.** Header: avatar-or-
initials circle, name (omitted in anonymous mode — existing rule, unchanged
here), headline, location, one-line summary, inline skill chips. Below:
single-column experience (small logo/initial box, title, company — masked
per the existing content-based anonymous rule, date range, description)
then education. No sidebar, no availability/work-authorization badges, no
contact CTA — none of that data exists in this product yet, and inventing
placeholders would misrepresent the candidate. Every field rendered is
already returned by the existing `GET /api/v1/talent-network/:publicID`
response; this is restyling, not a new field.

**No competitor/product name appears anywhere in code, comments, commits,
or docs** — see the `hire-no-competitor-names-in-code` project convention.
Layout choices are described generically throughout this document and the
implementation for that reason.

## Risks / Trade-offs

- **[Risk]** Three button visual states (one CTA + two status pills) is
  more surface than a single static button → **Mitigation**: the set is
  small, closed, and mirrors the three visibility values that already
  exist — not open-ended complexity.
- **[Risk]** An overlay panel is more component work than the tab-strip
  alternative → **Mitigation**: accepted deliberately; the tab-strip
  option was mocked and passed over specifically because it undersold the
  decision's weight.
- **[Risk]** Redesigning the page's visual layout while fields like
  work-authorization/availability remain unavailable could read as an
  unfinished profile next to a fuller reference layout → **Mitigation**:
  deliberately scoped out — those fields don't exist in this product, and
  placeholder data would misrepresent the candidate. Revisit only if/when
  that data is actually collected.
