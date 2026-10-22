<script lang="ts">
  import PipelineView from '#lib/components/PipelineView.svelte';
  import { messages } from './messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<PipelineView />
