## 1. The gauge projection

- [x] 1.1 In `web/src/lib/ghost.test.ts`, write failing tests for a `ghostGauge` projection: it returns one segment per `criteria_total`, fills exactly `criteria.length` of them, escalates tone as the fired count rises, gives one-of-four and two-of-four distinct tones, and returns null for a signal `ghostBadge` refuses.
- [x] 1.2 Implement `ghostGauge` in `web/src/lib/ghost.ts` returning `{ segments, filled, tone }`, with tone derived from the fired count and documented as the reason it is not derived from `level`.

## 2. The gauge row

- [x] 2.1 Rewrite `web/src/lib/components/GhostChecklist.svelte` as a collapsed row — gauge, hedged wording, `fired/total` scale, disclosure button — with the existing checklist and caveat rendered only when expanded.
- [x] 2.2 Render the gauge segments from `ghostGauge`: filled segments in the escalating tone, unfilled in the neutral border tone, the whole gauge `aria-hidden` since the wording and scale carry the same information as text.
- [x] 2.3 Wire the disclosure: a `$state` boolean toggled by a `<button>` carrying `aria-expanded` and `aria-controls` pointing at the checklist region.

## 3. Job page placement

- [x] 3.1 In `web/src/lib/components/JobView.svelte`, swap `GhostBadge` for the gauge row in the header slot (keeping `supersedesReality` as the ghost-vs-reality switch) and delete the section that rendered the panel before the description, updating both surrounding comments to match.
- [x] 3.2 Confirm `GhostBadge.svelte` still has a caller (`JobRow`) and leave it untouched.

## 4. Trim the expanded view

- [x] 4.1 Test-drive `detailFor` returning an empty detail where firing IS the fact, and lower-case facts that read as a continuation of the criterion's own line.
- [x] 4.2 Give each criterion a `short` name and collapse the unfired ones into one "No data on …" line, replacing four rows that each said it.
- [x] 4.3 Drop the inner border in favour of a rule, and replace the caveat sentence with a link to `/features/ghost-jobs`.

## 5. Keep the explaining page honest

- [x] 5.1 Replace `GhostLandingView`'s hand-copied chip and checklist markup with `GhostBadge` and `GhostChecklist` fed fixture `Ghost` payloads.
- [x] 5.2 Move the caveat sentence out of the copied panel and onto the page beside the preview.

## 6. Verify

- [x] 6.1 `cd web && pnpm test` green, `pnpm lint` and `pnpm build` clean.
- [x] 6.2 Visually verify the job page and the landing — collapsed row one line tall, expanded checklist complete, both light and dark, at desktop and mobile widths — using headless Chrome over CDP.
- [x] 6.3 Delete the throwaway `dev-ghost-gauge` route.

## 7. Act on review

- [x] 7.1 Keep the disclosed region mounted (`hidden`/`flex`, not `{#if}`) so `aria-controls` resolves in the collapsed state, and verify it over CDP.
- [x] 7.2 Replace "No data on …" with `ghostUnobserved`'s "Not observed: …" — the payload cannot support the no-data reading — and pin the wording with a test.
- [x] 7.3 Restore `<ul>/<li>` for the fired criteria, `aria-hidden` the tick, and separate the two lowest gauge tones.
- [x] 7.4 Correct the comments and spec sentences the change had made false: `JobView`'s caveat claim, `ghostSignals`' `fact` docstring, design.md's caveat contradiction, and the delta spec's "facts behind every fired criterion".
