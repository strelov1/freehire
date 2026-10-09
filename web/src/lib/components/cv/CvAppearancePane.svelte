<script lang="ts">
  // The shell both appearance tabs share: the intro line, the load gate, and the Save
  // that writes the whole record. Only the controls differ between Template and
  // Typography, so only the controls live in the pages.
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import { Button } from '$lib/ui';
  import { cvAppearance } from '$lib/cvAppearance.svelte';
  import { messages } from './CvAppearancePane.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  // `lead` is the half that differs; the caveat below it is the same on both tabs and
  // so belongs here rather than in two prop strings to keep in step.
  let { lead, children }: { lead: string; children: Snippet } = $props();

  const s = $derived(t(messages, locale()));

  onMount(() => {
    // A "Saved." or an error raised on the other tab belongs to that visit, not to this
    // one — the store outlives the pane, so the pane clears it on arrival.
    cvAppearance.clearSaveStatus();
    void cvAppearance.ensureLoaded();
  });
</script>

<div class="space-y-6">
  <p class="text-sm text-muted-foreground">
    {lead} {s.leadSuffix}
  </p>

  {#if cvAppearance.loadFailed}
    <p class="text-sm text-destructive">{s.loadFailed}</p>
  {:else if !cvAppearance.loaded}
    <p class="text-muted-foreground">{s.loading}</p>
  {:else}
    {@render children()}

    <div class="flex flex-wrap items-center gap-2">
      <Button variant="primary" disabled={cvAppearance.saving} onclick={() => cvAppearance.save()}>
        {s.saveDefaults}
      </Button>
    </div>
    {#if cvAppearance.saveError}
      <p class="text-sm text-destructive">{cvAppearance.saveError}</p>
    {:else if cvAppearance.saved}
      <p class="text-xs text-muted-foreground">{s.saved}</p>
    {/if}
  {/if}
</div>
