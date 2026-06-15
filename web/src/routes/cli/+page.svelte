<script lang="ts">
  import { page } from '$app/state';
  import CliView from '$lib/components/CliView.svelte';
  import Seo from '$lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/cli`);
</script>

<Seo
  title="freehire CLI — search and track jobs from the terminal"
  description="A small Go CLI over the freehire job API, built so an AI agent or a script can search, open and track jobs from the terminal — no browser. Authenticate once with an API key."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <CliView />
</div>
