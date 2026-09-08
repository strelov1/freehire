## Context

`internal/ingest/sources/source.go`'s `fullBoardListing` interface doc comment already states the bar precisely (Context for this file, not restated here — see the file itself and `full-board-listing-marker`'s own spec, added by this change). `taleo.go` already carries the exact pattern this fix applies to `teamtailor`: a page-ceiling error message ("still yielding requisitions after the page cap: walk truncated, not exhausted") and a matching `taleoEndlessFake` test double that forces the walk through its whole cap to prove the ceiling itself fails loudly. Both are mirrored here rather than reinvented.

`teamtailor.jobURLs` is the one listing walk both `Fetch` and `FetchNew` share — fixing it once fixes both callers.

The live measurement (Context in proposal.md) used a plain `curl` with a browser-shaped `User-Agent` against public `teamtailor.com` career pages — no stealth transport, no auth, confirming the listing itself needs neither; the production adapter's own `HTMLGetter` already works the same way.

## Goals / Non-Goals

**Goals:**
- Stop a real board from losing postings to a page cap that no longer matches reality.
- Bring `teamtailor` in line with the `fullBoardListing` bar using the exact fix shape already proven on `workday`/`careerplug`/`icims`/`taleo` — no new mechanism.
- Capture the marker's own contract as an `openspec` capability, since it was previously only in `AGENTS.md`/code comments — useful for every future adapter this ongoing audit reaches, not just this one.

**Non-Goals:**
- Fixing the other 7 audited-and-failing adapters (`hh`, `neogov`, `edjoin`, `workstream`, `bayt`, `peopleforce`, `gusto`). Confirmed with the user: `teamtailor`'s live bug is the priority for this change; the rest stay a follow-up.
- Finding a source-declared total for `teamtailor` to verify against. No such signal was found in the platform's public listing; pagination-to-empty-page is the proof this adapter uses, matching how several already-marked adapters (`greenhouse`, `lever`, `workable`, `ashby`) also rely on "no artificial cap" rather than a count check.
- Auditing or marking any of the ~163 still-untouched adapters, `apploi` (the largest by real volume) included.

## Decisions

**Raise `ttMaxPages` to 1000, not remove it.** A page ceiling is still the right safety backstop against a board that never returns an empty page — the fix is not "no ceiling," it's "a ceiling wide enough not to be reached by a real board, paired with treating reaching it as failure rather than success." 1000 is chosen as a wide multiple over the measured real maximum (~125-129 pages for the largest board found), the same proportional margin `taleo`'s own 200-page cap and `mycareersfuture`'s 1500 already use relative to their own measured maximums.

**The natural-end proof reads the RAW per-page link count, not the count of links newly added after cross-page dedup.** The original draft of this change kept the pre-existing `newLinks == 0` check (added strictly to skip already-seen links, but also doubling as the walk's stop condition) on the reasoning that it also catches a board that serves the same page forever for an out-of-range request. Found on review: that reasoning conflates two different signals. A page can be non-empty yet add nothing NEW for a reason that is NOT "the board repeats its last page" — a sort tie spanning a page boundary, for instance — and stopping there is unproven: a genuinely unseen posting could still sit on a page beyond it. Verified live against both currently-crawled boards this change already measured (`migen`, `tantor`): neither actually repeats a page past its end — both answer a page with zero raw links, a real empty page, not an echo. So switching the stop condition to the raw link count (checked before dedup) costs nothing against the one behavior the old check was defending against, and closes the unproven-duplicate-page gap the mechanical batch fix (`openspec/changes/fullboardlisting-hand-rolled-batch`) found and fixed for its five adapters. `jobURLs` still deduplicates when BUILDING the result list — only the stop condition changed.

**Both new failure paths return `fmt.Errorf`, not a typed sentinel error.** No caller of `teamtailor.Fetch`/`FetchNew` branches on a specific listing-failure reason today (unlike, say, `ErrPostingGone`, which the pipeline treats differently from an ordinary failure) — an ordinary wrapped error is what every other page-cap fix in this codebase (`taleo`, `careerplug`, `icims`) already returns for the identical situation, and inventing a sentinel here would be complexity nothing reads.

**The `fullBoardListing` marker's contract becomes its own new `openspec` capability, not folded into `job-lifecycle`'s existing sweep requirements.** `job-lifecycle`'s spec already covers the per-run unseen sweep and the separate chronic-board safety net, both by their own observable behavior — but the "which sources are trusted for the board-scoped variant, and what they must prove" question sits one layer below that, at the adapter-completeness level, and was never captured as an `openspec` requirement anywhere (only in `internal/ingest/sources/AGENTS.md` and `source.go`'s own comment). Adding it as its own capability keeps `job-lifecycle`'s spec about what the sweep DOES, and this new one about what makes a source eligible to participate in the sharper variant of it — a distinction the code itself already draws (`sources.FullBoardListingProviders` is a `sources`-package concern, not a `job-lifecycle`/pipeline one).

## Risks / Trade-offs

- **[Risk]** A `teamtailor` board larger than the ~125-129 pages measured could still exist, exceeding even the raised 1000-page ceiling. → **Mitigation**: unlike before this change, hitting that ceiling now fails the crawl loudly (logged, retried, cooled down like any other board failure) instead of silently truncating — the exact property the `fullBoardListing` bar is designed to guarantee. If it happens, it will be visible, not discovered by another live audit.
- **[Trade-off]** A genuinely broken board that always returns a full page of SOME non-empty content forever (e.g. a server bug that returns different junk every time, which the raw-link-count check cannot distinguish from real postings) would now cost up to 1000 requests before failing, versus 100 before. → Accepted: bounded, per-board, and only on an already-broken board; not a cost any healthy board pays.
