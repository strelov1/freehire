<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { completeProviderReauthentication } from '$lib/recentAuth';

  let error = $state<string | null>(null);
  onMount(async () => {
    const code = page.url.searchParams.get('code');
    if (!code) {
      error = 'The identity check did not complete. Please try again.';
      return;
    }
    try {
      await goto(resolve(await completeProviderReauthentication(code)));
    } catch {
      error = 'The identity check expired or was already used. Please try again.';
    }
  });
</script>

<svelte:head><title>Confirm identity · freehire</title></svelte:head>
<div class="mx-auto max-w-md py-16 text-center">
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else}
    <p class="text-sm text-muted-foreground">Confirming your identity…</p>
  {/if}
</div>
