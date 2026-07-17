<script lang="ts">
  import ContributeView from '$lib/components/ContributeView.svelte';
</script>

<svelte:head>
  <title>Contribute a board — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the form and list readable within the content column. -->
<div class="max-w-3xl">
  <ContributeView />
</div>
