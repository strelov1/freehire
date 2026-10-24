<script lang="ts">
  import { page } from '$app/state';
  import McpView from '#lib/components/McpView.svelte';
  import Seo from '#lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/mcp`);
</script>

<Seo
  title="freehire MCP — search IT jobs inside Claude, ChatGPT and Cursor"
  description="Add freehire as a connector to your AI assistant and search the live IT job catalogue from a chat. Free, no account needed to search; sign in to save and track jobs."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <McpView />
</div>
