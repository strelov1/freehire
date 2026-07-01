## 1. Implement the mobile filters tab

- [x] 1.1 In `JobsView.svelte`, import lucide `SlidersHorizontal` and add a
      `md:hidden` icon-only tab pinned `fixed left-0 top-20` (mirroring the swipe
      tab) that opens `drawerOpen`, with a `bg-primary` corner badge showing
      `filters.active` when `> 0`.
- [x] 1.2 Remove the inline mobile Filters button; render the `mb-4` row only for
      the embedded (non-standalone) view holding its inline search `Input`, so the
      standalone list has no empty top offset.
- [x] 1.3 Add `pl-12 md:pl-0` to the job-count line so the left tab does not cover
      it on mobile.

## 2. Verify

- [x] 2.1 `svelte-check` passes with 0 errors.
- [x] 2.2 Headless mobile-viewport screenshot of `/jobs` confirms: left filters
      tab with badge, no overlap with the swipe tab, job count clear of the tab,
      and no empty top offset.
