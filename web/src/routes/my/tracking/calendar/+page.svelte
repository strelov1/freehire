<script lang="ts">
  import TrackingCalendar from '$lib/components/TrackingCalendar.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<svelte:head>
  <title>Calendar — Tracking — freehire</title>
</svelte:head>

<TrackingCalendar events={data.events} />
