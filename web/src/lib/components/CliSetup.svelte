<script lang="ts">
  import { CLI_INSTALL } from '#lib/cliLinks.js';
  import { announce } from '#lib/announce.svelte.js';

  let { compact = false }: { compact?: boolean } = $props();

  // The CLI half of /mcp: one prompt a person hands to a terminal agent, which does the
  // three steps itself. The same steps are spelled out on /cli for someone typing them.
  const prompt = `Set up freehire for me so you can search jobs and track my applications.
1. Install the CLI: run \`${CLI_INSTALL}\`.
2. Sign in: run \`freehire auth login\` and let me finish the sign-in in my browser.
3. Check it works: run \`freehire search "golang" --remote\` and show me the first results.
Once that is done, let me know it is ready.`;

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout>;

  async function copy() {
    try {
      await navigator.clipboard.writeText(prompt);
      copied = true;
      announce('Copied to the clipboard');
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1600);
    } catch {
      // Clipboard can be blocked — the prompt is plainly visible to select by hand.
    }
  }
</script>

<section class={compact ? '' : 'rounded-2xl border border-border bg-secondary/20 p-5 sm:p-8'}>
  <h2 class={compact ? 'text-base font-semibold tracking-tight' : 'text-xl font-semibold tracking-tight'}>Copy and send this to your agent</h2>
  <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
    Works with Codex, Claude Code, Gemini CLI and any agent that can run a terminal command. Sign-in opens your
    browser, so there is no key to paste.
  </p>
  <figure class="mt-5 max-w-2xl overflow-hidden rounded-xl border border-border bg-secondary/60 shadow-sm">
    <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
      <span>prompt</span>
      <button
        type="button"
        onclick={copy}
        aria-label={copied ? 'Copied agent prompt' : 'Copy agent prompt'}
        class="ml-auto shrink-0 rounded-md border border-border px-2 py-0.5 font-sans text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        {copied ? 'copied ✓' : 'copy'}
      </button>
    </figcaption>
    <pre class="overflow-x-auto whitespace-pre-wrap p-4 font-mono text-sm leading-relaxed">{prompt}</pre>
  </figure>
</section>
