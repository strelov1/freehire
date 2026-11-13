<script lang="ts">
  // The CV's ATS score as a small scorecard: a ring coloured by its band with the score in
  // it and the band's word under it, the fixes worth making first beside it, and the way to
  // the full check (/roast, public). The bands and fixes are #lib/atsScore's.
  import { resolve } from '$app/paths';
  import { ArrowRight, CircleAlert } from '@lucide/svelte';
  import { atsTone, atsVerdict, topAtsFixes } from '#lib/atsScore.js';
  import type { ATSReport } from '#lib/types.js';

  let { report }: { report: ATSReport } = $props();

  const R = 42;
  const CIRC = 2 * Math.PI * R;
  const score = $derived(Math.max(0, Math.min(100, report.overall)));
  // ATSReportView's strong tone is `text-primary` (near black), which reads as an unfilled
  // ring here; the scorecard draws a strong score in the brand colour instead.
  const tone = $derived(atsTone(score) === 'text-primary' ? 'text-brand' : atsTone(score));
  const fixes = $derived(topAtsFixes(report));
  // What the band means for the visitor, in the line beside the ring.
  const READING: Record<string, string> = {
    Strong: 'an ATS reads this CV well.',
    Fair: 'an ATS reads most of this CV.',
    'Needs work': 'an ATS will miss parts of this CV.',
  };
</script>

<!-- One row at every width: a small ring the eye reads first, then what it means and what
     to do about it. No frame of its own — the step around it is the card. -->
<div class="flex items-start gap-4">
  <div class="relative size-16 shrink-0">
    <svg
      viewBox="0 0 100 100"
      class="size-16"
      role="img"
      aria-label="ATS score {score} out of 100, {atsVerdict(score)}"
    >
      <circle cx="50" cy="50" r={R} fill="none" stroke-width="9" class="stroke-muted" />
      <circle
        cx="50"
        cy="50"
        r={R}
        fill="none"
        stroke="currentColor"
        stroke-width="9"
        stroke-linecap="round"
        stroke-dasharray="{(CIRC * score) / 100} {CIRC}"
        transform="rotate(-90 50 50)"
        class={tone}
      />
    </svg>
    <span class="absolute inset-0 grid place-items-center text-lg font-semibold tabular-nums" aria-hidden="true">
      {score}
    </span>
  </div>

  <div class="flex min-w-0 flex-1 flex-col gap-2.5">
    <p class="text-sm">
      <span class={['font-semibold', tone]}>{atsVerdict(score)}</span>
      <span class="text-muted-foreground">· {READING[atsVerdict(score)]}</span>
    </p>
    {#if fixes.length > 0}
      <div class="flex flex-col gap-1.5">
        <p class="text-xs font-medium text-muted-foreground">Worth fixing first</p>
        <ul class="flex flex-col gap-1">
          {#each fixes as fix (fix)}
            <li class="flex items-start gap-2 text-sm">
              <CircleAlert class="mt-0.5 size-3.5 shrink-0 text-warning-strong" aria-hidden="true" />
              <span class="text-pretty">{fix}</span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    <a
      href={resolve('/roast')}
      target="_blank"
      rel="noopener"
      class="inline-flex items-center gap-1 self-end text-sm font-medium text-brand-strong underline-offset-4 hover:underline"
    >
      Check your CV in detail <ArrowRight class="size-3.5" aria-hidden="true" />
    </a>
  </div>
</div>
