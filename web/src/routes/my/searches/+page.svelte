<script lang="ts">
  import SavedSearchesView from '$lib/components/SavedSearchesView.svelte';
</script>

<svelte:head>
  <title>Saved searches — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <SavedSearchesView />
</div>
