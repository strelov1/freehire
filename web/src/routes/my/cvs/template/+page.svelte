<script lang="ts">
  // One of the two appearance panes. The record they edit is shared (cvAppearance) —
  // this one owns the template choice, Typography owns the type and the margins.
  import { cvAppearance } from '$lib/cvAppearance.svelte';
  import CvAppearancePane from '$lib/components/cv/CvAppearancePane.svelte';
  import TemplateGallery from '$lib/tailor/TemplateGallery.svelte';
</script>

<svelte:head>
  <title>CV template default — freehire</title>
</svelte:head>

<CvAppearancePane lead="The template a new CV starts with.">
  <TemplateGallery bind:value={cvAppearance.templateId} />
</CvAppearancePane>
