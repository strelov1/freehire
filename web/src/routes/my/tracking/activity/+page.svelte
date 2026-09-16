<script lang="ts">
  import ActivityGrid from '$lib/components/ActivityGrid.svelte';
  import { messages } from '$lib/components/ActivityGrid.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<ActivityGrid prefetched={data.prefetched} />
