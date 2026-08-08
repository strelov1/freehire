## Why

`/match/[slug]` and the Tailor workspace's Job Match tab both show the AI fit report — the Job
Match tab re-embeds `MatchAnalysisFull` verbatim, read-only, beneath the workspace's own live
document score. Getting from "here's a job" to "here's my tailored CV" requires a full page hop
through `/match` before Tailor will even bootstrap: the tailoring bootstrap 409s with "run the fit
analysis first" when no cached analysis exists, and the frontend sends the candidate to `/match`
to produce one, then back. Removing `/match` and letting the Tailor workspace run the fit analysis
itself collapses two page hops into one entry point.

## What Changes

- Delete the standalone `/match/[slug]` route (`+page.svelte` + `+page.server.ts`).
- Add a permanent redirect at `/match/[slug]` → `/tailor/[slug]` for existing outbound links
  (job cards, `JobDrawer`, `SwipeDeck`, `MatchSummary`'s "view fit" links, and any bookmarked URLs).
- Repoint the existing `/jobs/[slug]/fit` legacy redirect from `/match/[slug]` to `/tailor/[slug]`
  (it currently 308s to `/match/[slug]`, which would otherwise redirect a second time).
- **BREAKING**: the tailoring bootstrap's `409 "run the fit analysis first"` response is no longer
  handled by navigating away from Tailor. The Tailor workspace now runs the same SSE fit-analysis
  stream itself on catching that 409, then retries the bootstrap — the candidate never leaves
  the workspace. The bootstrap's server contract (still 409s without a cached analysis) is
  unchanged; only the frontend's response to it changes.
- The Job Match tab's internal layout (live score above the labelled fit-analysis snapshot) is
  **unchanged** — it was already the correct separation. The Score (ATS-readability) tab is
  **explicitly not folded in**: `tailor-ats-delta` already forbids sharing a tab with the
  job-anchored match score, a lesson kept from a prior "Verdict tab" design that mixed them and
  was reverted for being unreadable.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `job-fit-analysis`: the "Dedicated fit analysis page" and "Sidebar reduced to a summary linking
  to the page" requirements described a standalone page (drifted in the current spec text to the
  superseded `/jobs/[slug]/fit` path; the page actually lives at `/match/[slug]` today). Both
  requirements are replaced: there is no dedicated fit-analysis page any more, and the entry point
  for running/viewing the analysis is the Tailor workspace's Job Match tab, which triggers the
  stream itself when no cached analysis exists.
- `tailor-workspace`: the CV-list requirement's premise that "a tailored CV is created only from
  the match page" no longer holds — a tailored CV (and its fit analysis) can now be created
  directly by opening `/tailor/[slug]` for a vacancy that has none yet.

## Impact

- Removed: `web/src/routes/match/[slug]/+page.svelte`, `+page.server.ts`.
- Changed: `web/src/routes/jobs/[slug]/fit/+page.ts` (redirect target); a new redirect at
  `web/src/routes/match/[slug]/+page.ts`; `web/src/routes/tailor/[slug]/+page.svelte` (409 handling
  runs the fit stream inline instead of `goto`-ing away).
- Changed link targets: `web/src/lib/components/MatchSummary.svelte` (two `resolve('/match/[slug]', ...)`
  call sites) and any other job-card/drawer entry points that link to `/match/[slug]`.
- Unaffected: `ArtifactPanel.svelte` tab set and the Score/ATS-delta tab, `MatchAnalysisFull.svelte`
  itself (still used, just no longer given its own route), `cvmatch`/`matchanalysis` scoring logic,
  `/my/activity/matches`, `/my/profile`'s `ATSReportView`.
