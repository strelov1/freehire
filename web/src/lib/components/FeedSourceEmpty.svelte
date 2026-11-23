<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { announce } from '#lib/announce.svelte.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // The two screens of a saved-search source that cannot show cards: no matches (filters are
  // never widened to fill the feed) and a search deleted elsewhere. `focus`: take focus and
  // announce on mount, as FeedNewEnd does (swipe mode, where the screen replaces the card).
  let { kind, focus = true }: { kind: 'no_matches' | 'deleted'; focus?: boolean } = $props();
  let heading = $state<HTMLElement | null>(null);
  let failed = $state(false);

  const title = $derived(kind === 'deleted' ? 'This search was deleted' : 'No matches for this search');

  onMount(() => {
    if (!focus) return;
    heading?.focus();
    announce(title);
  });

  async function backToProfile() {
    failed = false;
    try {
      await feedSource.choose({ source: 'profile' });
    } catch {
      failed = true;
    }
  }
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
  <h2 bind:this={heading} tabindex="-1" class="text-lg font-semibold outline-none">{title}</h2>
  {#if kind === 'no_matches'}
    <p class="text-sm text-muted-foreground">We don't widen your search to fill the feed.</p>
    <a
      class="rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-brand-foreground hover:opacity-90"
      href={resolve('/my/notifications/searches')}>Edit search</a
    >
    <Button variant="outline" onclick={backToProfile}>Back to matching by profile</Button>
  {:else}
    <Button onclick={backToProfile}>Back to matching by profile</Button>
  {/if}
  {#if failed}
    <p role="status" class="text-sm text-destructive">Couldn't switch the source — try again.</p>
  {/if}
</div>
