<script lang="ts">
  import { messages } from '$lib/components/activity.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import SavedJobs from '$lib/components/SavedJobs.svelte';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitles.saved}</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <SavedJobs />
</div>
