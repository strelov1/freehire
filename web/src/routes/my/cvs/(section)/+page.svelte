<script lang="ts">
  import CvList from '#lib/components/cv/CvList.svelte';
  import { messages } from './listMessages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<CvList />
