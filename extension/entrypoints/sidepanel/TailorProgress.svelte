<script lang="ts">
  import type { TailorRun } from './tailorRun.svelte';

  /** A tailoring run in progress: its requirement count, a bar, and the link to watch it. */
  let { run, watchUrl }: { run: TailorRun; watchUrl: string } = $props();

  let total = $derived(run.progress.total);
  let pct = $derived(total ? Math.round((run.progress.done / total) * 100) : 0);
</script>

<div class="tailoring" aria-live="polite">
  <div class="mrow">
    <span class="label">Tailoring your CV</span>
    <span class="count">{total ? `${run.progress.done} of ${total} requirements` : 'starting…'}</span>
  </div>
  <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={pct}>
    <div class="fill" class:pending={!total} style="width:{total ? pct : 100}%"></div>
  </div>
  <a class="link" href={watchUrl} target="_blank" rel="noreferrer">Watch it work →</a>
  <p class="hint">Runs on freehire; the tab it opened shows each edit.</p>
</div>

<style>
  .tailoring {
    margin-top: 12px;
    display: grid;
    gap: 6px;
  }
  .mrow {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }
  .label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted-foreground);
    font-weight: 600;
  }
  .count {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .bar {
    height: 8px;
    border-radius: 999px;
    background: var(--muted);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    border-radius: 999px;
    background: var(--brand);
    transition: width 0.3s ease;
  }
  /* Before the run lays down its plan there is nothing to count: a full, pulsing bar says
     "working" without inventing a percentage. */
  .fill.pending {
    opacity: 0.4;
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.8;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .fill.pending {
      animation: none;
    }
  }
  .link {
    font-size: 12px;
    font-weight: 500;
    color: var(--brand-strong);
  }
  .hint {
    font-size: 13px;
    color: var(--muted-foreground);
  }
</style>
