<script lang="ts">
  // Prev/Next stepper over a bound `page`, for a list paged in local state:
  // nothing here touches the URL and there are no links, so the pages it steps
  // through exist only for as long as the component is mounted.
  //
  // NOT the thing a public listing wants. `web/src/lib/components/Pagination.svelte`
  // is the other shape — real `<a href="?page=N">` in the SSR markup, with window
  // elision and rel=prev/next — and it exists because a crawler pages by following
  // links, and a button it cannot press advertises nothing. The two were both called
  // Pagination, which read as duplication and invited a merge that would have traded
  // that away; this one is `Pager` now, after what it actually is.
  //
  // Dormant, and unhardened because of it: nothing has ever rendered this, so `perPage`
  // is taken on trust. A `0` makes `totalPages` Infinity and leaves Next enabled
  // forever. Guard it — and test the clamp effect at runtime, not only at init —
  // before the first consumer lands, rather than after it finds out.
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    page = $bindable(1),
    total,
    perPage = 20,
    class: className,
    children,
  }: {
    page?: number;
    total: number;
    perPage?: number;
    class?: string;
    children?: Snippet;
  } = $props();

  let totalPages = $derived(Math.max(1, Math.ceil(total / perPage)));
  let canPrev = $derived(page > 1);
  let canNext = $derived(page < totalPages);

  // `total` shrinks whenever a filter narrows the result set, which can strand
  // `page` past the end — "Page 7 of 3", with Next disabled and nothing to show.
  // Clamping here keeps the bound value honest for the consumer's query too.
  $effect(() => {
    const clamped = Math.min(Math.max(page, 1), totalPages);
    if (clamped !== page) page = clamped;
  });

  function prev() {
    if (canPrev) page--;
  }
  function next() {
    if (canNext) page++;
  }
</script>

<nav class={cn('flex items-center gap-2', className)} aria-label="Pagination">
  <button
    type="button"
    onclick={prev}
    disabled={!canPrev}
    class="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm transition-colors hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
    aria-label="Previous page"
  >
    Previous
  </button>
  <span class="text-sm text-muted-foreground">
    Page {page} of {totalPages}
  </span>
  <button
    type="button"
    onclick={next}
    disabled={!canNext}
    class="inline-flex h-9 items-center justify-center rounded-md border border-border px-3 text-sm transition-colors hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
    aria-label="Next page"
  >
    Next
  </button>
  {#if children}
    {@render children()}
  {/if}
</nav>
