<script lang="ts">
  import { page } from '$app/state';
  import ChatGptView from '#lib/components/ChatGptView.svelte';
  import Seo from '#lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/chatgpt`);
</script>

<Seo
  title="freehire for ChatGPT — search IT jobs inside ChatGPT"
  description="Add the freehire app to ChatGPT in one click and search the live IT job catalogue from a chat: jobs as cards, salary ranges, similar roles and which postings are still open. Free, no account needed."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ChatGptView />
</div>
