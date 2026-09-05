## 1. The CTA rank table

- [x] 1.1 In `web/src/lib/autoApplyButton.test.ts`, write the failing table-driven test for
      a new `jobCtaPlan(state)` covering all six `AutoApplyButtonState` kinds: what the
      auto-apply button shows (rendered at all, label, primary, `Pro` marker, disabled) and
      what the external button shows (label, primary), per design.md's table.
- [x] 1.2 Add a failing test asserting the invariant the table exists to protect: never two
      primary buttons at once, and one in every kind that still leaves the reader an action
      — which is every kind but `queued` and `applied`.
- [x] 1.3 Export `JobCtaPlan` and `jobCtaPlan` from `web/src/lib/autoApplyButton.ts` to make
      1.1 and 1.2 pass, with a comment saying why `declined`/`failed` re-promote the
      external button.

## 2. The buttons

- [x] 2.1 Make `applyCta` take the plan's `external` half: `Apply`/`Show origin` label and
      `primary`/`outline` variant. Destination, `rel`, `target`, `onApplyClick` unchanged.
- [x] 2.2 Make `autoApplyCta` take the plan's `autoApply` half: label from the plan instead
      of its own `{#if}` chain, `primary` when the plan says so, and the `Pro` marker span
      (`bg-foreground text-background`) only when `pro` is set.

## 3. The layout

- [x] 3.1 Add a `ctaGroup` snippet rendering `autoApplyCta` then `applyCta`, and render it
      `ml-auto hidden lg:flex` inside the header block's title row in `JobView.svelte`.
- [x] 3.2 Remove both CTAs from the `actionStrip` snippet, leaving Discussion, Report, Save
      and Add-to-list; confirm the tab row's shared `border-b` rule still reads as one edge
      across the column.
- [x] 3.3 Verify the sub-`lg` path is untouched: the quiet strip still renders under the
      title and the sticky bottom bar still carries the apply CTA.

## 4. The posting's dates

- [x] 4.1 In `web/src/lib/utils.test.ts`, write the failing test for a short style on
      `timeAgo` and `formatDateOrAgo`; then add the optional `style` argument, passing
      `style: 'short'` through to a second cached `Intl.RelativeTimeFormat`.
- [x] 4.2 Rewrite the `postingDates` snippet in `JobView.svelte` as a clock icon and a
      refresh icon, each with the short relative time, an `sr-only` label carrying the word
      it replaced, and a `title` carrying the field name and the exact timestamp.
- [x] 4.3 Drop the duplicate: stop passing `postedAt` to `RealityBadge` in `JobView.svelte`,
      then remove the now-orphaned `postingContrast` (`web/src/lib/reality.ts`), its tests,
      and `RealityBadge`'s `postedAt` prop.

## 5. Verify

- [x] 5.1 `pnpm --dir web test` and `pnpm --dir web lint` green.
- [x] 5.2 Run the app and check the job page in the browser at desktop width: full tab
      labels, green `Auto-apply` with `Pro` beside an outline `Show origin` on a Greenhouse
      posting, plain green `Apply` on a non-Greenhouse one, the dates reading as two icons,
      and the phone layout unchanged. Check the `Pro` marker in both themes.
