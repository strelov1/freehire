<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { announce } from '#lib/announce.svelte.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // The saved-search screens that replace the card: it matches nothing, it cannot be applied
  // in full, or it is gone. Filters are never relaxed to fill the feed.
  let { kind, mode = 'list' }: { kind: 'no_matches' | 'unusable' | 'deleted'; mode?: 'list' | 'swipe' } = $props();
  let heading = $state<HTMLElement | null>(null);
  let failed = $state(false);
  const titles = {
    no_matches: 'No matches for this search',
    unusable: "This search can't be used here",
    deleted: 'This search was deleted',
  };
  const title = $derived(titles[kind]);

  onMount(() => {
    heading?.focus();
    announce(title);
  });

  async function backToProfile() {
    failed = false;
    if (!(await feedSource.choose({ source: 'profile' }, mode))) {
      failed = true;
      announce("Didn't switch — try again", 'assertive');
    }
  }
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
  <h2 bind:this={heading} tabindex="-1" class="text-lg font-semibold outline-none">{title}</h2>
  {#if kind === 'no_matches'}
    <p class="text-sm text-muted-foreground">We don't widen your search to fill the feed.</p>
  {:else if kind === 'unusable'}
    <p class="text-sm text-muted-foreground">
      Part of it can't be applied, and we don't show partial results. Edit the search to fix it.
    </p>
  {/if}
  {#if kind !== 'deleted'}
    <a
      class="rounded-lg bg-brand px-4 py-2 text-sm font-medium text-brand-foreground hover:opacity-90"
      href={resolve('/my/notifications/searches')}>Edit search</a
    >
  {/if}
  <Button variant={kind === 'deleted' ? 'primary' : 'ghost'} onclick={backToProfile}>Back to matching by profile</Button
  >
  {#if failed}
    <p class="text-sm text-destructive">Didn't switch — try again.</p>
  {/if}
</div>
