<script lang="ts">
  import BuddyFeedPanel from '#lib/components/BuddyFeedPanel.svelte';
  import BuddySimilarRail from '#lib/components/BuddySimilarRail.svelte';
</script>

<svelte:head><title>Feed · Interview buddies</title></svelte:head>

<h1 class="sr-only">Interview buddies feed</h1>
<div class="flex flex-col gap-4 xl:flex-row xl:gap-6">
  <div class="min-w-0 flex-1"><BuddyFeedPanel /></div>
  <aside class="xl:sticky xl:top-20 xl:w-68 xl:shrink-0 xl:self-start"><BuddySimilarRail /></aside>
</div>
