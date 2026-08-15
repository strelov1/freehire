<script lang="ts">
  import { onMount } from 'svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // A full navigation, not goto(): the target is the Go API rather than a route of this
  // app, and the session cookie has to ride the request. The load already established
  // that there is a session, so this is a handoff, not a gate.
  onMount(() => {
    window.location.href = data.apiUrl;
  });
</script>

<svelte:head><title>Connect the extension · freehire</title></svelte:head>

<div class="mx-auto max-w-md py-16 text-center">
  <p class="text-sm text-muted-foreground">Connecting the freehire extension…</p>
</div>
