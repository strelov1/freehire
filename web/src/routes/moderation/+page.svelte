<script lang="ts">
  import ModerationView from '$lib/components/ModerationView.svelte';
</script>

<svelte:head>
  <title>Moderation — freehire</title>
  <!-- Moderator-only page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ModerationView />
</div>
