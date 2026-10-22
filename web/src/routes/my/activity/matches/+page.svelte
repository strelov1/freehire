<script lang="ts">
  import { messages } from '#lib/components/activity.messages.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import AnalysesView from '#lib/components/AnalysesView.svelte';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitles.matches}</title>
</svelte:head>

<AnalysesView />
