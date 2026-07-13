<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowLeft } from '@lucide/svelte';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import JobFitFull from '$lib/components/JobFitFull.svelte';

  let { data } = $props();
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
</div>
