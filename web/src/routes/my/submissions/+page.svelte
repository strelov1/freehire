<script lang="ts">
  import MySubmissionsView from '$lib/components/MySubmissionsView.svelte';
</script>

<svelte:head>
  <title>My submissions — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <MySubmissionsView />
</div>
