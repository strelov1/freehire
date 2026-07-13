<script lang="ts">
  import InboxView from '$lib/components/InboxView.svelte';
</script>

<svelte:head>
  <title>Inbox — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container (wide for inbox), auth gate,
     and noindex. The inbox fills the content column full-width. -->
<InboxView />
