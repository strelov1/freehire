<script lang="ts">
  import MyJobsView from '$lib/components/MyJobsView.svelte';
</script>

<svelte:head>
  <title>My jobs — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <MyJobsView />
</div>
