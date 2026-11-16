<script lang="ts">
  import { requirementStatusMeta, toneBar, toneText } from '#lib/matchAnalysis.js';
  import type { MatchRequirement } from '#lib/types.js';

  // The fit analysis's ATS view: the coverage tally, the requirements that need
  // attention, and the covered ledger.
  let { requirements, stacked }: { requirements: MatchRequirement[]; stacked: boolean } = $props();

  // Coverage tally for the ATS-view header: covered folds in synonym-only matches (both are
  // positive, matching the hero's count), `addit` is the fixable near-miss, `gap` the genuine
  // miss. `synonym` is a sub-count of covered, surfaced as a legend note on the coverage meter.
  const reqTally = $derived.by(() => {
    const t = { covered: 0, synonym: 0, addit: 0, gap: 0 };
    for (const r of requirements) {
      if (r.status === 'covered') t.covered++;
      else if (r.status === 'synonym-only') {
        t.covered++;
        t.synonym++;
      } else if (r.status === 'missing-have') t.addit++;
      else if (r.status === 'missing-gap') t.gap++;
    }
    return t;
  });
  // Split the ledger so the misses stand out: `attentionReqs` are the fixable
  // near-misses + genuine gaps, pulled into a highlighted callout; `coveredReqs`
  // are everything already satisfied.
  const attentionReqs = $derived(requirements.filter((r) => r.status === 'missing-have' || r.status === 'missing-gap'));
  const coveredReqs = $derived(requirements.filter((r) => r.status === 'covered' || r.status === 'synonym-only'));
  const reqTotal = $derived(reqTally.covered + reqTally.addit + reqTally.gap);
  const headingClass = 'text-sm font-medium';
</script>

<!-- ATS requirements: a light multi-column ledger with a coverage tally, so 29 terse
     rows read as a scannable matrix instead of a boxed-in tower. -->
<section class="flex flex-col gap-5">
  <!-- Coverage meter: the plain text tally becomes a segmented bar, so the
       covered/gap split reads at a glance instead of as three numbers. -->
  <div class="flex flex-col gap-2.5">
    <h2 class="{headingClass} text-muted-foreground">Requirements · ATS view</h2>
    {#if reqTotal}
      <div class="flex items-baseline gap-2">
        <span class="text-xl font-bold tabular-nums leading-none"
          >{reqTally.covered}<span class="font-medium text-muted-foreground">/{reqTotal}</span></span
        >
        <span class="text-sm text-muted-foreground">requirements covered</span>
      </div>
      <div class="flex h-2 overflow-hidden rounded-full bg-secondary">
        {#if reqTally.covered}<div
            class="fit-meter h-full bg-brand"
            style="width: {(reqTally.covered / reqTotal) * 100}%"
          ></div>{/if}
        {#if reqTally.addit}<div
            class="fit-meter h-full bg-warning"
            style="width: {(reqTally.addit / reqTotal) * 100}%"
          ></div>{/if}
        {#if reqTally.gap}<div
            class="fit-meter h-full bg-destructive"
            style="width: {(reqTally.gap / reqTotal) * 100}%"
          ></div>{/if}
      </div>
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {#if reqTally.covered}<span class="flex items-center gap-1.5"
            ><span class="size-2 rounded-full bg-brand"></span>{reqTally.covered} covered</span
          >{/if}
        {#if reqTally.addit}<span class="flex items-center gap-1.5"
            ><span class="size-2 rounded-full bg-warning"></span>{reqTally.addit} to add</span
          >{/if}
        {#if reqTally.gap}<span class="flex items-center gap-1.5"
            ><span class="size-2 rounded-full bg-destructive"></span>{reqTally.gap} gap</span
          >{/if}
        {#if reqTally.synonym}<span class="flex items-center gap-1.5"
            ><span class="size-2 rounded-full border border-dotted border-brand-strong"></span>{reqTally.synonym} via synonym</span
          >{/if}
      </div>
    {/if}
  </div>

  <!-- Needs attention: the near-misses + genuine gaps pulled out and
       highlighted, so the few misses are seen at a glance instead of hunted
       for in the covered ledger. A tone-coloured rule + label carries the
       status, replacing the repeated pills. -->
  {#if attentionReqs.length}
    <div class="rounded-xl border border-destructive/25 bg-destructive/5 p-4">
      <h3 class="mb-2 text-xs font-semibold text-destructive-strong">
        Needs attention · {attentionReqs.length}
      </h3>
      <ul class={['grid gap-x-10 gap-y-0.5', !stacked && 'sm:grid-cols-2']}>
        {#each attentionReqs as r, i (i)}
          {@const meta = requirementStatusMeta(r.status)}
          <li class="flex items-center gap-2.5 py-1.5">
            <span class="h-4 w-[3px] shrink-0 rounded-full {toneBar[meta.tone]}"></span>
            <span class="min-w-0 flex-1 text-sm font-medium leading-snug text-foreground">{r.text}</span>
            {#if r.priority && r.priority.toLowerCase() !== 'required'}
              <span class="shrink-0 text-xs font-medium text-muted-foreground">{r.priority}</span>
            {/if}
            <span class="shrink-0 text-xs font-semibold {toneText[meta.tone]}">{meta.label}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <!-- Covered ledger: a quiet checklist — a tone glyph (✓ / ≈ for a synonym
       match) carries the status, so 20+ rows read as a scannable matrix
       instead of a wall of identical pills. `preferred` sits on the right. -->
  {#if coveredReqs.length}
    <div class="flex flex-col gap-2.5">
      <h3 class="text-xs font-semibold text-muted-foreground">
        Covered · {reqTally.covered}
      </h3>
      <ul class={['grid gap-x-10', !stacked && 'sm:grid-cols-2 xl:grid-cols-3']}>
        {#each coveredReqs as r, i (i)}
          {@const syn = r.status === 'synonym-only'}
          {@const weak = r.evidence_strength === 'keyword'}
          <li class="flex items-baseline gap-2.5 border-b border-border/60 py-2">
            <span class="shrink-0 text-xs leading-snug {syn ? 'text-muted-foreground' : 'text-brand-strong'}"
              >{syn ? '≈' : '✓'}</span
            >
            <span
              class={['min-w-0 flex-1 text-sm leading-snug', syn && 'border-b border-dotted border-brand-strong/50']}
              >{r.text}</span
            >
            {#if r.evidence_strength}
              <span
                class={['shrink-0 text-xs font-medium', weak ? 'text-warning-strong' : 'text-muted-foreground']}
                title={weak
                  ? 'Only a bare mention in your CV — back it with a concrete result'
                  : 'Backed by ' + r.evidence_strength + ' evidence in your CV'}>{r.evidence_strength}</span
              >
            {/if}
            {#if r.priority && r.priority.toLowerCase() !== 'required'}
              <span class="shrink-0 text-xs font-medium text-muted-foreground">{r.priority}</span>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</section>

<style>
  /* Meters sweep in, as on the rest of the fit analysis. */
  .fit-meter {
    transform-origin: left;
    animation: fit-grow 0.7s cubic-bezier(0.16, 1, 0.3, 1) backwards;
    animation-delay: 0.15s;
  }
  @keyframes fit-grow {
    from {
      transform: scaleX(0);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .fit-meter {
      animation: none;
    }
  }
</style>
