<script lang="ts">
  import IntegrationsView from '#lib/components/IntegrationsView.svelte';
</script>

<svelte:head>
  <title>Integrations — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the connection cards readable within the content column. -->
<div class="max-w-2xl">
  <IntegrationsView />
</div>
