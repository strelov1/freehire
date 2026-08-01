<script lang="ts">
  import JobBoard from '$lib/components/JobBoard.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<svelte:head>
  <title>Tracking · List — freehire</title>
</svelte:head>

<!-- Same rows and the same application panel as the board, read as a list. -->
<JobBoard view="list" initial={data.board} />
