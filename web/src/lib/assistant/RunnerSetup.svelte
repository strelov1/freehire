<script lang="ts">
  import { Copy, Check, Terminal } from '@lucide/svelte';

  /** Shown when the assistant has nowhere to run: it executes on the user's own
   *  machine, and no runner is connected. This is a setup step, not a failure,
   *  so it reads as instructions rather than an error. */
  let { compact = false }: { compact?: boolean } = $props();

  const steps = [
    { label: 'Install the CLI', cmd: 'curl -fsSL https://freehire.me/install.sh | sh' },
    { label: 'Sign in', cmd: 'freehire auth login' },
    { label: 'Install the coding agent', cmd: 'npm i -g @zed-industries/claude-code-acp' },
    { label: 'Log in to Claude', cmd: 'claude' },
    { label: 'Connect this machine', cmd: 'freehire runner' },
  ];

  let copied = $state<number | null>(null);

  async function copy(i: number, cmd: string) {
    try {
      await navigator.clipboard.writeText(cmd);
      copied = i;
      setTimeout(() => (copied === i ? (copied = null) : null), 1500);
    } catch {
      // Clipboard can be blocked; the command is visible and selectable anyway.
    }
  }
</script>

<div class="m-3 rounded-xl border border-border bg-card p-6">
  <div class="flex items-start gap-3">
    <Terminal class="mt-0.5 size-5 shrink-0 text-muted-foreground" />
    <div class="min-w-0">
      <h2 class="text-base font-medium">Run the assistant on your own machine</h2>
      <p class="mt-1 text-sm text-muted-foreground">
        The assistant works with your own Claude subscription. It runs on your computer, so your
        Claude credentials never reach our servers — and the work is billed to your account, not
        ours.
      </p>
    </div>
  </div>

  <ol class="mt-5 space-y-3">
    {#each steps as step, i (step.cmd)}
      <li class="flex items-start gap-3">
        <span
          class="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground"
        >
          {i + 1}
        </span>
        <div class="min-w-0 flex-1">
          <div class="text-sm">{step.label}</div>
          <div class="mt-1 flex items-center gap-2">
            <code
              class="min-w-0 flex-1 overflow-x-auto whitespace-nowrap rounded-md bg-muted px-2.5 py-1.5 font-mono text-xs"
              >{step.cmd}</code
            >
            <button
              type="button"
              onclick={() => copy(i, step.cmd)}
              aria-label={`Copy: ${step.cmd}`}
              class="inline-flex size-7 shrink-0 items-center justify-center rounded-md border border-border transition-colors hover:bg-muted"
            >
              {#if copied === i}
                <Check class="size-3.5 text-green-600" />
              {:else}
                <Copy class="size-3.5" />
              {/if}
            </button>
          </div>
        </div>
      </li>
    {/each}
  </ol>

  {#if !compact}
    <p class="mt-5 border-t border-border pt-4 text-xs text-muted-foreground">
      Keep the last command running while you use the assistant — it is idle until you send a
      message. Stop it with Ctrl-C when you are done. Your chats, CVs and applications stay here;
      only the AI runs on your machine.
    </p>
  {/if}
</div>
