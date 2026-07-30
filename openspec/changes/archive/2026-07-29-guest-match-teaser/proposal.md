## Why

A signed-out visitor browsing the feed sees no hint that the product can tell them
how well a job fits — the card-level coverage bar renders only for a signed-in
viewer with a skills profile, so the single most persuasive reason to sign in is
invisible exactly where the decision is made. The one place a guest does see a
teaser (the job-detail Profile match block) shows hardcoded `React / Docker / SQL /
Kafka` chips at a fixed `76%`, which contradicts the job on screen and reads as a
mock rather than as a preview of a real feature.

## What Changes

- Job cards show a blurred profile-match teaser (tinted skill chips plus the
  coverage strip) to the two locked viewer states — guest and signed-in-without-skills
  — where today they show nothing. A viewer with a real match is unaffected.
- The teaser's figures become **deterministic per job**, derived from the job's
  public slug: a coverage percent in 60–90 and a have/missing split over the job's
  **own** skills. Same job, same numbers on every render, on the server and in the
  browser, on the card and in the sidebar.
- The Profile match block's teaser drops its hardcoded skill names for the real
  skills of the job being viewed, capped to a single non-wrapping row.
- The blurred teaser is hidden from assistive technology and replaced there by the
  sign-in affordance, so a screen reader is never told a fabricated score.

## Capabilities

### New Capabilities
<!-- none: this extends an existing capability -->

### Modified Capabilities
- `job-profile-match`: the locked (guest / no-profile) states change from a static
  sidebar-only teaser to a deterministic, job-derived teaser rendered in both the
  sidebar block and the job card.

## Impact

- `web/src/lib/jobMatch.ts` — new pure `matchTeaser()` helper beside the existing
  `resolveMatchState` / `computeClientMatch`; `web/src/lib/jobMatch.test.ts` grows.
- `web/src/lib/components/JobMatchBar.svelte` — new `blurred` prop.
- `web/src/lib/components/JobRow.svelte` — chip tint and the blurred wrapper for the
  locked states.
- `web/src/lib/components/JobMatch.svelte` — the static teaser constant is removed.
- Frontend only: no API, no schema, no worker touched. The match endpoint is still
  never called for a locked viewer, so no new request volume.
