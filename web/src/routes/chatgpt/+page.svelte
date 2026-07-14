<script lang="ts">
  import { page } from '$app/state';
  import ChatGptView from '$lib/components/ChatGptView.svelte';
  import Seo from '$lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/chatgpt`);
</script>

<Seo
  title="freehire GPT — search and track jobs inside ChatGPT"
  description="A custom GPT wired to the freehire job API, so you can search IT jobs, open postings and companies, and track your applications from a ChatGPT conversation. Free and open source."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ChatGptView />
</div>
