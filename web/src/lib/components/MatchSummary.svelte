<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight, FileText } from '@lucide/svelte';
  import { verdictTone, type Tone } from '$lib/matchAnalysis';
  import type { MatchAnalysisResponse } from '$lib/types';
  import { Button } from '$lib/ui';

  // What the Profile-match sidebar has to SAY about the LLM fit analysis: the cached
  // verdict when one exists, or the prompt to upload a CV when the page has read that there
  // is none. Nothing else, and nothing to press.
  //
  // It used to carry the `Tailor my CV` button too. That button is the page's single primary
  // call to action and now lives in the CTA row beside `Apply`, where the reader decides —
  // see JobView.svelte's `tailorCta`. Its allowance caption went with it rather than staying
  // behind: ConfirmTailorDialog already states what a session costs and what today has left,
  // at the moment of the commit, and a count stated in two places is a count that can
  // disagree.
  //
  // This component fetches nothing. The page reads the match analysis once and hands it
  // down, because the CTA row needs the same response to know whether to offer the button
  // at all, and moving that button must not buy a second call to the same endpoint.
  let {
    slug,
    matchAnalysis,
  }: { slug: string; matchAnalysis: MatchAnalysisResponse | null } = $props();

  const analysis = $derived(matchAnalysis?.analysis ?? null);
  const topGap = $derived(analysis?.gaps?.[0] ?? null);
  // `=== false`, not `!has_cv`: the prompt is for a reader the page has READ as having no
  // CV, never for one it has not heard back about yet. The CTA row's own gate is the exact
  // complement of this one.
  const noCv = $derived(matchAnalysis?.has_cv === false);

  const toneText: Record<Tone, string> = {
    strong: 'text-brand-strong',
    good: 'text-brand-strong',
    moderate: 'text-warning-strong',
    weak: 'text-warning-strong',
    poor: 'text-destructive',
  };
</script>

<!-- No visible heading: what the block shows says what it is, and a section label plus a
     sentence of explanation above it was three lines of chrome. The aria-label stays — a
     section still has to be named for assistive tech. The section is not rendered at all
     when there is nothing to report, rather than drawn empty above its own rule. -->
{#if noCv || analysis}
  <section class="flex flex-col gap-3 border-t border-border pt-4" aria-label="Fit analysis">
    {#if noCv}
      <div class="flex items-center justify-between gap-2">
        <span class="flex items-center gap-1.5 text-xs text-muted-foreground"><FileText class="size-3.5 shrink-0" />Upload a CV to analyse</span>
        <Button variant="primary" size="sm" href={resolve('/my/profile')}>Upload CV</Button>
      </div>
    {:else if analysis}
      {@const tone = verdictTone(analysis.overall_score)}
      <a href={resolve('/tailor/[slug]', { slug })} class="group flex flex-col gap-2 rounded-lg border border-border p-3 transition-colors hover:border-brand/40 hover:bg-accent/40">
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-2xl font-bold tabular-nums leading-none {toneText[tone]}">{analysis.overall_score}%</span>
          <span class="text-sm font-medium {toneText[tone]}">{analysis.verdict}</span>
        </div>
        {#if topGap}
          <p class="text-xs text-muted-foreground"><span class="font-medium">Top gap:</span> {topGap}</p>
        {/if}
        <span class="flex items-center gap-1 text-xs font-medium text-primary">
          View full analysis <ArrowRight class="size-3.5 transition-transform group-hover:translate-x-0.5" />
        </span>
      </a>
    {/if}
  </section>
{/if}
