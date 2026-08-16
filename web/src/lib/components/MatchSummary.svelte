<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { ArrowRight, FileText, SquarePen } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { askConfirmTailor } from '$lib/confirmTailorDialog.svelte';
  import { verdictTone, type Tone } from '$lib/matchAnalysis';
  import type { MatchAnalysisResponse } from '$lib/types';
  import { Button } from '$lib/ui';

  // A compact match summary in the Profile-match sidebar: the overall %, verdict, and the
  // single biggest gap when an analysis is cached, with a link to the full-page analysis
  // (which runs the live streaming compute). It never computes inline.
  //
  // A guest sees the same pitch and button — the offer is the point of showing it to them —
  // but the button opens sign-in instead of the analysis page. That page streams an
  // authenticated compute client-side, so a guest landing there would 401 on every visit,
  // and repeated 401/403s from one address on the shared access log are what got real
  // users fail2banned once already.
  let { slug }: { slug: string } = $props();

  const guest = $derived(!isAuthenticated());

  let data = $state<MatchAnalysisResponse | null>(null);

  // Read the cached summary (never runs the model); re-fetch on navigation with a guard
  // so a slow response for a previous job can't overwrite the current one.
  $effect(() => {
    const forSlug = slug;
    data = null;
    if (!isAuthenticated()) return;
    api.getMatchAnalysis(forSlug)
      .then((d) => {
        if (forSlug === slug) data = d;
      })
      .catch(() => {});
  });

  const analysis = $derived(data?.analysis ?? null);
  const topGap = $derived(analysis?.gaps?.[0] ?? null);
  // A new (never-analysed) job can't be analysed once the monthly AI credits are spent; a
  // recompute of an already-analysed job stays free, so this only gates the fresh CTA.
  const credits = $derived(data?.credits ?? null);
  const creditsSpent = $derived(!!credits && credits.remaining <= 0);

  const toneText: Record<Tone, string> = {
    strong: 'text-brand-strong',
    good: 'text-brand-strong',
    moderate: 'text-warning-strong',
    weak: 'text-warning-strong',
    poor: 'text-destructive',
  };

  // Tailoring navigates away; the confirmation dialog owns its own fetch of the
  // deterministic skill/requirement check, so this only needs to await the yes/no.
  let tailoring = $state(false);

  async function startTailoring() {
    if (tailoring) return;
    tailoring = true;
    const ok = await askConfirmTailor(slug);
    if (!ok) {
      tailoring = false;
      return;
    }
    await goto(resolve('/tailor/[slug]', { slug }));
  }
</script>

<section class="flex flex-col gap-3 border-t border-border pt-4" aria-label="Analyze match">
  <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Analyze match</p>

  {#if data && !data.has_cv}
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
  {:else if creditsSpent}
    <p class="text-sm text-muted-foreground">You're out of AI credits for this month. They renew {credits ? new Date(credits.resets_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : 'next month'}.</p>
  {:else}
    <p class="text-sm text-muted-foreground">How your CV reads against this role — fit, gaps, and ATS flags.</p>
    <Button
      variant="primary"
      size="lg"
      disabled={tailoring}
      onclick={guest ? () => openAuthDialog('login') : startTailoring}
      class="w-full justify-center gap-2 rounded-xl font-semibold"
    >
      Tailor my CV
      <SquarePen class="size-[1.15rem]" aria-hidden="true" />
    </Button>
    <!-- The line under the button says what happens next: for a signed-in viewer that's
         the credit it will spend, for a guest that it runs on a CV they haven't given us
         yet. A guest has no credits to report, so the two never both apply. -->
    {#if credits}
      <p class="text-xs text-muted-foreground">{credits.remaining} AI credits left this month</p>
    {:else if guest}
      <p class="text-xs text-muted-foreground">Sign in and add a CV to run it.</p>
    {/if}
  {/if}
</section>
