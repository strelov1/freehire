<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { tick } from 'svelte';
  import { api, ApiError } from '#lib/api.js';
  import type { InterviewPrepSheet, ToolJob } from '#lib/api-methods/jobs.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import CatalogJobPicker from '#lib/components/CatalogJobPicker.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { signinUrl } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';

  const title = 'Interview prep';
  const description =
    'Pick a Freehire job to practice questions based on its posting and find examples you can use in your answers.';
  const slug = $derived(page.url.searchParams.get('job'));
  const userID = $derived(currentUser()?.id);
  type SheetResult =
    | { kind: 'loaded'; sheet: InterviewPrepSheet }
    | { kind: 'error'; message: string; retryable: boolean };

  let outcome = $state.raw<{ slug: string; userID: typeof userID; result: SheetResult }>();
  const result = $derived(outcome && outcome.slug === slug && outcome.userID === userID ? outcome.result : null);
  const sheet = $derived(result?.kind === 'loaded' ? result.sheet : null);

  let attempt = $state(0);
  let picker = $state<HTMLDivElement>();
  let questionsHeading = $state<HTMLHeadingElement>();
  let focusForSlug: string | undefined;

  $effect(() => {
    const selected = slug;
    const viewer = userID;
    void attempt;
    const controller = new AbortController();
    outcome = undefined;
    if (selected) void loadSheet(selected, viewer, controller.signal);
    return () => controller.abort();
  });

  async function loadSheet(selected: string, viewer: typeof userID, signal: AbortSignal) {
    try {
      const data = await api.getInterviewPrep(selected, signal);
      if (signal.aborted) return;
      outcome = { slug: selected, userID: viewer, result: { kind: 'loaded', sheet: data } };
    } catch (e) {
      if (signal.aborted) return;
      const retryable = !(e instanceof ApiError && e.status === 404);
      const message = retryable
        ? 'We couldn’t load your prep sheet.'
        : 'This job is no longer available. Choose another job below.';
      outcome = { slug: selected, userID: viewer, result: { kind: 'error', message, retryable } };
    }
    if (focusForSlug !== selected) return;
    focusForSlug = undefined;
    await tick();
    if (!signal.aborted) (questionsHeading ?? picker?.querySelector('input'))?.focus();
  }

  async function selectJob(job: ToolJob) {
    focusForSlug = job.public_slug;
    if (slug === job.public_slug) {
      attempt++;
      return;
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- the query is appended to a resolved path
    await goto(`${resolve('/tools/interview-prep')}?job=${encodeURIComponent(job.public_slug)}`, { reset: false });
  }

  async function changeJob() {
    await goto(resolve('/tools/interview-prep'), { reset: false });
    await tick();
    picker?.querySelector('input')?.focus();
  }
</script>

<Seo title={title + ' · Freehire'} {description} canonical={page.url.origin + resolve('/tools/interview-prep')} />

<div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-12">
  <a href={resolve('/tools')} class="text-sm text-muted-foreground hover:underline">Job search tools</a>
  <h1 class="mt-4 text-3xl font-semibold tracking-tight sm:text-4xl">{title}</h1>
  <p class="mt-3 max-w-xl text-muted-foreground">{description}</p>

  <section class="mt-8 border-y border-border py-5" aria-label="Job">
    {#if sheet}
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <h2 class="wrap-break-word font-semibold">
            <a href={resolve('/jobs/[slug]', { slug: sheet.job.public_slug })} class="hover:underline"
              >{sheet.job.title}</a
            >
          </h2>
          <p class="wrap-break-word text-sm text-muted-foreground">
            {[sheet.job.company, sheet.job.location].filter(Boolean).join(' · ')}
          </p>
        </div>
        <Button variant="outline" size="sm" onclick={changeJob}>Change job</Button>
      </div>
    {:else if slug && !result}
      <p role="status" class="text-sm text-muted-foreground">Preparing questions…</p>
    {:else}
      {#if result?.kind === 'error'}
        <div class="mb-3 flex flex-wrap items-center gap-2">
          <p role="alert" class="text-sm text-destructive">{result.message}</p>
          {#if result.retryable}
            <Button size="sm" variant="outline" onclick={() => attempt++}>Try again</Button>
          {/if}
        </div>
      {/if}
      <div bind:this={picker}>
        <CatalogJobPicker onSelect={selectJob} />
      </div>
    {/if}
  </section>

  {#if sheet}
    <section class="mt-7" aria-labelledby="prep-questions">
      <h2 id="prep-questions" class="text-xl font-semibold" tabindex="-1" bind:this={questionsHeading}>
        Practice questions
      </h2>
      <ol class="mt-4 list-decimal space-y-4 pl-5 marker:text-muted-foreground">
        {#each sheet.questions as { topic, prompt }, i (i)}
          <li class="wrap-break-word pl-1">
            {#if topic}<p class="text-sm font-medium">{topic}</p>{/if}
            <p class={topic ? 'mt-1 text-sm text-muted-foreground' : 'text-sm'}>{prompt}</p>
          </li>
        {/each}
      </ol>
    </section>

    <section class="mt-7 border-t border-border pt-6" aria-labelledby="prep-examples">
      <h2 id="prep-examples" class="text-xl font-semibold">Examples for your answers</h2>
      {#if userID === undefined || (!sheet.examples && !sheet.examples_unavailable)}
        <p class="mt-2 text-sm text-muted-foreground">Sign in to find relevant examples from your Experience Bank.</p>
        <Button
          class="mt-3"
          variant="outline"
          href={signinUrl({
            returnTo: `${resolve('/tools/interview-prep')}?job=${encodeURIComponent(sheet.job.public_slug)}`,
            mode: 'login',
          })}>Sign in to find examples</Button
        >
      {:else if sheet.examples_unavailable}
        <p role="alert" class="mt-2 text-sm text-destructive">We couldn’t load examples from your Experience Bank.</p>
        <Button class="mt-3" variant="outline" onclick={() => attempt++}>Try again</Button>
      {:else if sheet.examples?.length}
        <p class="mt-2 text-sm text-muted-foreground">
          Review these examples from your Experience Bank and fill in the details for a STAR answer.
        </p>
        <ul class="ph-no-capture mt-4 divide-y divide-border border-y border-border">
          {#each sheet.examples as example, i (i)}
            <li class="space-y-3 wrap-break-word py-4">
              <h3 class="font-medium">{example.claim}</h3>
              {#if example.place}<p class="text-sm text-muted-foreground">{example.place}</p>{/if}
              <dl class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
                {@render starDetail('Situation', 'What was happening, and why did it matter?', example.context)}
                {@render starDetail('Task', 'What were you responsible for?')}
                {@render starDetail('Action', 'What steps did you personally take?')}
                {@render starDetail('Result', 'What changed, and how did you know?', example.metrics?.join(' · '))}
              </dl>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="mt-2 text-sm text-muted-foreground">
          Add an achievement or upload a CV to find examples for this posting.
        </p>
        <div class="mt-3 flex flex-wrap gap-2">
          <Button variant="outline" href={resolve('/my/profile/experience')}>Open Experience Bank</Button>
          <Button variant="outline" href={resolve('/my/profile')}>Upload a CV</Button>
        </div>
      {/if}
    </section>
  {/if}
</div>

{#snippet starDetail(label: string, prompt: string, detail?: string)}
  <div>
    <dt class="font-medium">{label}</dt>
    <dd class="text-muted-foreground">{detail || prompt}</dd>
  </div>
{/snippet}
