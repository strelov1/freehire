<script lang="ts">
  import { fade } from 'svelte/transition';
  import { Layers } from '@lucide/svelte';
  import type { Snippet } from 'svelte';

  // The mobile controls for a list page (jobs, companies, …): an inline toolbar at the
  // top of the list — the results total on the left, and (on the jobs list) a Swipe entry
  // on the right. Once that toolbar scrolls out of view, the Swipe entry re-reveals as a
  // floating edge tab so it stays reachable deep in the list. Filters are opened from the
  // header search box's All-filters trigger (see HeaderListSearch), not from here. Mobile-
  // only; the desktop sidebar aside carries filters there, so this shows only the total
  // (right-aligned) at md+. Render it at the top of the list column, outside the view's
  // status branches, so the controls stay reachable while the list is loading/empty/errored.
  //
  // `total` is null until the list is ready (then the count appears); `unit` is the
  // already-pluralised noun ("jobs" / "companies"). `onSwipe` is optional — pass it only
  // where a swipe deck exists (the standalone jobs list). `showDesktopTotal` is false when
  // the desktop layout already surfaces the total elsewhere (the company page's sidebar
  // stat), so the above-list line isn't shown twice; the mobile toolbar total is unaffected.
  // `sortControl` is an optional leading control (the jobs feed's sort selector) rendered
  // in the mobile toolbar and beside the desktop total; it shows even when `total` is null
  // so the control stays reachable while the list is empty or standing in a prompt.
  let {
    total,
    unit,
    onSwipe,
    showDesktopTotal = true,
    sortControl,
  }: {
    total: number | null;
    unit: string;
    onSwipe?: () => void;
    showDesktopTotal?: boolean;
    sortControl?: Snippet;
  } = $props();

  // Reveal the floating Swipe edge tab once the inline toolbar leaves the viewport. The
  // toolbar is `md:hidden`, so on desktop it never intersects and this stays true — but
  // the revealed tab is `md:hidden` too, so nothing shows there. Only the Swipe tab reads
  // `pinned`, so skip the observer entirely on lists without a swipe deck.
  let toolbarEl = $state<HTMLElement>();
  let pinned = $state(false);
  $effect(() => {
    const el = toolbarEl;
    if (!el || !onSwipe) return;
    const io = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry) pinned = !entry.isIntersecting;
      },
      { threshold: 0 },
    );
    io.observe(el);
    return () => io.disconnect();
  });
</script>

<!-- Mobile inline toolbar: total on the left, controls on the right. The Swipe entry is
     icon-only here (labelled for a11y) so the row stays on one line with the count and the
     sort control; the word would crowd it out on a narrow phone. -->
<div bind:this={toolbarEl} class="mb-3 flex items-center gap-2 md:hidden">
  {#if total !== null}
    <span class="shrink-0 whitespace-nowrap text-sm text-muted-foreground" aria-live="polite">
      <span class="font-semibold tabular-nums text-foreground">{total.toLocaleString()}</span>
      {unit}
    </span>
  {/if}
  <div class="ml-auto flex items-center gap-2">
    {@render sortControl?.()}
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

<!-- Desktop: the total (and any sort control) sit top-right above the list (filters
     live in the sidebar). Renders when there's a total OR a sort control to show, so
     the control stays visible on an empty/prompt list where the total is null. -->
{#if showDesktopTotal && (total !== null || sortControl)}
  <div class="mb-3 hidden items-center justify-end gap-3 md:flex">
    {#if total !== null}
      <span class="text-sm text-muted-foreground" aria-live="polite">
        <span class="font-semibold tabular-nums text-foreground">{total.toLocaleString()}</span>
        {unit}
      </span>
    {/if}
    {@render sortControl?.()}
  </div>
{/if}

<!-- Scroll-revealed floating control: Swipe (right), where present. Filters are reached
     from the header search box's trigger, which stays visible while scrolling. -->
{#if pinned && onSwipe}
  <button
    type="button"
    onclick={onSwipe}
    aria-label="Swipe mode"
    title="Swipe mode"
    transition:fade={{ duration: 150 }}
    class="fixed right-0 top-16 z-30 flex items-center py-2 pl-2 pr-3 text-muted-foreground transition-colors hover:text-foreground md:hidden"
  >
    <Layers class="size-4 shrink-0" />
  </button>
{/if}
