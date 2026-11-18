<script lang="ts">
  import IntegrationsView from '#lib/components/IntegrationsView.svelte';
</script>

<svelte:head>
  <title>Integrations — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex.
     Full column width, like every other section, so card edges do not move between pages. -->
<div>
  <IntegrationsView />
</div>
