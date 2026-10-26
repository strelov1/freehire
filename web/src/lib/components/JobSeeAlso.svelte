<script lang="ts">
  import { resolve } from '$app/paths';
  import type { SeeAlsoCard } from '#lib/collections.js';
  import CollectionMark from './CollectionMark.svelte';

  // A horizontally-scrolling row of links into existing /collections/:slug
  // pages, each showing the collection's live open-job count — mirrors the
  // /collections hub's card, minus the description (this block is a
  // secondary discovery aid, not the hub itself). `cards` is computed
  // server-side (+page.server.ts) from this job's own facets and its
  // company's collections; no client-side fetch.
  let { cards }: { cards: SeeAlsoCard[] } = $props();
</script>

{#if cards.length > 0}
  <section class="mt-8">
    <h2 class="mb-3 text-sm font-semibold text-muted-foreground">See also</h2>
    <div class="no-scrollbar flex gap-3 overflow-x-auto pb-1">
      {#each cards as card (card.slug)}
        <a
          href={resolve('/collections/[slug]', { slug: card.slug })}
          class="flex w-max shrink-0 flex-col items-start gap-1.5 rounded-lg border border-border px-3 py-2 transition-colors hover:bg-muted"
        >
          <div class="flex w-full items-center justify-between gap-2">
            <CollectionMark mark={card.mark} />
            {#if card.count !== null}
              <span
                class="shrink-0 rounded-full bg-secondary px-2 py-0.5 font-mono text-xs text-muted-foreground"
              >
                {card.count.toLocaleString()} jobs
              </span>
            {/if}
          </div>
          <span class="whitespace-nowrap text-sm font-medium text-foreground">{card.title} jobs</span>
        </a>
      {/each}
    </div>
  </section>
{/if}
