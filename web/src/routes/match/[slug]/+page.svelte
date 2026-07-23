<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { ArrowLeft, SquarePen, Loader } from '@lucide/svelte';
  import { Button } from '$lib/ui';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import MatchAnalysisFull from '$lib/components/MatchAnalysisFull.svelte';

  let { data } = $props();

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

  <!-- The Tailor CTA renders inside the verdict card (bottom-right), so it appears exactly when
       the analysis lands. MatchAnalysisFull owns the card; the page owns the button + navigation
       and hands it down as a snippet. -->
  {#snippet tailorCta()}
    <Button
      variant="outline"
      size="lg"
      onclick={startTailoring}
      disabled={tailoring}
      class="gap-2 rounded-xl border-transparent bg-brand-muted font-semibold text-brand-strong transition hover:bg-brand-muted hover:text-brand-strong hover:opacity-80 disabled:opacity-60"
    >
      {#if tailoring}
        <Loader class="size-[1.15rem] animate-spin" />Preparing…
      {:else}
        <SquarePen class="size-[1.15rem]" />Tailor my CV
      {/if}
    </Button>
  {/snippet}

  <MatchAnalysisFull job={data.job} initial={data.fit} {tailorCta} />
</div>
