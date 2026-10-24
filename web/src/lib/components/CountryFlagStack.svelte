<script lang="ts">
  import { filterHref } from '#lib/enrichment.js';
  import { countryLabel } from '#lib/facets.js';
  import { CountryFlag } from '#lib/ui/index.js';

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
  //
  // In `link` mode the stack is not itself a row of links: lapped flags left each one a
  // ~10px sliver to hit, and a "+N" chip that named countries nobody could reach. The stack
  // is instead the face of a disclosure button, and opening it lists EVERY country as a
  // labelled chip linking to its filter — each a full-size target, wrapped onto as many
  // lines as the list needs. Display-only callers (the browse card, which is itself a link)
  // keep the plain stack.
  let {
    codes,
    max = 6,
    link = false,
    class: className = '',
    labelClass = 'text-xs font-medium text-muted-foreground',
  }: {
    codes: string[];
    max?: number;
    link?: boolean;
    class?: string;
    // The single-country label defaults to a small muted caption, right for the
    // browse card and the company panel's "Offices" list — both already sit beside
    // other small muted text. The job facet list is the odd one out: its other
    // values are `text-sm font-medium` in the ordinary foreground colour, so it
    // overrides this to match rather than reading as a lesser fact about the role.
    labelClass?: string;
  } = $props();

  const shown = $derived(codes.slice(0, max));
  const extra = $derived(codes.length - shown.length);

  let expanded = $state(false);
  // Per-instance, so two stacks on one page cannot claim the same list.
  const listId = $props.id();
</script>

<!-- The overlapping flags themselves, shared by the display-only stack and the face of the
     `link` mode's disclosure button. -->
{#snippet stack()}
  {#each shown as code, i (code)}
    <span
      class={[
        'relative inline-flex rounded-full ring-2 ring-card transition hover:z-10 hover:-translate-y-px',
        i > 0 && '-ml-[var(--lap)]',
      ]}
      style:z-index={shown.length - i}
    >
      <CountryFlag {code} label={countryLabel(code)} />
    </span>
  {/each}
  {#if extra > 0}
    <span class="ml-1.5 text-xs font-medium text-muted-foreground">+{extra}</span>
  {/if}
{/snippet}

{#if codes.length === 1}
  {@const code = codes[0] ?? ''}
  {@const label = countryLabel(code)}
  {#snippet chip()}
    <!-- Decorative here: the country is named right beside it, so the flag's own label
         would only read the name twice. -->
    <span aria-hidden="true" class="contents"><CountryFlag {code} {label} /></span>
    <span class={['truncate', labelClass]}>{label}</span>
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
{:else if codes.length > 1 && link}
  <div class={['flex flex-col items-end gap-2', className]}>
    <!-- The button's name is spelled out: the flags inside it are images whose labels
         would otherwise be read one by one, followed by a bare "+N". -->
    <button
      type="button"
      class="inline-flex min-h-6 items-center rounded-full px-1 transition-colors hover:bg-accent"
      style:--lap="0.4em"
      aria-expanded={expanded}
      aria-controls={listId}
      aria-label={expanded ? 'Hide the country list' : `Show all ${codes.length} countries`}
      onclick={() => (expanded = !expanded)}
    >
      {@render stack()}
    </button>
    {#if expanded}
      <ul id={listId} class="flex flex-wrap justify-end gap-1.5">
        {#each codes as code (code)}
          {@const label = countryLabel(code)}
          <li>
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal filter link from filterHref; query-only, no route to resolve --><a
              href={filterHref('countries', code)}
              class="inline-flex min-h-7 items-center gap-1.5 rounded-full border border-border px-2 text-xs font-medium transition-colors hover:bg-accent hover:text-foreground"
            >
              <!-- Decorative, as in the single-country chip: the name follows. -->
              <span aria-hidden="true" class="contents"><CountryFlag {code} {label} class="text-sm" /></span>
              {label}
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{:else if codes.length > 1}
  <div class={['flex items-center', className]} style:--lap="0.4em">
    {@render stack()}
  </div>
{/if}
