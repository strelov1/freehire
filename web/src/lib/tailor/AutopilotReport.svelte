<script lang="ts">
  // What an unattended run did, shown above the fit analysis: one row per requirement with
  // how it was closed, plus the two things to do next — run again, or undo the whole run.
  //
  // This is the run's own account of itself, not a re-scored verdict: nothing here recomputes
  // the fit analysis underneath, which still measures the BASE CV against the vacancy.
  import { RotateCcw, Undo2 } from '@lucide/svelte';
  import { summarizeRun, statusMeta } from './autopilot';
  import type { AutopilotEntry } from '$lib/generated/contracts';

  let {
    report,
    revertable,
    busy = false,
    onRerun,
    onUndo,
  }: {
    report: AutopilotEntry[] | undefined;
    revertable: boolean;
    /** A run is in flight, or an undo is mid-round-trip: neither action may be started twice. */
    busy?: boolean;
    onRerun: () => void;
    onUndo: () => void;
  } = $props();

  const summary = $derived(summarizeRun(report));
  const rows = $derived(report ?? []);

  const toneClass: Record<string, string> = {
    closed: 'text-emerald-600 dark:text-emerald-400',
    open: 'text-amber-600 dark:text-amber-400',
    skipped: 'text-muted-foreground',
  };
</script>

{#if rows.length > 0}
  <section class="mb-4 rounded-xl border border-border bg-muted/30 p-3">
    <header class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-foreground">
        Autopilot · {summary.closed} of {summary.total} closed
      </h3>
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          disabled={busy}
          onclick={onRerun}
        >
          <RotateCcw class="size-3.5" />
          Run again
        </button>
        {#if revertable}
          <button
            type="button"
            class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
            disabled={busy}
            onclick={onUndo}
          >
            <Undo2 class="size-3.5" />
            Undo the run
          </button>
        {/if}
      </div>
    </header>

    <ul class="flex flex-col gap-1.5">
      {#each rows as row, i (`${i}-${row.requirement}`)}
        {@const meta = statusMeta(row.status)}
        <li class="text-sm">
          <div class="flex items-baseline gap-2">
            <span class={['text-xs font-medium', toneClass[meta.tone]]}>
              {meta.tone === 'closed' ? '✓' : meta.tone === 'open' ? '○' : '–'}
            </span>
            <span class="min-w-0 flex-1 text-foreground">{row.requirement}</span>
          </div>
          <p class="ml-5 text-xs text-muted-foreground">
            {meta.label}{row.note ? ` — ${row.note}` : ''}
          </p>
        </li>
      {/each}
    </ul>

    {#if summary.notReached > 0}
      <p class="mt-2 text-xs text-muted-foreground">
        {summary.notReached} requirement{summary.notReached === 1 ? '' : 's'} were not reached — running
        again picks up from there.
      </p>
    {/if}
  </section>
{/if}
