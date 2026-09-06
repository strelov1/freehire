<script lang="ts">
  // The other appearance pane. Page margins sit here rather than in a third tab: they are
  // the same question as the type — how the text sits on the page — and a tab holding four
  // number fields would be a tab nobody opens.
  import { onMount } from 'svelte';
  import { Button } from '$lib/ui';
  import StyleSettings from '$lib/components/cv/StyleSettings.svelte';
  import MarginSettings from '$lib/components/cv/MarginSettings.svelte';
  import {
    cvAppearance,
    ensureCvAppearanceLoaded,
    saveCvAppearance,
  } from '$lib/cvAppearance.svelte';

  onMount(ensureCvAppearanceLoaded);
</script>

<svelte:head>
  <title>CV typography defaults — freehire</title>
</svelte:head>

<div class="space-y-6">
  <p class="text-sm text-muted-foreground">
    The type and margins a new CV starts with. Changing these only affects CVs you create from now
    on — CVs you already have keep their own appearance.
  </p>

  {#if cvAppearance.status === 'loading'}
    <p class="text-muted-foreground">Loading…</p>
  {:else if cvAppearance.status === 'error'}
    <p class="text-sm text-destructive">{cvAppearance.loadError}</p>
  {:else}
    <StyleSettings bind:style={cvAppearance.style} fonts={cvAppearance.fonts} />

    <section class="space-y-2">
      <h2 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        Page margins <span class="font-normal normal-case tracking-normal">(inches)</span>
      </h2>
      <MarginSettings bind:margins={cvAppearance.margins} />
    </section>

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
