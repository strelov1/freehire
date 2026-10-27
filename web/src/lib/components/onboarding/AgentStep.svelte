<script lang="ts">
  // The tour's MCP step: the /mcp setup, compact, opening on the account connector — the
  // candidate is signed in, so that is the one that can save jobs to their board.
  import McpSetup from '#lib/components/McpSetup.svelte';
</script>

<h2 class="text-xl font-semibold tracking-tight text-balance">Search jobs from your AI assistant with MCP</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">
  Connect freehire to Claude, ChatGPT or Cursor and ask for jobs in plain words. It searches the whole catalogue for
  you.
</p>

<div class="mt-5">
  <McpSetup compact initialMode="account" />
</div>
<p class="mt-6 text-sm text-muted-foreground">
  Then ask it, for example: “Find jobs that fit my profile and save the best three.”
</p>
