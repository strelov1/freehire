<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { announce } from '#lib/announce.svelte.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // Both phases are spent. The way on is to change what the feed matches on: edit the
  // profile or the saved search, or open the source menu for another one.
  // `focus`: take focus and announce on mount (a screen replacing the card).
  let {
    focus = true,
    onChooseSource,
    children,
  }: { focus?: boolean; onChooseSource?: () => void; children?: Snippet } = $props();
  let heading = $state<HTMLElement | null>(null);
  const bySearch = $derived(feedSource.current?.kind === 'saved_search');

  onMount(() => {
    if (!focus) return;
    heading?.focus();
    announce("You're all caught up");
  });

  function chooseAnother(event: MouseEvent) {
    // This click is outside the menu and would close it right after it opens.
    event.stopPropagation();
    onChooseSource?.();
  }
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
  <h2 bind:this={heading} tabindex="-1" class="text-lg font-semibold outline-none">You're all caught up</h2>
  <div class="mt-2 flex flex-wrap justify-center gap-2">
    {#if bySearch}
      <Button variant="cta" size="lg" href={resolve('/my/notifications/searches')}>Edit search</Button>
    {:else}
      <Button variant="cta" size="lg" href={resolve('/my/profile/skills')}>Edit profile</Button>
    {/if}
    <Button variant="secondary" size="lg" onclick={chooseAnother}>Choose another search</Button>
    {@render children?.()}
  </div>
</div>
