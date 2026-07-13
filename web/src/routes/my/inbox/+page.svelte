<script lang="ts">
  import InboxView from '$lib/components/InboxView.svelte';
</script>

<svelte:head>
  <title>Inbox — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="max-w-3xl">
  <InboxView />
</div>
