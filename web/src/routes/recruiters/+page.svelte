<script lang="ts">
  import RecruitersView from '$lib/components/RecruitersView.svelte';
</script>

<svelte:head>
  <title>Post a job — freehire for recruiters</title>
  <meta
    name="description"
    content="Submit a tech job to freehire, the free, open-source IT job aggregator. Moderator-reviewed postings join one clean, searchable feed of developer roles."
  />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <RecruitersView />
</div>
