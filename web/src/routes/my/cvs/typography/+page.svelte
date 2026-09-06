<script lang="ts">
  // The other appearance pane. Page margins sit here rather than in a third tab: they are
  // the same question as the type — how the text sits on the page — and a tab holding four
  // number fields would be a tab nobody opens.
  import { cvAppearance } from '$lib/cvAppearance.svelte';
  import CvAppearancePane from '$lib/components/cv/CvAppearancePane.svelte';
  import StyleSettings from '$lib/components/cv/StyleSettings.svelte';
  import MarginSettings from '$lib/components/cv/MarginSettings.svelte';
</script>

<svelte:head>
  <title>CV typography defaults — freehire</title>
</svelte:head>

<CvAppearancePane lead="The type and margins a new CV starts with.">
  <StyleSettings bind:style={cvAppearance.style} fonts={cvAppearance.fonts} />

  <section class="space-y-2">
    <h2 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
      Page margins <span class="font-normal normal-case tracking-normal">(inches)</span>
    </h2>
    <MarginSettings bind:margins={cvAppearance.margins} />
  </section>
</CvAppearancePane>
