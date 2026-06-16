<script lang="ts">
  import NotificationsView from '$lib/components/NotificationsView.svelte';
</script>

<svelte:head>
  <title>Notifications — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <NotificationsView />
</div>
