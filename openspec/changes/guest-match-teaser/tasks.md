## 1. The teaser derivation

- [x] 1.1 Add `matchTeaser(seed, jobSkills)` to `web/src/lib/jobMatch.ts`, test-first in `web/src/lib/jobMatch.test.ts`: determinism for one seed, percent within 60–90 across many seeds, `matched === round(percent/100 × total)`, at least one held skill and at least one missing, `null` for a job with fewer than two skills, and a spread of percents across distinct slugs
- [x] 1.2 Confirm the returned `{percent, matched, total}` is assignable to the existing `ClientMatch` shape consumed by `JobMatchBar`, so the bar needs no second match type

## 2. The card teaser

- [x] 2.1 Add a `blurred` prop to `web/src/lib/components/JobMatchBar.svelte`: light blur, non-interactive, `aria-hidden` on the strip, and a visually-hidden sign-in invitation in place of the announced percentage
- [x] 2.2 In `web/src/lib/components/JobRow.svelte`, derive the teaser for the `guest` and `no-profile` states and pass it to `JobMatchBar` with `blurred`; a `ready` viewer keeps the real client-computed match unblurred
- [x] 2.3 Tint the card's skill chips from the teaser's missing set in the locked states, and blur the chips container only — the salary in the same row stays legible

## 3. The sidebar teaser

- [x] 3.1 Replace the hardcoded teaser constant in `web/src/lib/components/JobMatch.svelte` with `matchTeaser(job.public_slug, job.skills)`, wiring the percent, the bar width, and the "N of M skills" label to it
- [x] 3.2 Render the teaser chips from the job's own skills, capped at three at natural width (clipped, not ellipsised) so the row cannot wrap
- [x] 3.3 Add `teaserChips` so a capped row still carries a missing skill, and drop the teaser entirely for a single-skill job (no have/missing contrast to draw)

## 4. Verification

- [x] 4.1 `pnpm test` and `pnpm lint` green in `web/`; `pnpm build` succeeds
- [x] 4.2 Visually verify the guest state against the production API: the feed's blurred chips and strip with the salary still crisp, the sidebar teaser on a six-skill job, and the bare call-to-action on a single-skill job
- [ ] 4.3 Verify the two signed-in states (no profile skills, and a real match) in a browser — needs a session, so not covered by the guest pass above; the `ready` markup is unchanged and `no-profile` shares the guest branch apart from its call-to-action
