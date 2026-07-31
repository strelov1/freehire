## 1. Resolve the test-harness unknown first

- [x] 1.1 Prove whether vitest resolves a module that imports `.svelte` components — ANSWERED NO: the `$lib` alias does not resolve under vitest, and a relative import reaches `vite:import-analysis` untransformed because the svelte plugin does not apply in the test environment. Recorded in `design.md` under Decisions
- [x] 1.2 The documented fallback also fails — importing the registry for its keys pulls its `.svelte` imports along. Replaced by a compile-time invariant: `CRITERIA` gains `as const satisfies`, a `GhostCriterionCode` union is derived from it, and the dispatcher's registry is typed `Record<GhostCriterionCode, Component>` so a missing diagram is `TS2741`. No second list of criterion codes is introduced
- [x] 1.3 Apply that change to `web/src/lib/ghost.ts`: `as const satisfies readonly {...}[]` on `CRITERIA`, plus an exported `GhostCriterionCode`; confirm `ghostChecklist` and `ghostSignals` still compile against the now-readonly array

## 2. The level rule, test-first

- [x] 2.1 RED: extend `web/src/lib/ghost.test.ts` with `ghostLevel(criteria, contributors)` covering all four gate combinations — neither gate, convergence only, witnesses only, both
- [x] 2.2 RED: add the property test that no structural-only criteria set yields `likely`, exercising every subset of the structural codes
- [x] 2.3 GREEN: implement `ghostLevel` in `web/src/lib/ghost.ts` beside `CRITERIA`, with a comment naming `internal/ghost` as the authority it mirrors
- [x] 2.4 Move `CONVERGENCE` and `WITNESS_GATE` from `ghostSignals.ts` into `ghost.ts`; run the full web suite. The re-export planned here was added and then removed in task 8.7: once the gate matrix interpolated the constants instead of the page quoting them in prose, every consumer imported from `ghost.ts` directly and the re-export was left feeding only its own test

## 3. Copy vocabulary

- [x] 3.1 RED: extend `web/src/lib/ghostSignals.test.ts` to require a non-empty `gist` on every criterion as well as a `why`. The diagram-per-criterion invariant is NOT tested here — task 1 settled it as a compile-time check
- [x] 3.2 GREEN: add `gist` to `SignalExplainer` and write one ~25-word summary per criterion, leaving every existing `why` unchanged
- [x] 3.3 Add `web/src/lib/ghostFaq.test.ts` modelled on `tailorFaq.test.ts` — non-empty questions and answers, no duplicate questions, and the FAQPage payload matching the visible list. The intent-word blacklist that guards GHOST_SIGNALS is deliberately absent: a FAQ voices the reader's suspicion in order to refuse it, so the check flags good copy and can only be satisfied by blunting it. Reasoning recorded in the file

## 4. Pure geometry

- [x] 4.1 RED: add `web/src/lib/ghostDiagrams.test.ts` asserting the waffle model yields exactly 100 cells, that the solid count matches the range's lower bound and the hatched count the width of the band
- [x] 4.2 RED: extend that test so the gate matrix model's four cells are produced by `ghostLevel` rather than literal strings, and so each cell's axis labels are checked against the example it carries — a mislabelled axis leaves every level correct and the diagram still wrong
- [x] 4.3 GREEN: implement the waffle and matrix models in `web/src/lib/ghostDiagrams.ts`, following the `activityChart.ts` precedent — geometry in `.ts`, no rendering

## 5. Diagram components

- [x] 5.1 Add `web/src/lib/components/ghost/Prevalence.svelte` and `GateMatrix.svelte` as dumb renderers of the task-4 models
- [x] 5.2 Add `diagrams/Evergreen.svelte` — a stack of concurrent copy silhouettes and a repost count, with NO time axis
- [x] 5.3 Add `diagrams/AtsAbsent.svelte` — three states: role found, role absent from a crawled board with a check-age stamp, and a set-apart muted panel for a board not crawled and therefore not judged
- [x] 5.4 Add `diagrams/Silent.svelte` — three lanes: window elapsed with no reply (counts), reply arrived (does not count), no connected mailbox drawn hollow (not counted at all)
- [x] 5.5 Add `diagrams/Reports.svelte` — one contributor versus two, where the count field below the gate is drawn as carrying no value rather than as a zero
- [x] 5.6 Wire the registry so `SignalDiagram.svelte` dispatches on criterion code, typed `Record<GhostCriterionCode, Component>`; verify by temporarily deleting one entry that `pnpm run check` reports `TS2741`, then restore it
- [x] 5.7 Verify every diagram against the colour rule: nothing not-fired, not-counted or not-judged may render in a reassuring tone, and no green appears anywhere

## 6. Disclosure primitive

- [x] 6.1 Add `web/src/lib/components/ghost/Disclosure.svelte` wrapping native `<details>/<summary>` for styling only
- [x] 6.2 Confirm it operates with client JavaScript disabled and that collapsed content is present in the server-rendered HTML
- [x] 6.3 Leave `GhostChecklist.svelte` on its hand-rolled disclosure; add no migration

## 7. The sandbox

- [x] 7.1 Add `web/src/lib/components/ghost/GhostSandbox.svelte` — four criterion toggles plus a three-position contributor control (none / one / enough)
- [x] 7.2 Assemble a `Ghost` from that state via `ghostLevel` and render the real `GhostBadge` and `GhostChecklist`, importing them rather than reproducing their markup
- [x] 7.3 Confirm by hand: both structural criteria with no contributors holds at the lower level with no control able to raise it, and one contributor renders no count

## 8. Page restructure

- [x] 8.1 Rewrite `GhostLandingView.svelte` to the five-section order — hero, criteria, how the level is decided, your part, where it is blind — plus FAQ and closing CTA
- [x] 8.2 Compress "the line we do not cross" into a two-line contrast inside the hero, keeping it above the mechanics; place the prevalence waffle beneath it
- [x] 8.3 Build the criteria section as two tier-labelled groups, each cell carrying diagram, name, `fact`, `gist` and a disclosure holding `why`
- [x] 8.4 Replace the "what you actually see" section and the tier-explaining paragraph with the gate matrix and the sandbox
- [x] 8.5 Reduce "your part" to two numbered items — report manually, connect a mailbox — keeping the "without a connected mailbox nothing is counted" wording verbatim
- [x] 8.6 Move the FAQ onto `Disclosure`, collapsed, still rendering from `GHOST_FAQ`; leave the route file's `<Seo>` and JSON-LD untouched
- [x] 8.7 Delete the markup orphaned by the restructure and confirm no unused import or dead helper is left behind

## 9. Verification

- [x] 9.1 Run the full web test suite; run `pnpm run check` and the linter, comparing against the pre-change baseline rather than expecting zero
- [x] 9.2 Visually verify light and dark themes, checking that hatching, hollow lanes and the empty count slot stay distinguishable in dark
- [x] 9.3 Verify at a real narrow viewport rather than trusting a `--window-size` flag below ~500px
- [x] 9.4 Walk the primary entry path end to end — done against production after deploy. On a live marked job the row reads `Possibly inactive 2/4 · Details`, names the two fired criteria, summarises the rest as `Not observed: applications here, reports from people.` and links `How this works` to the landing. No occurrence of the word *ghost* and no accusatory vocabulary anywhere in the signal's own UI
- [x] 9.5 Confirm the served HTML still contains every FAQ answer and every criterion `why` with nothing expanded, and that the FAQ structured data still matches the visible list

## 10. Close out

- [x] 10.1 Request code review on the whole diff and act on Critical and Important findings. Fixed: the lint ERROR that broke CI (my verification grepped for "warning" and so never saw it — check the exit code, not a summary of the output); a new sentence claiming nothing shows below two criteria, which the gate matrix three lines below it visibly refutes; the sandbox guard running only one way; the waffle band separated from the remainder by hue alone in dark; four disclosures sharing one accessible name; the matrix rendered as divs rather than a table. Declined: typing EXPLAINERS by the criterion union, because dropping an unexplained criterion rather than failing to compile is documented behaviour and changing it is not this change's business
- [x] 10.2 Finish the branch, then archive and sync the change — merged as #1336, deployed to prod (blue), verified live
- [ ] 10.3 Offer a changelog entry — the page is user-facing, and the ghost signal's own entry was deferred until the signal actually fired
