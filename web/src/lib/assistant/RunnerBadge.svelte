<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Laptop, CloudOff } from '@lucide/svelte';
  import { runnerStatus, type RunnerStatus } from '$lib/assistant/api';

  /** Whether this machine is running the assistant's harness.
   *
   *  Polled rather than pushed: the runner connects to the agent backend, not
   *  to this page, so there is nothing to subscribe to. Ten seconds is fast
   *  enough to notice a runner starting while someone reads the instructions,
   *  and cheap enough to leave running. */
  let status = $state<RunnerStatus | null>(null);
  let timer: ReturnType<typeof setInterval> | undefined;

  async function refresh() {
    try {
      status = await runnerStatus();
    } catch {
      // Leave the last known state rather than flickering to "disconnected"
      // on a transient failure.
    }
  }

  onMount(() => {
    void refresh();
    timer = setInterval(refresh, 10_000);
  });
  onDestroy(() => clearInterval(timer));
</script>

{#if status}
  {#if status.connected}
    <span
      class="inline-flex items-center gap-1.5 rounded-full border border-green-600/30 bg-green-600/10 px-2 py-0.5 text-xs text-green-700 dark:text-green-400"
      title={`Running on this computer (${status.devices.join(', ')})`}
    >
      <Laptop class="size-3.5" />
      Running on your computer
    </span>
  {:else}
    <span class="inline-flex items-center gap-2 text-xs">
      <span
        class="inline-flex items-center gap-1.5 rounded-full border border-amber-600/30 bg-amber-600/10 px-2 py-0.5 text-amber-700 dark:text-amber-400"
      >
        <CloudOff class="size-3.5" />
        Computer not connected
      </span>
      <!-- The badge alone tells the user something is off without telling them
           what to do about it, so the fix travels with it. -->
      <code class="hidden rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] sm:inline"
        >freehire runner</code
      >
      <a
        href="https://github.com/strelov1/freehire-cli#bring-your-own-claude-byok"
        target="_blank"
        rel="noreferrer"
        class="text-muted-foreground underline underline-offset-2 hover:text-foreground"
      >
        how to connect
      </a>
    </span>
  {/if}
{/if}
