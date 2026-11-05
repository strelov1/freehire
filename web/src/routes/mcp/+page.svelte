<script lang="ts">
  import { page } from '$app/state';
  import { MCP_CATALOG_PITCH } from '#lib/mcpCopy.js';
  import McpView from '#lib/components/McpView.svelte';
  import Seo from '#lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/mcp`);
</script>

<Seo
  title="freehire MCP — search IT jobs inside Claude, ChatGPT, Codex and Cursor"
  description={`${MCP_CATALOG_PITCH} Find roles with AI, then connect your account to save jobs, track applications and manage alerts.`}
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <McpView />
</div>
