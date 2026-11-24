<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { announce } from '#lib/announce.svelte.js';

  // One line under the source pill when the feed is sorted by date for lack of skills.
  // Cards keep coming; filters are never relaxed. Announced once, when it first appears.
  onMount(() => announce('Sorted by date — add skills to sort by fit'));
</script>

<p class="rounded-lg border border-border bg-card px-4 py-3 text-sm">
  Sorted by date — <a class="underline underline-offset-4" href={resolve('/my/profile/skills')}>add skills</a> to sort by
  fit
</p>
