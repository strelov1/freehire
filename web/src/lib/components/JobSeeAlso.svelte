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
    <!-- Wraps rather than scrolling sideways: a hidden-scrollbar row cut its last card mid-word
         ("Data A…") with nothing to say there was more. -->
    <div class="flex flex-wrap gap-2">
      {#each cards as card (card.slug)}
        <a
          href={resolve('/collections/[slug]', { slug: card.slug })}
          class="flex max-w-full flex-col items-start gap-1.5 rounded-lg border border-border px-3 py-2 transition-colors hover:bg-muted"
        >
          <div class="flex w-full items-center justify-between gap-2">
            <CollectionMark mark={card.mark} />
            {#if card.count !== null}
              <span class="shrink-0 rounded-full bg-secondary px-2 py-0.5 text-xs tabular-nums text-muted-foreground">
                {card.count.toLocaleString()} jobs
              </span>
            {/if}
          </div>
          <span class="break-words text-sm font-medium text-foreground">{card.title} jobs</span>
        </a>
      {/each}
    </div>
  </section>
{/if}
