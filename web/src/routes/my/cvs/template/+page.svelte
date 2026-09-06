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

<CvAppearancePane
  intro="The template a new CV starts with. Changing it only affects CVs you create from now on — CVs you already have keep their own appearance."
>
  <TemplateGallery bind:value={cvAppearance.templateId} />
</CvAppearancePane>
