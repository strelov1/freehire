<script lang="ts">
  import RecommendationsView from '$lib/components/RecommendationsView.svelte';
</script>

<svelte:head>
  <title>Recommendations — freehire</title>
  <!-- Personal, per-user feed: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- pl-12 clears the mobile filters edge tab (reset at md, where the sidebar shows). -->
  <div class="pl-12 md:pl-0">
    <h1 class="text-2xl font-semibold tracking-tight">Recommended for you</h1>
    <p class="mt-1 text-sm text-muted-foreground">Jobs matched to your CV by meaning, not just keywords.</p>
  </div>
  <div class="mt-6">
    <RecommendationsView />
  </div>
</div>
