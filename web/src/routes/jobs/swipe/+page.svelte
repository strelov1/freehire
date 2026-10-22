<script lang="ts">
  import SwipeDeck from '#lib/components/SwipeDeck.svelte';
</script>

<svelte:head>
  <title>Swipe jobs — freehire</title>
  <!-- Personal, interactive triage view: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<!-- SwipeDeck renders itself as a full-screen fixed overlay (its own chrome:
     filter + close in the top bar, safe-area aware), so no page container here. -->
<SwipeDeck />
