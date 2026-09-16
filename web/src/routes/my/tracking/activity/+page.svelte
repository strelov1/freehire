<script lang="ts">
  import ContributionGrid from '$lib/components/ContributionGrid.svelte';
  import { messages } from '$lib/components/ContributionGrid.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<ContributionGrid prefetched={data.prefetched} />
