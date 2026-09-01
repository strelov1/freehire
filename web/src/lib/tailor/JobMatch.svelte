<script lang="ts">
  // How well the CV being edited matches this vacancy — measured on the rendered PDF's text
  // layer, against the vacancy and nothing else. Unlike the match analysis below it in the
  // same tab, this number moves as the candidate types: it is deterministic, costs no allowance,
  // and is recomputed after every saved edit.
  //
  // An unavailable score renders nothing at all. A panel that says "score unavailable"
  // teaches the candidate to ignore it; an absence teaches nothing.
  import { viewJobMatch, scoreTone, type JobMatchTone } from './jobmatch';
  import ScoreCategoryRow from './ScoreCategoryRow.svelte';
  import type { CvJobMatch } from '$lib/cv';
  import SkillIcon from '../components/SkillIcon.svelte';
  import { skillLabel } from '$lib/facets';

  let { data }: { data: CvJobMatch | null | undefined } = $props();

  const view = $derived(viewJobMatch(data));

  const toneText: Record<JobMatchTone, string> = {
    strong: 'text-brand-strong',
    moderate: 'text-warning-strong',
    poor: 'text-destructive',
  };
  const toneBar: Record<JobMatchTone, string> = {
    strong: 'bg-brand',
    moderate: 'bg-warning',
    poor: 'bg-destructive',
  };
  const tone = $derived(view ? scoreTone(view.overall) : 'moderate');
</script>

{#if view}
  <section class="mb-4 rounded-xl border border-border bg-card p-3">
    <header class="mb-2 flex items-baseline justify-between gap-2">
      <h3 class="text-sm font-semibold text-foreground">Job match</h3>
      <span class="text-sm font-semibold tabular-nums {toneText[tone]}">
        {view.overall}&nbsp;<span class="text-xs font-normal text-muted-foreground">/ 100</span>
      </span>
    </header>

    <div class="mb-2 h-1.5 overflow-hidden rounded-full bg-secondary" aria-hidden="true">
      <div class="h-full rounded-full transition-[width] {toneBar[tone]}" style="width: {view.overall}%"></div>
    </div>

    <p class="mb-3 text-[11px] leading-snug text-muted-foreground">
      Scored on your tailored CV as it renders now, against this vacancy.
      {#if view.partial}
        Some categories could not be checked and were left out of the score rather than
        counted against you.
      {/if}
    </p>

    <div class="border-t border-border/60">
      {#each view.rows as row (row.id)}
        <ScoreCategoryRow
          id="job-match-{row.id}"
          label={row.label}
          value={row.text}
          valueClass={row.available ? 'text-foreground' : 'text-muted-foreground'}
          note={row.available ? row.impact : row.reason}
          items={row.items}
        />
      {/each}
    </div>

    {#if view.missingSkills.length}
      <div class="mt-3 rounded-lg bg-warning/10 px-2 py-1.5">
        <p class="mb-1 text-xs font-medium text-warning-strong">
          Skills this vacancy names that your CV does not
        </p>
        <ul class="flex flex-wrap gap-1">
          {#each view.missingSkills as skill, i (i)}
            <li
              class="flex items-center gap-1 rounded border border-warning/30 bg-background/60 px-1.5 py-0.5 text-[11px] text-warning-strong"
            >
              <SkillIcon slug={skill} />{skillLabel(skill)}
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <!-- Keyed by index, never by the requirement's text: those texts are model output, and two
         identical lines in one ledger would be duplicate keys and a render crash. -->
    {#if view.requirements.missing.length}
      <div class="mt-3">
        <h4 class="mb-1 text-xs font-semibold text-foreground">Requirements your CV does not evidence</h4>
        <ul class="space-y-1 text-xs text-muted-foreground">
          {#each view.requirements.missing as req, i (i)}
            <li class="flex items-start gap-1.5">
              <span class="mt-1 size-1.5 shrink-0 rounded-full bg-warning" aria-hidden="true"></span>
              <span>{req.text}</span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <!-- Requirements naming no skill this vacancy states cannot be checked against the
         document at all. They are shown, labelled, and were never scored — saying so is the
         difference between "we did not check this" and "your CV fails this". -->
    {#if view.requirements.unverifiable.length}
      <details class="mt-3">
        <summary class="cursor-pointer text-xs text-muted-foreground">
          {view.requirements.unverifiable.length} requirement{view.requirements.unverifiable.length === 1
            ? ''
            : 's'} could not be checked automatically
        </summary>
        <ul class="mt-1 space-y-1 text-xs text-muted-foreground">
          {#each view.requirements.unverifiable as req, i (i)}
            <li class="flex items-start gap-1.5">
              <span class="mt-1 size-1.5 shrink-0 rounded-full bg-muted-foreground/40" aria-hidden="true"></span>
              <span>
                {req.text}
                {#if req.cached_status}
                  <span class="text-muted-foreground/70">— the earlier match analysis read this as {req.cached_status}</span>
                {/if}
              </span>
            </li>
          {/each}
        </ul>
      </details>
    {/if}
  </section>
{/if}
