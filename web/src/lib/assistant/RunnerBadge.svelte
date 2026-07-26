<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Laptop, Server, CloudOff } from '@lucide/svelte';
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
      title={`Running on your machine (${status.devices.join(', ')})`}
    >
      <Laptop class="size-3.5" />
      Your machine
    </span>
  {:else if status.required}
    <span
      class="inline-flex items-center gap-1.5 rounded-full border border-amber-600/30 bg-amber-600/10 px-2 py-0.5 text-xs text-amber-700 dark:text-amber-400"
      title="Start `freehire runner` to use the assistant"
    >
      <CloudOff class="size-3.5" />
      No machine connected
    </span>
  {:else}
    <span
      class="inline-flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground"
      title="Running on freehire's servers. Start `freehire runner` to use your own Claude."
    >
      <Server class="size-3.5" />
      Our servers
    </span>
  {/if}
{/if}
