<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { completeProviderReauthentication } from '#lib/recentAuth.js';

  let error = $state<string | null>(null);
  onMount(async () => {
    const code = page.url.searchParams.get('code');
    if (!code) {
      error = 'The identity check did not complete. Please try again.';
      announce(error, 'assertive');
      return;
    }
    try {
      await goto(resolve(await completeProviderReauthentication(code)));
    } catch {
      error = 'The identity check expired or was already used. Please try again.';
      announce(error, 'assertive');
    }
  });
</script>

<svelte:head><title>Confirm identity · freehire</title></svelte:head>
<div class="mx-auto max-w-md py-16 text-center">
  <h1 class="mb-3 text-xl font-semibold tracking-tight">Confirm your identity</h1>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else}
    <p class="text-sm text-muted-foreground">Confirming your identity…</p>
  {/if}
</div>
