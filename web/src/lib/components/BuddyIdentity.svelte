<script lang="ts">
  import { resolve } from '$app/paths';
  import { buddyHeading } from '#lib/buddyPresent.js';
  import { countryLabel } from '#lib/facets.js';
  import { CountryFlag } from '#lib/ui/index.js';
  import BuddyAvatar from './BuddyAvatar.svelte';

  // Who an interview buddy is, in one line: their generated avatar, the Talent Network's
  // dictionary heading, their country's flag, and the handle small beneath. Every list that
  // shows a member renders this, so the feed, the similar list and the conversations agree.
  //
  // `link` makes the whole identity the way to the member's page; a host that is already
  // a button (a conversation row, a dock header) passes false, since a link inside a
  // button is not a valid control.
  let {
    handle,
    summary,
    size = 'md',
    link = true,
    avatar = true,
  }: {
    handle: string;
    summary: { category: string; seniority: string; country: string };
    size?: 'sm' | 'md';
    link?: boolean;
    /** False where the host already draws the member's avatar larger (a network card). */
    avatar?: boolean;
  } = $props();

  const heading = $derived(buddyHeading(summary));
</script>

{#snippet body()}
  {#if avatar}<BuddyAvatar {handle} size={size === 'md' ? 'md' : 'sm'} class="shrink-0" />{/if}
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
  <a href={resolve('/buddies/[handle]', { handle })} class="group flex min-w-0 items-center gap-3 rounded-md">
    {@render body()}
  </a>
{:else}
  <span class="flex min-w-0 items-center gap-3">{@render body()}</span>
{/if}
