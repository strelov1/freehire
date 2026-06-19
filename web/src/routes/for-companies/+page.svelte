<script lang="ts">
  import ForCompaniesView from '$lib/components/ForCompaniesView.svelte';
</script>

<svelte:head>
  <title>List your job board — freehire for companies</title>
  <meta
    name="description"
    content="Get your company's whole ATS board indexed by freehire, the free, open-source IT job aggregator. Add a supported board with one line, or contribute an adapter — we crawl it regularly and close roles you take down."
  />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ForCompaniesView />
</div>
