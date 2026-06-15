<script lang="ts">
  import { browser } from '$app/environment';
  import { ArrowRight, Bookmark, Check, Flag } from '@lucide/svelte';
  import { markJobApplied, recordJobView, saveJob, unsaveJob } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { filterHref, formatSalary, summaryFacets } from '$lib/enrichment';
  import { markViewed } from '$lib/viewedJobs.svelte';
  import type { Job, UserJob } from '$lib/types';
  import { Badge, Button } from '$lib/ui';
  import { formatDate } from '$lib/utils';
  import CompanyLogo from './CompanyLogo.svelte';
  import ReportDialog from './ReportDialog.svelte';

  // The job is server-rendered: it arrives as a prop from the route's `load`, so
  // the article's content is in the initial HTML. Only the per-user interactions
  // below hydrate client-side.
  let { job }: { job: Job } = $props();

  // The signed-in user's interaction with this job (null when signed out or not
  // yet loaded). `showApplyPrompt` is the post-click "Did you apply?" question.
  let interaction = $state.raw<UserJob | null>(null);
  let showApplyPrompt = $state(false);
  // The report dialog (a problem-with-this-job complaint) opens over the page.
  let showReport = $state(false);
  const applied = $derived(interaction?.applied_at != null);
  const saved = $derived(interaction?.saved_at != null);

  // Presentational values derived from the (server-rendered) job.
  const posted = $derived(formatDate(job.posted_at));
  const e = $derived(job.enrichment ?? {});
  const salary = $derived(formatSalary(e));
  const facets = $derived(summaryFacets(job));

  // Record a view for signed-in users once the page hydrates (browser only).
  // Silent history that also tells us whether they
  // already applied; a failed view must not break the page. Re-runs on client
  // navigation to another job, resetting the per-user state first.
  $effect(() => {
    const slug = job.public_slug; // track the current job
    interaction = null;
    showApplyPrompt = false;
    if (!browser || !isAuthenticated()) return;
    recordJobView(slug)
      .then((rec) => {
        // Mark it locally so its card dims on back-navigation without a reload.
        markViewed(slug);
        if (job.public_slug === slug) interaction = rec;
      })
      .catch(() => {});
  });

  // The Apply link opens the external posting; once the user has gone to apply,
  // offer the "Did you apply?" choice (only when signed in and not already applied).
  function onApplyClick() {
    if (isAuthenticated() && !applied) showApplyPrompt = true;
  }

  async function confirmApplied() {
    try {
      interaction = await markJobApplied(job.public_slug);
    } catch {
      // Leave the prompt up so the user can retry; nothing else to do.
      return;
    }
    showApplyPrompt = false;
  }

  // "No": purely local — the job must not enter the tracker.
  function dismissApplyPrompt() {
    showApplyPrompt = false;
  }

  // Saving requires an account: a signed-out click opens the sign-in dialog
  // instead (no auto-save afterwards — once signed in they can click again).
  function onSaveClick() {
    if (!isAuthenticated()) {
      openAuthDialog('login');
      return;
    }
    toggleSave();
  }

  // Reporting also requires an account (the report is attributed to the user); a
  // signed-out click opens sign-in instead of the dialog.
  function onReportClick() {
    if (!isAuthenticated()) {
      openAuthDialog('login');
      return;
    }
    showReport = true;
  }

  // The toggle flips on the server's answer, not optimistically: both endpoints
  // return the full interaction, so the button can never drift from the truth.
  async function toggleSave() {
    try {
      interaction = saved ? await unsaveJob(job.public_slug) : await saveJob(job.public_slug);
    } catch {
      // Leave the current state; the user can retry.
    }
  }
</script>

<!-- Wide layout mirroring /jobs. The company line spans the very top; below it a
     sticky left sidebar (salary + actions + metadata) starts level with the title,
     and the description reads in the right column. On mobile everything stacks:
     company → title + apply CTA → metadata → description. -->
<article class="flex flex-col gap-4 lg:grid lg:grid-cols-[20rem_minmax(0,1fr)] lg:gap-x-6 lg:gap-y-4">
  <div class="flex items-center gap-3 lg:col-start-2 lg:row-start-1">
    <CompanyLogo name={job.company} size="size-8" />
    <p class="text-sm text-muted-foreground">
      {#if job.company_slug}
        <a href={`/companies/${job.company_slug}`} class="hover:text-foreground hover:underline">
          {job.company || 'Unknown company'}
        </a>
      {:else}
        {job.company || 'Unknown company'}
      {/if}
    </p>
  </div>

  <header class="flex flex-col gap-4 lg:col-start-2 lg:row-start-2">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <h1 class="text-2xl font-semibold tracking-tight">{job.title}</h1>

      {#if !job.closed_at || applied}
        <div class="flex shrink-0 flex-col gap-2 sm:items-end">
          {#if !job.closed_at}
            <Button
              variant="primary"
              href={job.url}
              target="_blank"
              rel="noopener noreferrer"
              onclick={onApplyClick}
              class="w-full sm:w-auto"
            >
              Show <ArrowRight class="size-4" />
            </Button>
          {/if}
          {#if applied}
            <Badge variant="secondary"><Check class="mr-1 size-3.5" /> You applied</Badge>
          {/if}
        </div>
      {/if}
    </div>

    {#if showApplyPrompt && !applied}
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-secondary px-4 py-3"
      >
        <span class="text-sm">Did you apply to this job?</span>
        <div class="flex items-center gap-2">
          <Button variant="primary" size="sm" onclick={confirmApplied}>Yes, save</Button>
          <Button variant="ghost" size="sm" onclick={dismissApplyPrompt}>No</Button>
        </div>
      </div>
    {/if}
  </header>

  <aside class="w-full shrink-0 lg:col-start-1 lg:row-span-3 lg:row-start-1">
    <div class="sticky top-6 flex flex-col gap-4 rounded-xl border border-border bg-card p-4">
      {#if salary}
        <p
          class="border-t border-border pt-4 text-xl font-semibold tabular-nums tracking-tight first:border-t-0 first:pt-0"
        >
          {salary}
        </p>
      {/if}

      <div class="flex flex-col gap-2 border-t border-border pt-4 first:border-t-0 first:pt-0">
        <Button variant="outline" onclick={onSaveClick} aria-pressed={saved} class="w-full">
          <Bookmark class={saved ? 'size-4 fill-current' : 'size-4'} />
          {saved ? 'Saved' : 'Save'}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onclick={onReportClick}
          class="w-full text-muted-foreground"
        >
          <Flag class="size-4" />
          Report this job
        </Button>
      </div>

      {#if facets.length}
        <dl class="flex flex-col gap-2 border-t border-border pt-4 text-sm first:border-t-0 first:pt-0">
          {#each facets as facet (facet.label)}
            <div class="flex items-baseline justify-between gap-3">
              <dt class="shrink-0 text-muted-foreground">{facet.label}</dt>
              <dd class="min-w-0 break-words text-right font-medium"
                >{#each facet.values as v, i (v.text)}{#if i > 0}, {/if}{#if v.href}<a
                      href={v.href}
                      class="hover:text-foreground hover:underline">{v.text}</a
                    >{:else}{v.text}{/if}{/each}</dd
              >
            </div>
          {/each}
        </dl>
      {/if}

      {#if e.skills?.length}
        <ul class="flex flex-wrap gap-1.5 border-t border-border pt-4 first:border-t-0 first:pt-0">
          {#each e.skills as skill}
            <li>
              <a href={filterHref('skills', skill)}>
                <Badge variant="secondary" class="transition-colors hover:bg-accent">{skill}</Badge>
              </a>
            </li>
          {/each}
        </ul>
      {/if}

      <div
        class="flex flex-wrap items-center gap-2 border-t border-border pt-4 first:border-t-0 first:pt-0"
      >
        <a href={filterHref('source', job.source)}>
          <Badge variant="outline" class="transition-colors hover:bg-accent hover:text-foreground">
            {job.source}
          </Badge>
        </a>
        {#if job.manually_added}
          <Badge variant="secondary">Manually added</Badge>
        {/if}
        {#if posted}<span class="text-xs text-muted-foreground">Posted {posted}</span>{/if}
      </div>
    </div>
  </aside>

  <div class="flex min-w-0 flex-col gap-6 lg:col-start-2 lg:row-start-3">
    {#if job.closed_at}
      {@const closed = formatDate(job.closed_at)}
      <div class="rounded-md border border-border bg-secondary px-4 py-3 text-sm">
        This position is no longer accepting applications{#if closed}
          (closed {closed}){/if}.
      </div>
    {/if}

    {#if job.description}
      <!-- Description is server-sanitized HTML (see internal/sources), safe to render. -->
      <div class="job-description text-sm leading-relaxed">{@html job.description}</div>
    {/if}
  </div>
</article>

{#if showReport}
  <ReportDialog slug={job.public_slug} onClose={() => (showReport = false)} />
{/if}

<style>
  .job-description :global(h1),
  .job-description :global(h2),
  .job-description :global(h3),
  .job-description :global(h4) {
    margin-top: 1.25rem;
    margin-bottom: 0.5rem;
    font-weight: 600;
  }

  .job-description :global(p) {
    margin: 0.5rem 0;
  }

  .job-description :global(ul),
  .job-description :global(ol) {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }

  .job-description :global(li) {
    display: list-item;
    list-style: disc outside;
    margin: 0.25rem 0;
  }

  /* ATS boards (e.g. Greenhouse) wrap each <li> in a block <p>; collapse its
     margins so the bullet sits beside the text instead of on its own line. */
  .job-description :global(li) > :global(p) {
    margin: 0;
  }

  .job-description :global(a) {
    text-decoration: underline;
  }

  .job-description :global(b),
  .job-description :global(strong) {
    font-weight: 600;
  }
</style>
