<script lang="ts">
  import SavedSearchesView from '$lib/components/SavedSearchesView.svelte';
</script>

<svelte:head>
  <title>Saved searches — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the list readable within the content column. -->
<div class="max-w-3xl">
  <SavedSearchesView />
</div>
