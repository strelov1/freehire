## 1. The CTA rank table

- [x] 1.1 In `web/src/lib/autoApplyButton.test.ts`, write the failing table-driven test for
      a new `jobCtaPlan(state)` covering all six `AutoApplyButtonState` kinds: what the
      auto-apply button shows (rendered at all, label, primary, `Pro` marker, disabled) and
      what the external button shows (label, primary), per design.md's table.
- [x] 1.2 Add a failing test asserting the invariant the table exists to protect: never two
      primary buttons at once, and one in every kind that still leaves the reader an action
      — which, once `applied` was found to belong to every source rather than to auto-apply,
      is every kind but `queued`.
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
      `hidden lg:flex` on its own right-aligned row under the title in `JobView.svelte`.
      (First written as `ml-auto` on the title's own row; moved off it because the buttons'
      position then depended on the title's length.)
- [x] 3.2 Remove both CTAs from the `actionStrip` snippet, leaving Discussion, Report, Save
      and Add-to-list; confirm the tab row's shared `border-b` rule still reads as one edge
      across the column.
- [x] 3.3 ~~Verify the sub-`lg` path is untouched.~~ **Superseded by 3.4** — the phone's
      path was deliberately changed instead, so there was nothing left to hold still.
- [x] 3.4 Give the phone the same two controls: the sticky bottom bar carries whichever the
      plan made primary, and `actionStrip` gains a phone-only anchor for the other. Delete
      `undemotedExternalCta`, whose whole premise was that the phone had no auto-apply
      button. `flex-wrap` on that strip's mobile copy — five labels do not fit one
      phone-width line.
- [x] 3.5 Move the view and applied counters out of the sidebar into `postingMeta`, and
      collapse the wrapper left holding one child.

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
- [x] 5.2 Run the app and check the job page in the browser: at 1440px and 1024px, full tab
      labels, green `Auto-apply` with `Pro` beside an outline `Show origin` on a Greenhouse
      posting, plain green `Apply` on a non-Greenhouse one, the dates and counters reading
      as icons, and the pinned header on scroll agreeing with the title row. On a phone, the
      sticky bar carrying `Auto-apply` and the strip carrying `Show origin`. Check the `Pro`
      marker in both themes.
      Layout only — there is no automated coverage that the tab row keeps its labels, and a
      component test that mounts JobView would be the way to get it.
