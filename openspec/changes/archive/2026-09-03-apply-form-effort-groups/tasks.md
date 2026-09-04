## 1. The grouping model

- [x] 1.1 Write the failing test `web/src/lib/applyFormGroups.test.ts` covering the
      partition: each `answer` value from the closed vocabulary lands in its group
      (`""` and any unknown value in Short answers; `choose one` / `choose any` /
      `yes / no` in Pick from a list; `written answer` in Written answers; `upload`
      in Attachments), groups come back cheapest-first, and a group no question
      falls into is absent from the result rather than present and empty.
- [x] 1.2 Write `web/src/lib/applyFormGroups.ts` to pass it — a pure module, no
      runes, no `$app/*` import, so the Node-only vitest config can load it.
- [x] 1.3 Extend the test: within a group the questions keep the served order, and
      each group carries its own count.
- [x] 1.4 Extend the test: the summary figures — total questions, and the number
      demanding a written answer — are derived from the same input, and the
      written-answer figure is reported as absent (not zero) when none demand one.
- [x] 1.5 Extend the test: the result says whether headings should render, false
      when exactly one group is non-empty and false for an empty question list.

## 2. The provider mark

- [x] 2.1 Write the failing test `web/src/lib/atsmarks.test.ts`: `greenhouse`
      resolves to a mark whose title is Greenhouse, and each of `ashby`,
      `workable`, `lever`, `recruitee` and an unknown provider resolves to nothing
      — asserting the absence deliberately, since silent partial coverage is the
      behaviour being relied on.
- [x] 2.2 Write `web/src/lib/atsmarks.ts` to pass it: a named map from provider slug
      to a `simple-icons` mark, mirroring `web/src/lib/techmarks.ts`, carrying a
      comment on why only one entry exists and what verified it (`siGreenhouse`'s
      `source` is `brand.greenhouse.io/brand-portal`).

## 3. The rendering

- [x] 3.1 Render the caption line in `JobApplyForm.svelte`: the provider's mark via
      `BrandMark` where `atsmarks` knows one and nothing where it does not, the
      provider's name, and the summary figures from `applyFormGroups`.
- [x] 3.2 Render the groups: a heading per group carrying its name and count, the
      headings omitted entirely when the model says so, and the standard-fields
      line kept as its own group so it is not orphaned once headings exist.
- [x] 3.3 Drop the per-question answer-kind hint, leaving `optional` alone on the
      row; key each group's `{#each}` by index, and carry forward the comment
      explaining why (the `each_key_duplicate` crash).
- [x] 3.4 Confirm `applyFormWorthShowing` and its exported contract are untouched,
      since `JobView.svelte` gates the whole tab on it.

## 4. Verification

- [x] 4.1 `pnpm --dir web test` green, `pnpm --dir web check` clean, eslint clean on
      the touched files.
- [x] 4.2 `pnpm check:dead` — both new modules are imported, so neither reads as a
      dead file.
- [x] 4.3 Look at the block in a browser on a Greenhouse posting (mark, summary,
      several groups) and on a posting from a provider with no mark, confirming the
      caption line degrades to text alone.
