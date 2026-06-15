<script lang="ts">
  import SubmitView from '$lib/components/SubmitView.svelte';
</script>

<svelte:head>
  <title>Submit a job — freehire</title>
  <!-- Contribution form: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <SubmitView />
</div>
