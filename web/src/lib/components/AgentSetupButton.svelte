<script lang="ts">
  import { onDestroy } from 'svelte';
  import { page } from '$app/state';
  import { Check, Copy } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';

  // One line a terminal agent (Claude Code, Codex, Cursor, Gemini CLI) can act on: it
  // fetches web/static/agent-setup.md and runs the steps for whichever host it is. A chat
  // app with no shell cannot, which is why this sits on /agents and /mcp, not site-wide.
  const prompt = $derived(
    `Fetch and execute the appropriate instructions to set me up for freehire from ${page.url.origin}/agent-setup.md`
  );
  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(timer));

  async function copy() {
    try {
      await navigator.clipboard.writeText(prompt);
      copied = true;
      announce('Copied. Paste it into your AI coding agent.');
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1800);
    } catch {
      announce('Could not copy. Select the text and copy it manually.');
    }
  }
</script>

<div class="mt-7 flex flex-wrap items-center gap-x-3 gap-y-2">
  <button
    type="button"
    onclick={copy}
    class="inline-flex items-center gap-2 rounded-full border border-border bg-secondary/40 px-5 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-secondary"
  >
    {copied ? 'Copied — paste it into your agent' : 'Onboard your agent to freehire'}
    {#if copied}<Check class="size-4" aria-hidden="true" />{:else}<Copy class="size-4" aria-hidden="true" />{/if}
  </button>
  <span class="text-sm text-muted-foreground">Copies a setup prompt for Claude Code, Codex, Cursor or Gemini CLI.</span>
</div>
