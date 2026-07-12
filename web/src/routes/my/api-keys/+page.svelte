<script lang="ts">
  import ApiKeysView from '$lib/components/ApiKeysView.svelte';
</script>

<svelte:head>
  <title>API keys — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the content readable within the content column. -->
<div class="max-w-3xl">
  <ApiKeysView />
</div>
