# Match/Tailor page merge — design

## Problem

The job-match/CV-tailoring experience is spread across four places that don't know about each other:

- `/match/[slug]` — full AI fit report (`MatchAnalysisFull`).
- Tailor's **Job Match** tab — the same `MatchAnalysisFull` re-embedded read-only, stacked under a separate live deterministic score (`JobMatch.svelte` / `cvmatch`).
- Tailor's **Score** tab — an unrelated ATS-readability delta (`AtsDelta.svelte`) plus `AutopilotReport`.
- `/my/profile` — a fourth, job-agnostic ATS report (`ATSReportView`), out of scope here.

Getting from "here's a job" to "here's my tailored CV" requires a full page hop (`/match` → `/tailor`), and the Job Match tab duplicates the Match page verbatim instead of linking to it. This spec removes the standalone Match page and reorganizes Tailor's right panel so there is one home for match/fit/ATS information.

## Out of scope

- **Colored underline overlay** marking specific problem spans inside the CV preview. The anchor mechanism already exists (`CvHtmlPreview.svelte`'s `highlightPaths` prop + `.cv-lit` dotted-underline styling, currently used for revision diffing) and is reusable, but none of the three issue-producing analyses (ATS delta, match analysis, experience evidence) currently emit a path into that vocabulary — that's backend work in the analyzers. Separate spec, follows this one.
- **Merging the scoring engines themselves.** `cvmatch` (deterministic, scores the *tailored document text* against the posting, moves as you edit) and `matchanalysis` (LLM-driven, scores the *candidate* against the job from the base résumé, doesn't move on edit) measure genuinely different things — one document quality, one candidate fit. Averaging them into one number would misrepresent both. Resolved here instead by picking one as the visually primary ("hero") number and demoting the other to a supporting block, not by mechanical unification.
- `/my/profile`'s `ATSReportView` (job-agnostic) is untouched.

## Routing

- Delete the `/match/[slug]` route (`web/src/routes/match/[slug]/+page.svelte` + `+page.server.ts`).
- `/jobs/[slug]/fit` (legacy redirect) now 308s to `/tailor/[slug]` instead of `/match/[slug]`.
- Every current entry point that links to `/match/[slug]` — job cards, `JobDrawer.svelte`, `SwipeDeck.svelte`, `MatchSummary.svelte`, the "Tailor my CV" CTA — points to `/tailor/[slug]` instead.
- Add a plain redirect at `/match/[slug]` → `/tailor/[slug]` for old links already out in the wild (bookmarks, shared URLs), same pattern as the existing `/jobs/[slug]/fit` redirect.

## Tailor bootstrap

Today, opening `/tailor/[slug]` requires a cached fit analysis to already exist; if not, the backend returns 409 and the frontend `goto`s back to `/match/[slug]` to run it. With `/match` gone, `/tailor/[slug]` must trigger the AI fit compute itself on first open when no cached analysis exists, instead of redirecting elsewhere. This reuses the existing SSE-streamed compute path (`$lib/matchAnalysis.ts`) that `/match` used to kick off — only the trigger site moves.

## Tab structure

`ArtifactPanel.svelte` top-level tabs change from `jobmatch | score | history | jd` to:

**`jobmatch | history | jd`**

The `score` tab is removed as a top-level tab; its content moves inside `jobmatch` (see below). `history` and `jd` are unchanged.

`jobmatch` becomes the default tab (already is). Inside it, a sub-tab switcher with three panes:

| Sub-tab | Default | Content | Source |
|---|---|---|---|
| **Score** | yes | Live deterministic score of the current tailored text vs. the posting — the "hero" number, large, updates as the CV is edited. | `JobMatch.svelte` / `cvmatch` (unchanged) |
| **Fit** | no | AI fit snapshot — verdict, per-dimension scores, strengths/gaps. Explicitly a snapshot of the base profile; doesn't move as you edit. | `MatchAnalysisFull.svelte` (`autoRun=false`, `stacked`), unchanged, just relocated |
| **ATS** | no | ATS readability delta (base vs. tailored PDF text layer) + autopilot report. | `AtsDelta.svelte` + `AutopilotReport.svelte`, unchanged, just relocated from the old top-level `score` tab |

Rationale for Score-as-hero: Tailor is where the user is actively editing text, so the number that responds to their edits belongs up front. The AI fit verdict is a heavier, less immediate signal — useful, but supporting rather than primary in this specific context. (On job-browsing surfaces like cards/drawer, the AI fit teaser remains the primary signal shown today — this change only affects the Tailor workspace's internal layout.)

Naming note for implementation: the old top-level `score` tab (AtsDelta) and the new `Score` sub-tab (cvmatch) are different things with the same word — pick distinct internal keys (e.g. `ats` vs `score`) to avoid confusion in the component/prop names, even though the user-facing label for the hero sub-tab can stay "Score."

## Non-goals / explicit decisions already made

- No shared cross-page store is introduced. Each page still fetches its own data; the simplification is in navigation and layout, not in data-fetching architecture.
- `/my/activity/matches` (history list) is unaffected.
