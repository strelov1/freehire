<script lang="ts">
  import { Layers } from '@lucide/svelte';
  import type { Snippet } from 'svelte';

  // The mobile controls for a list page (jobs, companies, …): an inline toolbar at the
  // top of the list — the results total on the left, and (on the jobs list) a Swipe entry
  // on the right. Filters are opened from the header search box's All-filters trigger (see
  // HeaderListSearch), not from here. Mobile-only; the desktop sidebar aside carries filters
  // there, so this shows only the total (right-aligned) at md+. Render it at the top of the
  // list column, outside the view's status branches, so the controls stay reachable while
  // the list is loading/empty/errored.
  //
  // `total` is null until the list is ready (then the count appears); `unit` is the
  // already-pluralised noun ("jobs" / "companies"). `onSwipe` is optional — pass it only
  // where a swipe deck exists (the standalone jobs list). `showDesktopTotal` is false when
  // the desktop layout already surfaces the total elsewhere (the company page's sidebar
  // stat), so the above-list line isn't shown twice; the mobile toolbar total is unaffected.
  // `controls` is an optional slot for the list's own controls — however many the view
  // passes (the jobs feed's sort select, freshness select and evergreen toggle; the
  // company catalog's sort select) — rendered in the mobile toolbar and beside the
  // desktop total. It shows even when `total` is null so the controls stay reachable
  // while the list is empty or standing in a prompt.
  let {
    total,
    unit,
    onSwipe,
    showDesktopTotal = true,
    controls,
  }: {
    total: number | null;
    unit: string;
    onSwipe?: () => void;
    showDesktopTotal?: boolean;
    controls?: Snippet;
  } = $props();
</script>

<!-- Mobile inline toolbar: total on the left, controls on the right. The Swipe entry is
     icon-only here (labelled for a11y) so the row stays on one line with the count and the
     list controls; the word would crowd it out on a narrow phone — and the jobs list's
     evergreen toggle drops its word for the same reason.

     `flex-wrap` is the safety net under that: the controls are sized by their content
     (a long count, a translated label, a fourth control) and a row that runs out of width
     must break onto a second line rather than clip its rightmost control off-screen. -->
<div class="mb-3 flex flex-wrap items-center gap-2 md:hidden">
  {#if total !== null}
    <span class="shrink-0 whitespace-nowrap text-sm text-muted-foreground" aria-live="polite">
      <span class="font-semibold tabular-nums text-foreground">{total.toLocaleString()}</span>
      {unit}
    </span>
  {/if}
  <!-- Wraps too, not just the outer row. Without this the controls are one indivisible
       block: the row can drop the whole block to a second line but the block itself
       still overflows, so a longer control (a translated select value, a fourth entry)
       clips exactly as before. `justify-end` keeps a wrapped line right-aligned under
       the `ml-auto`. -->
  <div class="ml-auto flex flex-wrap items-center justify-end gap-2">
    {@render controls?.()}
    {#if onSwipe}
      <button
        type="button"
        onclick={onSwipe}
        aria-label="Swipe mode"
        title="Swipe mode"
        class="inline-flex items-center rounded-lg border border-border bg-card px-2.5 py-2 text-sm font-medium transition-colors hover:bg-accent"
      >
        <Layers class="size-4 shrink-0" />
      </button>
    {/if}
  </div>
</div>

<!-- Desktop: the total (and any list controls) sit top-right above the list (filters
     live in the sidebar). The two are gated INDEPENDENTLY: the total on `showDesktopTotal`
     (a view that renders its own total elsewhere suppresses this one), the controls on
     their own presence. Gating both on `showDesktopTotal` is what hid the controls
     entirely on a company page, where the sidebar carries the count. -->
{#if (showDesktopTotal && total !== null) || controls}
  <div class="mb-3 hidden items-center justify-end gap-3 md:flex">
    {#if showDesktopTotal && total !== null}
      <span class="text-sm text-muted-foreground" aria-live="polite">
        <span class="font-semibold tabular-nums text-foreground">{total.toLocaleString()}</span>
        {unit}
      </span>
    {/if}
    {@render controls?.()}
  </div>
{/if}
