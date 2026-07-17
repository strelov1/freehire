<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { ArrowLeft, Sparkles } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import { Button } from '$lib/ui';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import JobFitFull from '$lib/components/JobFitFull.svelte';

  let { data } = $props();

  // Tailoring is beta-gated (the server re-checks); the CTA only shows once a FRESH analysis
  // exists for a stored CV — a stale one (CV or job changed since) would reframe toward an
  // outdated verdict, so the user should recompute first. The analysis is what a tailored
  // copy reframes toward.
  const canTailor = $derived(
    !!data.fit?.analysis &&
      data.fit?.stale !== true &&
      data.fit?.has_cv === true &&
      (currentUser()?.beta_tester === true || currentUser()?.role === 'moderator'),
  );

  let tailoring = $state(false);
  let tailorError = $state('');

  async function startTailoring() {
    tailoring = true;
    tailorError = '';
    try {
      const res = await api.tailorCv(data.job.public_slug);
      await goto(`/my/cvs/${res.tailor_cv_id}`);
    } catch (e) {
      tailorError =
        e instanceof ApiError ? e.message : 'Could not start tailoring. Please try again.';
      tailoring = false;
    }
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
      <h1 class="text-xl font-bold leading-tight tracking-tight sm:text-2xl">{data.job.title}</h1>
      <div class="flex items-center gap-2">
        <CompanyLogo name={data.job.company} size="size-5" />
        <p class="text-sm text-muted-foreground">{data.job.company}</p>
      </div>
    </div>
  </header>

  <JobFitFull job={data.job} initial={data.fit} />

  {#if canTailor}
    <section
      class="flex flex-col gap-3 rounded-xl border border-border bg-muted/30 p-5 sm:flex-row sm:items-center sm:justify-between"
    >
      <div class="flex flex-col gap-1">
        <h2 class="flex items-center gap-2 text-sm font-semibold">
          <Sparkles class="size-4 text-primary" />Tailor your CV to this role
        </h2>
        <p class="text-sm text-muted-foreground">
          Create a copy of your CV reframed toward this vacancy — starting from the gaps this
          analysis found. You can keep editing it after.
        </p>
        {#if tailorError}
          <p class="text-sm text-destructive">{tailorError}</p>
        {/if}
      </div>
      <Button onclick={startTailoring} disabled={tailoring} class="shrink-0">
        {tailoring ? 'Preparing…' : 'Tailor my CV'}
      </Button>
    </section>
  {/if}
</div>
