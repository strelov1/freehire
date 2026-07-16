<script lang="ts">
  import JobBoard from '$lib/components/JobBoard.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<svelte:head>
  <title>Tracking — freehire</title>
</svelte:head>

<JobBoard initial={data.board} />
