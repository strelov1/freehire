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

**Entry point is an overlay panel, not a tab-strip addition.** Mocked
against adding a "Talent Network" tab to the existing `TabRow` (cheaper —
reuses tab machinery already on the page). The panel was chosen: opting
into a public profile is a deliberate, weighty decision, and a dedicated
overlay communicates that better than one more row among six settings tabs.

**The entry button reflects current status, not a static label.** `off` →
solid green "Join Talent Network" CTA. `public`/`anonymous` → an outlined
status pill with the mode's icon (e.g. "🌐 Talent Network: Public"), still
clickable, opens the same panel. Chosen over an always-static label because
the header becomes a real status indicator — worth three visual states
(a small, closed set) for that.

**The panel's public-link card is pinned at the top, before the mode
picker, and is always visible — including when `off`.** Mocked against a
version where the link only appears after picking a mode. The
always-visible version lets a candidate find their link without first
having to recall their current mode; when `off`, the card still shows what
the URL *would be* (same UUID either way — the setting change only affects
whether the route resolves, not the identifier).

**Each mode option carries an icon**: 🚫 Off, 🌐 Public, 🕶️ Anonymous —
paired with the short description text already shipped, purely a
scanability aid with no behavioral change.

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
