<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { ArrowLeft, SquarePen, Loader } from '@lucide/svelte';
  import { Button } from '$lib/ui';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import MatchAnalysisFull from '$lib/components/MatchAnalysisFull.svelte';

  let { data } = $props();

  // The Tailor CTA sits above the analysis and unlocks the moment the fit result lands. On a cold
  // match the SSR `data.fit.analysis` is null and MatchAnalysisFull streams the result client-side,
  // so we track its live state (pushed up via `onState`) instead of the SSR prop — otherwise the
  // button would only enable after a full reload. Credits meter the AI spend; a stale analysis
  // still tailors (we nudge a recompute rather than block).
  const hasCv = $derived(data.fit?.has_cv === true);
  // The streamed signal from MatchAnalysisFull (null until it first reports); once set it wins,
  // otherwise fall back to the SSR-cached fit (present on a warm revisit). Keeping `data` inside a
  // $derived (not a one-shot $state seed) means it also reacts to a client-side navigation.
  let streamedReady = $state<boolean | null>(null);
  let analyzing = $state(false);
  const analysisReady = $derived(streamedReady ?? !!data.fit?.analysis);
  const canTailor = $derived(analysisReady && hasCv);

  let tailoring = $state(false);

  async function startTailoring() {
    // The /tailor/[slug] surface owns the bootstrap + seeded agent session; this just goes there.
    tailoring = true;
    await goto(resolve('/tailor/[slug]', { slug: data.job.public_slug }));
  }
</script>

<svelte:head><title>Fit analysis · {data.job.title}</title></svelte:head>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-8 px-4 py-8 sm:py-10">
  <!-- Editorial masthead -->
  <header class="flex flex-col gap-4">
    <a
      href={resolve('/jobs/[slug]', { slug: data.job.public_slug })}
      class="flex w-fit items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
    >
      <ArrowLeft class="size-3.5" />Back to role
    </a>
    <div class="flex flex-col gap-2.5 border-b border-border pb-6">
      <div class="flex items-center gap-2">
        <CompanyLogo name={data.job.company} size="size-5" />
        <p class="text-sm text-muted-foreground">{data.job.company}</p>
      </div>
      <h1 class="text-xl font-bold leading-tight tracking-tight sm:text-2xl">{data.job.title}</h1>
    </div>
  </header>

  {#if hasCv}
    <div class="flex flex-col gap-2">
      <Button
        variant="outline"
        size="lg"
        onclick={startTailoring}
        disabled={!canTailor || tailoring}
        aria-busy={tailoring || (analyzing && !analysisReady)}
        class="w-full gap-2 rounded-xl border-transparent bg-brand-muted font-semibold text-brand-strong transition hover:bg-brand-muted hover:text-brand-strong hover:opacity-80 disabled:opacity-60 sm:w-fit sm:self-start sm:px-6"
      >
        {#if tailoring}
          <Loader class="size-[1.15rem] animate-spin" />Preparing…
        {:else if analyzing && !analysisReady}
          <Loader class="size-[1.15rem] animate-spin" />Analyzing your fit…
        {:else}
          <SquarePen class="size-[1.15rem]" />Tailor my CV
        {/if}
      </Button>
      {#if analyzing && !analysisReady}
        <p class="text-xs text-muted-foreground">
          The tailor button unlocks the moment your fit analysis is ready.
        </p>
      {:else if data.fit?.stale}
        <p class="text-xs text-amber-600 dark:text-amber-500">
          This analysis is out of date — recompute below for the sharpest tailoring.
        </p>
      {/if}
    </div>
  {/if}

  <MatchAnalysisFull
    job={data.job}
    initial={data.fit}
    onState={(a, running) => {
      streamedReady = !!a;
      analyzing = running;
    }}
  />
</div>
