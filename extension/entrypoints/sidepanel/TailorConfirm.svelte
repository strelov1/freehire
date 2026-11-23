<script lang="ts">
  import { Check, TriangleAlert } from '@lucide/svelte';
  import { ConfirmDialog } from 'freehire-design-system';
  import {
    allowanceRefuses,
    allowanceRemaining,
    partitionBlockers,
    type FreehireJob,
    type JobMatch,
    type MatchAnalysisAllowance,
  } from '../../lib/freehire';

  // Pre-flight confirmation before a tailoring run: the deterministic (no-LLM)
  // skill/requirement check the card already has data for, shown one more time as an
  // explicit "are you sure" — see AGENTS.md for why this gate exists. Always shown, even
  // for a full match, just with different copy. Each confirm spends one of today's
  // tailoring sessions (`allowance`, the match analysis's `tailor_allowance`), so it says so.
  let {
    open = $bindable(false),
    job,
    match,
    tone,
    pct,
    allowance,
    onConfirm,
  }: {
    open: boolean;
    job: FreehireJob;
    match: JobMatch;
    tone: 'good' | 'warn' | 'bad';
    pct: number;
    allowance: MatchAnalysisAllowance | null;
    onConfirm: () => void;
  } = $props();

  let blockers = $derived(partitionBlockers(match.blockers));
  let hasGaps = $derived(match.missing.length > 0 || blockers.unmet.length > 0);
  // Nothing left without a refusal is the shadow run: no count rather than "0 left".
  let tailorsLeft = $derived(allowanceRemaining(allowance));
</script>

<ConfirmDialog
  bind:open
  title={`Tailor your CV for ${job.title}?`}
  confirmLabel={hasGaps ? 'Tailor anyway' : 'Tailor my CV'}
  {onConfirm}
>
  <div class="confirm-body">
    <div class="confirm-section">
      <div class="confirm-label">Skills coverage</div>
      <div class="confirm-row">
        <span class="confirm-count">{match.matched.length} of {match.total} required skills</span>
        <span class="confirm-pct {tone}">{pct}%</span>
      </div>
      <div class="bar"><div class="fill {tone}" style="width:{pct}%"></div></div>
      <div class="chips">
        {#each match.matched as s (s)}<span class="chip good" aria-label="{s} — you have this skill">{s}</span>{/each}
        {#each match.missing as s (s)}<span class="chip miss" aria-label="{s} — missing">{s}</span>{/each}
      </div>
    </div>

    {#if blockers.unmet.length > 0 || blockers.met.length > 0}
      <div class="confirm-section">
        <div class="confirm-label">Requirements</div>
        <ul class="confirm-checklist">
          {#each blockers.unmet as b (b.category + b.reason)}
            <li class="confirm-check warn"><TriangleAlert class="icon-sm" /><span>{b.reason}</span></li>
          {/each}
          {#each blockers.met as b (b.category + b.reason)}
            <li class="confirm-check good"><Check class="icon-sm" /><span>{b.reason}</span></li>
          {/each}
        </ul>
      </div>
    {/if}

    <p class="confirm-count">
      {#if allowanceRefuses(allowance)}
        You've used today's tailoring sessions.
      {:else}
        Uses one of today's tailoring sessions{tailorsLeft ? ` — ${tailorsLeft} left` : ''}.
      {/if}
    </p>
  </div>
</ConfirmDialog>

<style>
  .bar {
    height: 8px;
    border-radius: 999px;
    background: var(--muted);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    border-radius: 999px;
  }
  .fill.good {
    background: var(--brand);
  }
  .fill.warn {
    background: var(--warning);
  }
  .fill.bad {
    background: var(--destructive);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin-top: 6px;
  }
  .chip {
    font-size: 12px;
    padding: 3px 9px;
    border-radius: 999px;
    white-space: nowrap;
  }
  .chip.good {
    background: var(--brand-muted);
    color: var(--brand-strong);
    border: 1px solid color-mix(in srgb, var(--brand) 30%, transparent);
  }
  .chip.miss {
    background: color-mix(in srgb, var(--destructive) 10%, transparent);
    color: var(--destructive);
    border: 1px solid color-mix(in srgb, var(--destructive) 30%, transparent);
  }
  .confirm-body {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .confirm-section {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .confirm-label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted-foreground);
    font-weight: 600;
  }
  .confirm-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }
  .confirm-count {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .confirm-pct {
    font-weight: 700;
    font-size: 14px;
  }
  .confirm-pct.good {
    color: var(--brand-strong);
  }
  .confirm-pct.warn {
    color: var(--warning-strong);
  }
  .confirm-pct.bad {
    color: var(--destructive);
  }
  .confirm-checklist {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .confirm-check {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    font-size: 12.5px;
    line-height: 1.4;
  }
  .confirm-check.warn {
    color: var(--warning-strong);
  }
  .confirm-check.good {
    color: var(--muted-foreground);
  }
  .confirm-check.good :global(.icon-sm) {
    color: var(--brand-strong);
  }
</style>
