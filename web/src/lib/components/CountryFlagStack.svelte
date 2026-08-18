<script lang="ts">
  import { filterHref } from '$lib/enrichment';
  import { countryLabel } from '$lib/facets';
  import { CountryFlag } from '$lib/ui';

  // An overlapping cluster of round country flags (an avatar-stack). Collapses a
  // long country list into a compact, space-saving row: each flag laps over the
  // previous one, capped at `max` with a "+N" chip for the remainder. Used on the
  // browse card (display-only) and the job sidebar (`link` on), where a wide remote
  // role can list a dozen eligible countries.
  //
  // A single country is common (most postings aren't remote-everywhere) and a bare
  // flag alone leans on its title tooltip to say which one, so that case renders the
  // country name next to the flag instead of collapsing into the stack.
  //
  // The flags overlap by `--lap` (an em fraction of a flag's width) via a negative
  // left margin, and each carries a `ring-card` outline so neighbours stay legible
  // where they touch. Earlier flags sit on top (descending z-index) so the row reads
  // left-to-right; hovering a flag raises it above its neighbours.
  let {
    codes,
    max = 6,
    link = false,
    class: className = '',
  }: {
    codes: string[];
    max?: number;
    link?: boolean;
    class?: string;
  } = $props();

  const shown = $derived(codes.slice(0, max));
  const extra = $derived(codes.length - shown.length);
</script>

{#if codes.length === 1}
  {@const code = codes[0] ?? ''}
  {@const label = countryLabel(code)}
  {#snippet chip()}
    <CountryFlag {code} {label} />
    <span class="truncate text-xs font-medium text-muted-foreground">{label}</span>
  {/snippet}
  <div class={['inline-flex items-center gap-1.5', className]}>
    {#if link}
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal filter link from filterHref; query-only, no route to resolve -->
      <a href={filterHref('countries', code)} class="inline-flex items-center gap-1.5 hover:text-foreground">
        {@render chip()}
      </a>
    {:else}
      {@render chip()}
    {/if}
  </div>
{:else if codes.length > 1}
  <div class={['flex items-center', className]} style:--lap="0.4em">
    {#each shown as code, i (code)}
      {@const label = countryLabel(code)}
      <span
        class={[
          'relative inline-flex rounded-full ring-2 ring-card transition hover:z-10 hover:-translate-y-px',
          i > 0 && '-ml-[var(--lap)]',
        ]}
        style:z-index={shown.length - i}
      >
        {#if link}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal filter link from filterHref; query-only, no route to resolve -->
          <a href={filterHref('countries', code)} class="inline-flex"><CountryFlag {code} {label} /></a>
        {:else}
          <CountryFlag {code} {label} />
        {/if}
      </span>
    {/each}
    {#if extra > 0}
      <span class="ml-1.5 text-xs font-medium text-muted-foreground">+{extra}</span>
    {/if}
  </div>
{/if}
