<script lang="ts">
  import InboxView from '$lib/components/InboxView.svelte';
</script>

<svelte:head>
  <title>Inbox — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. A
     touch wider than the single-column pages so the two-pane inbox has room. -->
<div class="max-w-4xl">
  <InboxView />
</div>
