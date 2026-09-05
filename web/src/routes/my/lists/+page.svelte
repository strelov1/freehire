<script lang="ts">
  import JobListsView from '$lib/components/JobListsView.svelte';
</script>

<svelte:head>
  <title>Job lists — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the list readable within the content column. -->
<div class="max-w-3xl">
  <JobListsView />
</div>
