<script lang="ts">
  import { Globe } from '@lucide/svelte';
  import { logoDevUrl } from '$lib/logo';

  // A company's logo from logo.dev's name endpoint. `fallback=404` makes the API
  // return 404 — instead of a generic monogram — when it has no logo, so the image
  // fails to load and we show the globe icon instead. The globe therefore covers
  // both "no logo found" and "request failed" (offline, API down). Shared by the job
  // row, the job page, and the company page for one look (and by the OG-image card,
  // via $lib/logo).
  let { name, size = 'size-4' }: { name: string; size?: string } = $props();

  let failed = $state(false);
  const src = $derived(logoDevUrl(name) ?? undefined);

  // A new company means a fresh attempt — clear a prior failure when the name changes
  // (the company page reuses this instance across navigations).
  $effect(() => {
    void name; // re-run when the company changes
    failed = false;
  });
</script>

{#if !name || failed}
  <Globe class="{size} shrink-0 text-muted-foreground" />
{:else}
  <img
    {src}
    alt=""
    class="{size} shrink-0 rounded object-contain"
    loading="lazy"
    onerror={() => (failed = true)}
  />
{/if}
