<script lang="ts">
  // One of the two appearance panes. The record they edit is shared (cvAppearance) —
  // this one owns the template choice, Typography owns the type and the margins.
  import { onMount } from 'svelte';
  import { Button } from '$lib/ui';
  import TemplateGallery from '$lib/tailor/TemplateGallery.svelte';
  import {
    cvAppearance,
    ensureCvAppearanceLoaded,
    saveCvAppearance,
  } from '$lib/cvAppearance.svelte';

  onMount(ensureCvAppearanceLoaded);
</script>

<svelte:head>
  <title>CV template default — freehire</title>
</svelte:head>

<div class="space-y-6">
  <p class="text-sm text-muted-foreground">
    The template a new CV starts with. Changing it only affects CVs you create from now on — CVs
    you already have keep their own appearance.
  </p>

  {#if cvAppearance.status === 'loading'}
    <p class="text-muted-foreground">Loading…</p>
  {:else if cvAppearance.status === 'error'}
    <p class="text-sm text-destructive">{cvAppearance.loadError}</p>
  {:else}
    <TemplateGallery bind:value={cvAppearance.templateId} />

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
