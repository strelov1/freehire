## Why

Every "Tailor CV" entry point (extension side panel, web application drawer, web job-page
sidebar) navigates straight to `/tailor/[slug]` with no in-context warning, even though the
app already computes a fully deterministic (no-LLM) fit picture for that exact job — skill
coverage (`internal/jobmatch`) and hard-constraint blockers such as location, work mode, and
work authorization (`internal/hardconstraint`) — and already serves both together on
`GET /jobs/:slug/match`. A candidate can spend a tailoring credit on a role their own profile
already shows they don't clear, without ever seeing that signal first. Surfacing it as an
explicit "are you sure" checkpoint costs nothing extra to compute; it only needed a place to
show up before the click commits.

## What Changes

- Add a confirmation modal, shown every time a "Tailor CV" / "Tailor my CV" button is clicked
  (never skipped, even for a perfect match — content adapts instead), before navigating to
  `/tailor/[slug]`.
- Modal content: a skills-coverage section (matched/missing, or an all-clear line) and a
  requirements section (one row per hard-constraint blocker, unmet ones tinted by severity,
  met ones shown satisfied), a footnote clarifying the check is deterministic and spends no AI
  credit, and Cancel / "Tailor my CV" (or "Tailor anyway" when gaps exist) actions.
- Extension: wire the side panel's `MatchCard` "Tailor my CV" button to this modal, reusing
  the `match` data the card already has loaded (no new fetch).
- Web: wire both the application-tracking drawer's "Tailor CV" button and the job-page
  sidebar's "Tailor my CV" button to this modal, via a new singleton dialog controller that
  fetches the job's match/blockers itself when opened.
- Explicitly unchanged: the "View full analysis" revisit links (both surfaces) keep navigating
  directly — they revisit an existing analysis rather than start a new tailor. The `/tailor`
  workspace's own post-arrival "Tailor it for me" / "Walk me through it" choice is untouched.
- No backend change: `GET /jobs/:slug/match` already returns `blockers`; only client types and
  UI need to catch up to data already on the wire.

## Capabilities

### New Capabilities
- `tailor-preflight-check`: the client-side confirmation modal shown before starting a CV
  tailoring session, presenting the existing deterministic skill/blocker check and requiring
  an explicit confirm.

### Modified Capabilities
(none — the endpoints and evaluators this reads are unchanged; only a new client-side
capability consumes their existing output)

## Impact

- **Extension**: `extension/lib/freehire.ts` (widen `JobMatch` wire type with `blockers`),
  `extension/entrypoints/sidepanel/MatchCard.svelte` (button → dialog).
- **Web**: `web/src/lib/jobMatch.ts` (lift shared `toneText`/chip-class helpers),
  `web/src/lib/components/JobMatch.svelte` (switch to the lifted helpers, no behavior change),
  new `web/src/lib/confirmTailorDialog.svelte.ts` + `web/src/lib/components/ConfirmTailorDialog.svelte`,
  `web/src/routes/+layout.svelte` (mount point), `web/src/lib/components/JobDrawer.svelte` and
  `web/src/lib/components/MatchSummary.svelte` (call sites).
- **Backend**: none.
- **Dependencies**: none new — reuses `internal/jobmatch`, `internal/hardconstraint`, and the
  existing `GET /jobs/:slug/match` endpoint as-is.
