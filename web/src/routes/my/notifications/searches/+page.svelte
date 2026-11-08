<script lang="ts">
  import SavedSearchesView from '#lib/components/SavedSearchesView.svelte';
</script>

<svelte:head>
  <title>Search alerts — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. Full
     column width, like every other section, so card edges do not move between pages. -->
<div>
  <SavedSearchesView />
</div>
