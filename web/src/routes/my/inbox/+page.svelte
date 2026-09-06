<script lang="ts">
  import InboxView from '$lib/components/InboxView.svelte';
</script>

<svelte:head>
  <title>Inbox — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. A
     touch wider than the single-column pages so the two-pane inbox has room. -->
<div class="max-w-4xl">
  <!-- The title sits here rather than inside InboxView so it is present while the view
       is still loading, the way every other /my/* section titles itself. -->
  <div class="mb-6 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Inbox</h1>
    <p class="text-sm text-muted-foreground">
      The mail your applications generate, linked back to the application it belongs to.
    </p>
  </div>

  <InboxView />
</div>
