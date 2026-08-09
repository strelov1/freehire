## Context

Full background and the brainstorming trail (including two approaches that were considered and
rejected) live in `docs/superpowers/specs/2026-08-08-match-tailor-merge-design.md`. Summary:

The AI fit report currently has two homes — the standalone `/match/[slug]` page and the Tailor
workspace's Job Match tab, which re-embeds the same `MatchAnalysisFull` component read-only. Tailor
cannot bootstrap without a cached analysis; today it 409s and sends the candidate to `/match` to
produce one. This design removes the standalone page and moves that trigger into Tailor itself.

Two adjacent ideas were raised and explicitly rejected during brainstorming:

1. **Folding the Score (ATS-delta) tab into Job Match.** `tailor-ats-delta/spec.md` already
   documents why not: it "MUST NOT share a tab with the job-anchored match score... stacking them
   under one heading is what made the previous Verdict tab unreadable." This is prior, paid-for
   product learning, not a fresh opinion — it stands.
2. **Merging the `cvmatch` (deterministic, document-vs-posting) and `matchanalysis` (LLM,
   candidate-vs-job) scores into one number.** They score different subjects (the edited document
   vs. the candidate) and are not commensurable — averaging them would misrepresent both. Resolved
   instead by keeping the existing visual hierarchy (live score primary, fit snapshot secondary),
   which the Job Match tab already implements correctly.

Both are recorded here so the tasks phase doesn't quietly reopen them.

## Goals / Non-Goals

**Goals:**
- One entry point for "how do I match this job and tailor my CV for it": `/tailor/[slug]`.
- No page hop required to produce a first fit analysis for a vacancy.
- Old `/match/[slug]` and `/jobs/[slug]/fit` links keep working (redirect, not 404).

**Non-Goals:**
- Changing the Job Match tab's internal layout (live score + labelled snapshot stays as-is).
- Changing the Score/ATS-delta tab in any way — it keeps its own top-level tab, unchanged.
- Colored underline overlay of analysis findings on the CV preview (separate, later change — the
  anchor mechanism exists via `CvHtmlPreview.svelte`'s `highlightPaths`, but no analysis emits a
  path yet).
- Unifying the `cvmatch`/`matchanalysis` scoring engines.

## Decisions

**Redirect, don't 404, on the old page.** `/match/[slug]` becomes a permanent (308) redirect to
`/tailor/[slug]`, mirroring the existing `/jobs/[slug]/fit` → (today) `/match/[slug]` pattern. This
covers bookmarks and any external links already in the wild without a content negotiation on our
side.

**Frontend absorbs the 409, not the backend.** The tailoring bootstrap's contract (409 "run the fit
analysis first" when no cached analysis exists) is left alone — `cv-tailoring` isn't touched. The
`/tailor/[slug]` page catches that specific 409 (matched on message, as it already discriminates it
from the "no résumé" 409) and, instead of `goto`-ing to `/match`, opens the existing SSE fit stream
(`$lib/matchAnalysis.ts`) inline, then retries the bootstrap once the stream completes. This reuses
100% of the existing compute/streaming path; only the trigger site moves. Alternative considered:
make the bootstrap endpoint itself run the analysis synchronously when missing — rejected, because
the fit chain is a multi-second, three-stage LLM call and the bootstrap is expected to be fast; the
SPA already knows how to show streaming progress, the server endpoint does not.

**`MatchAnalysisFull` is not deleted or forked.** It keeps rendering inside the Job Match tab
exactly as it does today (`autoRun=false`, `stacked`, seeded from the cached analysis). Only its
former standalone-page caller goes away.

## Risks / Trade-offs

- **[Risk]** Deleting the page could silently break a deep link some external tool or saved search
  already points at. → **Mitigation**: the redirect, not a hard delete, and it's covered by the
  same pattern as the existing `/jobs/[slug]/fit` alias.
- **[Risk]** Moving the fit-analysis trigger into Tailor changes what "opening Tailor" costs (it
  may now kick off an LLM chain the first time). → **Mitigation**: this was already true one hop
  earlier (opening `/match` triggered it); the total number of LLM calls per candidate journey is
  unchanged, only the page that issues it moves. The existing monthly quota
  (`job-fit-analysis`'s 10-analyses/30-day limit) still applies unmodified.
- **[Risk]** A future contributor reopens the "fold Score into Job Match" idea without seeing why
  it was rejected. → **Mitigation**: recorded here and left untouched in `tailor-ats-delta/spec.md`;
  no spec delta touches that file in this change.

## Migration Plan

1. Land the frontend 409-handling change in `/tailor/[slug]` (safe on its own — the redirect
   pattern isn't wired to it yet, so this alone gives the workspace a self-serve path even if a
   candidate happens to arrive without an existing analysis via a link that doesn't go through
   `/match`).
2. Repoint `/jobs/[slug]/fit` to `/tailor/[slug]`.
3. Repoint every in-app link to `/match/[slug]` (`MatchSummary.svelte` and any other job-card/drawer
   entry points) to `/tailor/[slug]`.
4. Add the `/match/[slug]` → `/tailor/[slug]` redirect route.
5. Delete the standalone `/match/[slug]` page implementation (`+page.svelte`, `+page.server.ts`)
   only after step 4 lands, so there's no window where the route exists but serves nothing.

No data migration; no rollback beyond reverting the commits (no destructive schema or data change
in this scope).

## Open Questions

None outstanding — scope was narrowed twice during brainstorming (dropping the Score-tab merge and
the scoring-engine merge) specifically to close open questions before this document was written.
