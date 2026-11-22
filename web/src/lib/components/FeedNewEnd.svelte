<script lang="ts">
  import { onMount } from 'svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // The pause between the last new card and the first rest card of the match feed.
  let { onContinue }: { onContinue: () => void } = $props();
  let heading = $state<HTMLElement | null>(null);

  onMount(() => {
    heading?.focus();
    announce("That's everything new");
  });
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
  <h2 bind:this={heading} tabindex="-1" class="text-lg font-semibold outline-none">That's everything new</h2>
  <p class="text-sm text-muted-foreground">
    You've seen everything new since your last visit. Next come the rest of your matches.
  </p>
  <Button onclick={onContinue}>Show the rest</Button>
</div>
