<script lang="ts">
  import { Building2, Code, Globe, Layers, TrendingUp } from '@lucide/svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { countryLabel } from '#lib/facets.js';
  import type { FamilyIconName } from '#lib/familymarks.js';
  import { glyphColorFor } from '#lib/markColor.js';
  import type { SeeAlsoMark } from '#lib/collections.js';
  import { CountryFlag } from '#lib/ui/index.js';

  // The one visual mark a collection link carries (see resolveSeeAlsoMark): a
  // backer's brand image, a technology logo, a country flag, or a family icon.
  let { mark }: { mark: SeeAlsoMark } = $props();

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

{#if mark.kind === 'image'}
  <img src={mark.src} alt="" loading="lazy" decoding="async" width="28" height="28" class="size-7 shrink-0 rounded-full object-contain" />
{:else if mark.kind === 'logo'}
  <span class={badgeClass} style:background-color="#{mark.hex}">
    <svg viewBox="0 0 24 24" class="size-4" role="img" aria-label={mark.title}>
      <path d={mark.path} fill={glyphColorFor(`#${mark.hex}`)} />
    </svg>
  </span>
{:else if mark.kind === 'flag'}
  <CountryFlag code={mark.countryCode} label={countryLabel(mark.countryCode)} class="shrink-0 text-3xl" />
{:else}
  {@const Icon = FAMILY_ICONS[mark.icon]}
  <span class={badgeClass} style:background-color={mark.color}>
    <Icon class="size-4 text-white" />
  </span>
{/if}
