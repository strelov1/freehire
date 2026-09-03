<script lang="ts">
  import { Building2, Code, Globe, Layers, TrendingUp } from '@lucide/svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { countryLabel } from '$lib/facets';
  import type { FamilyIconName } from '$lib/familymarks';
  import { glyphColorFor } from '$lib/markColor';
  import type { SeeAlsoCard } from '$lib/collections';
  import { CountryFlag } from '$lib/ui';

  // A horizontally-scrolling row of links into existing /collections/:slug
  // pages, each showing the collection's live open-job count — mirrors the
  // /collections hub's card, minus the description (this block is a
  // secondary discovery aid, not the hub itself). `cards` is computed
  // server-side (+page.server.ts) from this job's own facets and its
  // company's collections; no client-side fetch.
  let { cards }: { cards: SeeAlsoCard[] } = $props();

  // Family icon components live here, not in familymarks.ts: that module is
  // imported by seeAlsoMark.ts, which plain-Node vitest runs, and a Lucide
  // icon is a .svelte file the Svelte compiler must transform.
  const FAMILY_ICONS: Record<FamilyIconName, LucideIcon> = {
    tech: Code,
    role: Layers,
    seniority: TrendingUp,
    remote: Globe,
    company: Building2,
  };

  // Shared by the logo and family mark kinds — both are an icon centered on a
  // colored circle, differing only in the color and what's drawn inside.
  const badgeClass = 'flex size-7 shrink-0 items-center justify-center rounded-full';
</script>

{#if cards.length > 0}
  <section class="mt-8">
    <h2 class="mb-3 text-sm font-semibold text-muted-foreground">See also</h2>
    <div
      class="flex gap-3 overflow-x-auto pb-1 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {#each cards as card (card.slug)}
        <a
          href={resolve('/collections/[slug]', { slug: card.slug })}
          class="flex w-40 shrink-0 flex-col items-start gap-2 rounded-lg border border-border p-3 transition-colors hover:bg-muted"
        >
          <div class="flex w-full items-center justify-between gap-2">
            {#if card.mark.kind === 'image'}
              <img src={card.mark.src} alt="" class="size-7 shrink-0 rounded-full object-contain" />
            {:else if card.mark.kind === 'logo'}
              {@const mark = card.mark}
              <span class={badgeClass} style:background-color="#{mark.hex}">
                <svg viewBox="0 0 24 24" class="size-4" role="img" aria-label={mark.title}>
                  <path d={mark.path} fill={glyphColorFor(`#${mark.hex}`)} />
                </svg>
              </span>
            {:else if card.mark.kind === 'flag'}
              <CountryFlag
                code={card.mark.countryCode}
                label={countryLabel(card.mark.countryCode)}
                class="shrink-0 text-[28px]"
              />
            {:else}
              {@const Icon = FAMILY_ICONS[card.mark.icon]}
              <span class={badgeClass} style:background-color={card.mark.color}>
                <Icon class="size-4 text-white" />
              </span>
            {/if}
            {#if card.count !== null}
              <span
                class="shrink-0 rounded-full bg-secondary px-2 py-0.5 font-mono text-xs text-muted-foreground"
              >
                {card.count.toLocaleString()} jobs
              </span>
            {/if}
          </div>
          <span class="text-sm font-medium text-foreground">{card.title} jobs</span>
        </a>
      {/each}
    </div>
  </section>
{/if}
