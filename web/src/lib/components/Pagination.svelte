<script lang="ts">
  import { pageHref, pageWindow } from '$lib/pagination';

  // Real <a href> page links rendered into the SSR markup. This is how BOTH a
  // visitor and a crawler page a listing: link discovery happens when HTML is
  // parsed, so without these the only rows any listing ever advertises are its
  // first twenty. (It used to sit beside a scroll-to-bottom auto-load that was the
  // visitor's path; that grew the page endlessly and put the footer out of reach,
  // so these links are now the only way through.)
  //
  // Deliberately NOT `Pager` from the design system, which steps a bound `page` in
  // local state with buttons. Nothing there reaches the URL, so a shared link opens
  // page one and a crawler sees twenty rows — which is the whole reason this file
  // exists separately rather than reusing the primitive.
  //
  // Not hidden from visitors: a hidden link a crawler is meant to follow is the
  // shape of cloaking, and this doubles as the keyboard/no-JS way through the
  // catalogue. `data-sveltekit-noscroll` is deliberately absent — following one
  // of these IS a navigation to a new page of results, so landing at the top is
  // the right behaviour.
  //
  // `params` is the caller's filter store's `params` — never `page.url.searchParams`,
  // which the shallow URL writes leave behind (UrlSyncedState.params explains why).
  // Read from `page.url`, a listing entered bare renders every link here as a
  // filter-stripped `?page=N`, which is exactly the bug this note exists to prevent.
  //
  // The hrefs below skip `resolve()` (and disable the lint that asks for it):
  // `pathname` is handed in from `page.url.pathname`, which SvelteKit has already
  // resolved — base path applied, route params substituted. There is no route id
  // to resolve here, and re-resolving a resolved path would double any base.
  let {
    current,
    total,
    pathname,
    params,
  }: {
    current: number;
    total: number;
    pathname: string;
    params: URLSearchParams;
  } = $props();

  const pages = $derived(pageWindow(current, total));
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- every href here is built
     from `pathname`, which the caller takes from page.url.pathname: SvelteKit has
     already resolved it (base path applied, route params substituted). There is no
     route id left to resolve, and resolving a resolved path would double the base.
     Reported per-element rather than per-line, so it can't be silenced inline. -->

{#if total > 1}
  <nav class="mt-6 flex flex-wrap items-center justify-center gap-1" aria-label="Pagination">
    {#if current > 1}
      <a
        href={pageHref(pathname, params, current - 1)}
        rel="prev"
        class="rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        Previous
      </a>
    {/if}

    {#each pages as page, i (page === null ? `gap-${i}` : page)}
      {#if page === null}
        <span class="px-2 py-2 text-sm text-muted-foreground" aria-hidden="true">…</span>
      {:else if page === current}
        <span
          class="rounded-md bg-muted px-3 py-2 text-sm font-medium text-foreground"
          aria-current="page"
        >
          {page}
        </span>
      {:else}
        <a
          href={pageHref(pathname, params, page)}
          class="rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          {page}
        </a>
      {/if}
    {/each}

    {#if current < total}
      <a
        href={pageHref(pathname, params, current + 1)}
        rel="next"
        class="rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        Next
      </a>
    {/if}
  </nav>
{/if}
