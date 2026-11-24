<script lang="ts">
  import { onMount } from 'svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { signInAgainForFeed } from '#lib/feedSession.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // The session ended while Matches was open. Nothing else renders: the cards are no longer the
  // visitor's to see, and a state like "All caught up" would be false. Sign-in is the only thing in
  // flight; if the navigation is refused, say so and let the visitor try again.
  let heading = $state<HTMLElement | null>(null);
  let refused = $state(false);

  async function signIn() {
    refused = false;
    try {
      await signInAgainForFeed();
    } catch {
      refused = true;
      announce("Couldn't open sign-in — try again", 'assertive');
    }
  }

  onMount(() => {
    heading?.focus();
    announce('Your session ended. Taking you to sign in');
    void signIn();
  });
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center" aria-busy={!refused}>
  <h2 bind:this={heading} tabindex="-1" class="text-lg font-semibold outline-none">Your session ended</h2>
  {#if refused}
    <p class="text-sm text-muted-foreground">We couldn't open sign-in.</p>
    <Button variant="cta" size="lg" class="mt-2" onclick={signIn}>Sign in again</Button>
  {:else}
    <p class="text-sm text-muted-foreground">Taking you to sign in…</p>
  {/if}
</div>
