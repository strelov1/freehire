<script lang="ts">
  import { messages } from '$lib/components/activity.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import JobHistory from '$lib/components/JobHistory.svelte';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitles.history}</title>
</svelte:head>

<JobHistory />
