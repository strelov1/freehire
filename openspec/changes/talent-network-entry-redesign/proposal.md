## Why

The talent-network-profile-visibility change (still open as PR #1727, not
yet merged/archived) shipped a working but under-designed presentation: the
visibility toggle is a small control buried in a Settings tab, and the
public profile page is a bare field list. Manual local testing surfaced
that the entry point is easy to miss and the public page doesn't read as
something a candidate would want to share. This change fixes the
presentation before merge, while the feature is still fresh — no reason to
ship a weaker UI and revisit it later.

## What Changes

- Replace the inline `TalentNetworkSettings.svelte` render on the Settings
  tab with a status-aware button near the top of `my/profile` (solid green
  "Join Talent Network" when off; an outlined status pill showing the
  current mode + icon once public/anonymous) that opens an overlay panel.
- The overlay panel shows a public-link card pinned at the top (visible and
  clickable even when visibility is `off`, so a candidate can always see
  what the URL would be) above the mode picker, which now carries an icon
  per option (🚫 Off / 🌐 Public / 🕶️ Anonymous).
- Redesign `web/src/routes/talent-network/[publicId]/+page.svelte` from a
  flat field list into a header block (avatar-or-initials, name, headline,
  location, summary, skill chips) followed by a single-column experience
  and education list.
- No backend, API, database, or visibility/redaction-rule change of any
  kind — every field already returned by the existing endpoints is reused
  as-is, only its presentation changes.

## Capabilities

### New Capabilities
- `talent-network-entry-ui`: the candidate-facing entry point and public
  profile page presentation for the Talent Network feature — the
  status-reflecting entry button, the overlay panel's always-visible
  public-link card, the icon-bearing mode picker, and the public profile
  page's header-block-plus-single-column layout. This is presentational
  behavior layered on top of `talent-network-profile` (visibility
  semantics, masking rules, API shape) without changing any of it.

### Modified Capabilities
(none — `talent-network-profile` has not been archived into
`openspec/specs/` yet, since the change that introduces it (PR #1727) is
still open, so there is no existing spec folder to delta against. This
change's new capability above is additive on top of that still-pending
spec, not a modification of it.)

## Impact

- **Frontend only**: `web/src/lib/components/TalentNetworkSettings.svelte`
  (replaced by a new panel-hosting component), `web/src/routes/my/profile/+page.svelte`
  (button placement), `web/src/routes/talent-network/[publicId]/+page.svelte`
  (layout).
- No changes to `internal/handler/talent_network_profile.go`,
  `internal/handler/me_talent_network.go`, `internal/resumeextract/visibility.go`,
  or any migration/query.
