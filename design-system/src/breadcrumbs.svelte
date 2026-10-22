<script lang="ts" module>
  export type BreadcrumbItem = { name: string; href?: string };
</script>

<script lang="ts">
  import { ChevronRight } from '@lucide/svelte';
  import { cn } from './cn.js';

  // A visible trail back to the page a detail page sits under — the one navigational
  // pattern the app never had a rendered version of before this: the SEO structured
  // data (`breadcrumbJsonLd` in web's `#lib/seo`) fed a search engine, never a reader.
  //
  // An item without an `href` renders as the current page — bold, unlinked, carrying
  // `aria-current="page"`. The component does not enforce that this is the LAST item;
  // stating the current page as the final entry with no `href` is the whole contract,
  // the same way a caller is trusted to close a list correctly.

  let { items, class: className }: { items: BreadcrumbItem[]; class?: string } = $props();
</script>

<nav aria-label="Breadcrumb" class={cn('text-sm', className)}>
  <ol class="flex flex-wrap items-center gap-1.5">
    <!-- Keyed by position, not name: two levels can legitimately share a label (a job
    titled exactly "QA" filed under the QA category), and a trail never reorders or
    drops an entry from under itself, so position IS identity here. -->
    {#each items as item, i (i)}
      {#if i > 0}
        <ChevronRight class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
      {/if}
      <li class="flex items-center">
        {#if item.href}
          <a href={item.href} class="text-muted-foreground transition-colors hover:text-foreground hover:underline">
            {item.name}
          </a>
        {:else}
          <span class="font-medium text-foreground" aria-current="page">{item.name}</span>
        {/if}
      </li>
    {/each}
  </ol>
</nav>
