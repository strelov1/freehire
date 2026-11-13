<script lang="ts">
  // Getting started: search from an AI assistant over MCP. Shares the canonical MCP setup
  // and its optional OAuth sign-in instructions.
  import McpSetup from '#lib/components/McpSetup.svelte';
</script>

<h2 class="text-xl font-semibold tracking-tight text-balance">Search jobs from your AI assistant with MCP</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">
  Connect freehire to Claude, ChatGPT or Cursor and ask for jobs in plain words. It searches the whole catalogue for
  you.
</p>

<div class="mt-5">
  <McpSetup compact />
</div>
<p class="mt-6 text-sm text-muted-foreground">
  Then ask it, for example: “Find jobs that fit my profile and save the best three.”
</p>
