<script lang="ts">
  // The shell both appearance tabs share: the intro line, the load gate, and the Save
  // that writes the whole record. Only the controls differ between Template and
  // Typography, so only the controls live in the pages.
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import { Button } from '$lib/ui';
  import {
    cvAppearance,
    ensureCvAppearanceLoaded,
    saveCvAppearance,
  } from '$lib/cvAppearance.svelte';

  let { intro, children }: { intro: string; children: Snippet } = $props();

  onMount(ensureCvAppearanceLoaded);
</script>

<div class="space-y-6">
  <p class="text-sm text-muted-foreground">{intro}</p>

  {#if cvAppearance.status === 'loading'}
    <p class="text-muted-foreground">Loading…</p>
  {:else if cvAppearance.status === 'error'}
    <p class="text-sm text-destructive">{cvAppearance.loadError}</p>
  {:else}
    {@render children()}

    <div class="flex flex-wrap items-center gap-2">
      <Button variant="primary" disabled={cvAppearance.saving} onclick={saveCvAppearance}>
        Save defaults
      </Button>
    </div>
    {#if cvAppearance.saveError}
      <p class="text-sm text-destructive">{cvAppearance.saveError}</p>
    {:else if cvAppearance.saved}
      <p class="text-xs text-muted-foreground">Saved.</p>
    {/if}
  {/if}
</div>
