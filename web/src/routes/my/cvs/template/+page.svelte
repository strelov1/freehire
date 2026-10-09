<script lang="ts">
  // One of the two appearance panes. The record they edit is shared (cvAppearance) —
  // this one owns the template choice, Typography owns the type and the margins.
  import { cvAppearance } from '$lib/cvAppearance.svelte';
  import CvAppearancePane from '$lib/components/cv/CvAppearancePane.svelte';
  import TemplateGallery from '$lib/tailor/TemplateGallery.svelte';
  import { messages } from './messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<CvAppearancePane lead={s.lead}>
  <TemplateGallery bind:value={cvAppearance.templateId} />
</CvAppearancePane>
