<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { ArrowLeft, SquarePen } from '@lucide/svelte';
  import { currentUser } from '$lib/auth.svelte';
  import { Button } from '$lib/ui';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import MatchAnalysisFull from '$lib/components/MatchAnalysisFull.svelte';

  let { data } = $props();

  // Tailoring is a beta-tester feature; the CTA shows once an analysis exists for a stored
  // CV. A stale analysis (CV or job changed since) still tailors — we nudge a recompute for
  // the sharpest reframing (see below) rather than block, since any CV re-upload marks every
  // past analysis stale and blocking would hide the feature too often.
  const canTailor = $derived(
    !!data.fit?.analysis && data.fit?.has_cv === true && currentUser()?.beta_tester === true,
  );

  let tailoring = $state(false);

  async function startTailoring() {
    // The /tailor/[slug] surface owns the bootstrap + seeded agent session; this just goes there.
    tailoring = true;
    await goto(`/tailor/${data.job.public_slug}`);
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

  <MatchAnalysisFull job={data.job} initial={data.fit} />

  {#if canTailor}
    <section
      class="flex flex-col gap-3 rounded-xl border border-border bg-muted/30 p-5 sm:flex-row sm:items-center sm:justify-between"
    >
      <div class="flex flex-col gap-1">
        <h2 class="flex items-center gap-2 text-sm font-semibold">
          <SquarePen class="size-4 text-primary" />Tailor your CV to this role
        </h2>
        <p class="text-sm text-muted-foreground">
          Create a copy of your CV reframed toward this vacancy — starting from the gaps this
          analysis found. You can keep editing it after.
        </p>
        {#if data.fit?.stale}
          <p class="text-xs text-amber-600 dark:text-amber-500">
            This analysis is out of date — recompute above for the sharpest tailoring.
          </p>
        {/if}
      </div>
      <Button onclick={startTailoring} disabled={tailoring} class="shrink-0">
        {tailoring ? 'Preparing…' : 'Tailor my CV'}
      </Button>
    </section>
  {/if}
</div>
