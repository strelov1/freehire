<script lang="ts">
  import TrackingCalendar from '$lib/components/TrackingCalendar.svelte';
  import type { PageData } from './$types';
  import { messages } from './messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  let { data }: { data: PageData } = $props();

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<TrackingCalendar prefetched={data.prefetched} />
