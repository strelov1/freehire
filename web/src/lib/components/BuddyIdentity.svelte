<script lang="ts">
  import { User } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { buddyHeading } from '#lib/buddyPresent.js';
  import { countryLabel } from '#lib/facets.js';
  import { Avatar, CountryFlag } from '#lib/ui/index.js';

  // Who an interview buddy is, in one line: the silhouette, the Talent Network's dictionary
  // heading, their country's flag, and the handle small beneath. Every list that shows a
  // member renders this, so the feed, the similar list and the conversations agree.
  //
  // The avatar is ALWAYS the plain silhouette — never the member's photo, not even the
  // blurred one the Talent Network serves: a buddy handle is a separate anonymous identity,
  // and the same blurred picture on both would link it to the catalogue card.
  //
  // `link` makes the whole identity the way to the member's page; a host that is already
  // a button (a conversation row, a dock header) passes false, since a link inside a
  // button is not a valid control.
  let {
    handle,
    summary,
    size = 'md',
    link = true,
  }: {
    handle: string;
    summary: { category: string; seniority: string; country: string };
    size?: 'sm' | 'md';
    link?: boolean;
  } = $props();

  const heading = $derived(buddyHeading(summary));
</script>

{#snippet personIcon()}
  <User class={size === 'md' ? 'size-5' : 'size-4'} aria-hidden="true" />
{/snippet}

{#snippet body()}
  <Avatar size={size === 'md' ? 'md' : 'sm'} class="shrink-0 bg-secondary" fallbackIcon={personIcon} />
  <span class="flex min-w-0 flex-col">
    <span class="flex min-w-0 items-center gap-1.5">
      <span class="truncate font-semibold text-foreground {size === 'md' ? 'text-base' : 'text-sm'} {link ? 'group-hover:underline' : ''}">{heading}</span>
      {#if summary.country}
        <CountryFlag code={summary.country} label={countryLabel(summary.country)} class="text-xs" />
      {/if}
    </span>
    <span class="truncate text-xs font-normal text-muted-foreground">{handle}</span>
  </span>
{/snippet}

{#if link}
  <a href={resolve('/my/buddies/[handle]', { handle })} class="group flex min-w-0 items-center gap-3 rounded-md">
    {@render body()}
  </a>
{:else}
  <span class="flex min-w-0 items-center gap-3">{@render body()}</span>
{/if}
