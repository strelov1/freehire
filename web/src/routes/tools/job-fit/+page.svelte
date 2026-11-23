<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight } from '@lucide/svelte';
  import { tick } from 'svelte';
  import { api, ApiError } from '#lib/api.js';
  import type { ResumeReviewPosting, ToolJob } from '#lib/api-methods/jobs.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import CatalogJobPicker from '#lib/components/CatalogJobPicker.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { skillLabel } from '#lib/facets.js';
  import { reviewResume } from '#lib/resumeJobReview.js';
  import { signinUrl } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';

  const title = 'Review your CV for a job';
  const description = 'See which lines to highlight and where to add an example before you apply.';
  const sampleResume =
    'Backend Engineer\nBuilt payment APIs in Go and cut response times by 30%.\nSkills: Go, PostgreSQL, Docker';
  const slug = $derived(page.url.searchParams.get('job'));

  type LoadResult = { job: ResumeReviewPosting; error?: never } | { job?: never; error: string };
  let loaded = $state.raw<{ slug: string; result: LoadResult }>();
  const result = $derived(loaded && loaded.slug === slug ? loaded.result : undefined);
  const job = $derived(result?.job);
  let attempt = $state(0);
  let resume = $state('');
  let reviewed = $state(false);
  let picker = $state<HTMLDivElement>();
  let resumeInput = $state<HTMLTextAreaElement>();
  let resultsHeading = $state<HTMLHeadingElement>();
  const findings = $derived(reviewed && job ? reviewResume(resume, job.skills ?? [], job.requirements ?? []) : []);
  const tailorPath = $derived(job ? resolve('/tailor/[slug]', { slug: job.public_slug }) : '');

  $effect(() => {
    const selected = slug;
    void attempt;
    reviewed = false;
    if (!selected) return;
    const controller = new AbortController();
    void loadJob(selected, controller.signal);
    return () => controller.abort();
  });

  async function loadJob(selected: string, signal: AbortSignal) {
    try {
      const posting = await api.getResumeReviewPosting(selected, signal);
      if (!signal.aborted) loaded = { slug: selected, result: { job: posting } };
    } catch (error) {
      if (!signal.aborted) {
        const message =
          error instanceof ApiError && error.status === 404
            ? 'This job is no longer available. Choose another job below.'
            : 'We couldn’t load this job. Search for it below.';
        loaded = { slug: selected, result: { error: message } };
      }
    }
  }

  async function selectJob(selected: ToolJob) {
    if (selected.public_slug === slug) {
      loaded = undefined;
      attempt++;
      return;
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- the query is appended to a resolved path
    await goto(`${resolve('/tools/job-fit')}?job=${encodeURIComponent(selected.public_slug)}`, { reset: false });
  }

  async function changeJob() {
    loaded = undefined;
    await goto(resolve('/tools/job-fit'), { reset: false });
    await tick();
    picker?.querySelector('input')?.focus();
  }

  async function review(event: SubmitEvent) {
    event.preventDefault();
    if (!job || !resume.trim()) return;
    reviewed = true;
    await tick();
    resultsHeading?.focus();
  }

  function useSample() {
    resume = sampleResume;
    reviewed = false;
    resumeInput?.focus();
  }
</script>

<Seo title={title + ' · Freehire'} {description} canonical={page.url.origin + resolve('/tools/job-fit')} />

<div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-12">
  <a href={resolve('/tools')} class="text-sm text-muted-foreground hover:underline">Job search tools</a>
  <h1 class="mt-4 text-3xl font-semibold tracking-tight sm:text-4xl">{title}</h1>
  <p class="mt-3 max-w-xl text-muted-foreground">{description}</p>

  <section class="mt-8 border-y border-border py-5" aria-label="Job">
    {#if job}
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <h2 class="wrap-break-word font-semibold">{job.title}</h2>
          <p class="wrap-break-word text-sm text-muted-foreground">
            {[job.company, job.location].filter(Boolean).join(' · ')}
          </p>
        </div>
        <Button variant="outline" size="sm" onclick={changeJob}>Change job</Button>
      </div>
    {:else if slug && !result}
      <p role="status" class="text-sm text-muted-foreground">Loading job…</p>
    {:else}
      {#if result?.error}<p role="alert" class="mb-3 text-sm text-destructive">{result.error}</p>{/if}
      <div bind:this={picker}>
        <CatalogJobPicker onSelect={selectJob} />
      </div>
    {/if}
  </section>

  {#if job && job.skills?.length}
    <form class="ph-no-capture mt-6" onsubmit={review}>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <label for="resume-text" class="text-sm font-semibold">CV text</label>
        {#if !resume.trim()}
          <Button variant="ghost" size="sm" onclick={useSample}>Try a sample CV</Button>
        {/if}
      </div>
      <textarea
        id="resume-text"
        bind:this={resumeInput}
        bind:value={resume}
        oninput={() => {
          reviewed = false;
        }}
        aria-describedby="resume-hint"
        rows={8}
        maxlength={24000}
        class="mt-2 block w-full resize-y rounded-lg border border-input bg-background p-3 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        placeholder="Paste your experience and skills from your CV"></textarea>
      <p id="resume-hint" class="mt-2 text-sm text-muted-foreground">Your CV text stays in this browser tab.</p>
      <Button class="mt-4" type="submit" variant="primary" disabled={!resume.trim()}>Review CV</Button>
    </form>
  {:else if job}
    <p class="mt-6 text-sm text-muted-foreground">
      This posting has no named skills. In Tailor, you can review the full description.
    </p>
  {/if}

  {#if reviewed && job}
    <section aria-labelledby="review-results" class="ph-no-capture mt-8 border-t border-border pt-6">
      <h2 id="review-results" class="text-xl font-semibold" tabindex="-1" bind:this={resultsHeading}>
        What to change before you apply
      </h2>
      {@render findingGroup('Highlight this experience', 'example')}
      {@render findingGroup('Back it up with an example', 'mention')}
      {@render findingGroup('Check before you apply', 'missing')}
    </section>
  {/if}

  {#if job && (reviewed || !job.skills?.length)}
    <div class="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
      <p class="text-sm text-muted-foreground">Review the full posting and edit your CV in Tailor.</p>
      <Button
        variant="outline"
        href={isAuthenticated()
          ? tailorPath
          : signinUrl({
              returnTo: tailorPath,
              cancelTo: `${resolve('/tools/job-fit')}?job=${encodeURIComponent(job.public_slug)}`,
              mode: 'login',
            })}
      >
        {isAuthenticated() ? 'Open in Tailor' : 'Sign in to open Tailor'}
        <ArrowRight class="size-4" aria-hidden="true" />
      </Button>
    </div>
  {/if}
</div>

{#snippet findingGroup(heading: string, kind: 'example' | 'mention' | 'missing')}
  {@const group = findings.filter((finding) => finding.kind === kind)}
  {#if group.length}
    <div class="mt-6">
      <h3 class="text-base font-semibold">{heading}</h3>
      <ul class="mt-2 divide-y divide-border border-y border-border">
        {#each group as finding (finding.skill)}
          <li class="space-y-2 wrap-break-word py-3 text-sm">
            <p class="font-semibold">
              {skillLabel(finding.skill)}{finding.priority === 'required' ? ' · Required' : ''}
            </p>
            {#if finding.requirement}
              <p class="text-muted-foreground">The posting asks for “{finding.requirement}”</p>
            {/if}
            {#if finding.snippet}
              <blockquote class="rounded-md bg-muted px-3 py-2 text-foreground">“{finding.snippet}”</blockquote>
            {/if}
            {#if kind === 'example'}
              <p>Move this bullet up under the relevant role so it’s easy to find.</p>
            {:else if kind === 'mention'}
              <p>If you used {skillLabel(finding.skill)} on a project, say what you did and what changed.</p>
            {:else}
              <p>If you’ve used {skillLabel(finding.skill)}, add a real example. Otherwise, check if it’s essential.</p>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}
{/snippet}
