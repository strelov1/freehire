<script lang="ts">
  import { page } from '$app/state';
  import ContributeLandingView from '$lib/components/ContributeLandingView.svelte';
  import Seo from '$lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/contribute`);
</script>

<Seo
  title="Contribute a board — help freehire cover the whole market"
  description="freehire is an open, free search engine for tech jobs — and it's only as good as its coverage. Spot a company we don't track yet? Paste its link on freehire or send it to the bot: if the board is new, we crawl all its jobs and reward you with points."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ContributeLandingView />
</div>
