<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check, FileText, Lock, TriangleAlert } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import {
    resolveMatchState,
    matchBarSegments,
    matchTeaser,
    teaserChips,
    partitionBlockers,
  } from '$lib/jobMatch';
  import { profileStore } from '$lib/profile.svelte';
  import type { BlockerSeverity, Job, JobMatchResult } from '$lib/types';
  import { Button } from '$lib/ui';
  import MatchSummary from './MatchSummary.svelte';

  // The job is server-rendered; only this personal signal hydrates client-side.
  let { job }: { job: Job } = $props();

  // The fetched match — set only in the `ready` state. Read by the template/segments
  // but never by `state`, so setting it can't re-trigger the fetch effect below.
  let match = $state.raw<JobMatchResult | null>(null);

  // One-time profile load for signed-in viewers (SSR-safe no-op otherwise); the
  // block resolves to `loading` until it settles so the CTA doesn't flash.
  $effect(() => {
    if (isAuthenticated()) profileStore.ensureLoaded();
  });

  // Top-level `skills` is the served dictionary facet; absent on a non-tech posting.
  const jobSkills = $derived(job.skills ?? []);

  const blockState = $derived(
    resolveMatchState({
      jobSkills,
      authenticated: isAuthenticated(),
      profileLoaded: profileStore.loaded,
      profileSkills: profileStore.profile?.skills,
    }),
  );

  // Fetch the real match only when ready; re-fetch on navigation to another job.
  $effect(() => {
    const slug = job.public_slug; // track the current job
    match = null;
    if (blockState !== 'ready') return;
    api.getJobMatch(slug)
      .then((m) => {
        if (job.public_slug === slug) match = m;
      })
      .catch(() => {});
  });

  const segments = $derived(match ? matchBarSegments(match) : { exact: 0, adjacent: 0 });
  const blockers = $derived(partitionBlockers(match?.blockers));

  // Warning tone by severity: hard constraints (work auth, certs) read as blocking,
  // fit constraints (location, language) as softer cautions.
  function toneText(severity: BlockerSeverity): string {
    if (severity === 'hard') return 'text-destructive';
    if (severity === 'medium') return 'text-amber-700 dark:text-amber-500';
    return 'text-muted-foreground';
  }

  // The locked states' teaser — deliberately not a real score, but built from this job's
  // own skills and seeded from its slug, so it agrees with the same job's card in the
  // feed instead of naming skills the posting never mentioned.
  // Gated on the block state, not just on the template branch it renders in, so "can the
  // teaser reach a viewer with a real match?" is answered here rather than by reading the
  // markup — and so the derivation doesn't run for the states that discard it.
  const teaser = $derived(
    blockState === 'guest' || blockState === 'no-profile'
      ? matchTeaser(job.public_slug, jobSkills)
      : null,
  );
  // Three is what a ~285px sidebar column fits at natural chip width; a fourth forced
  // every name down to an unreadable stub ("anal…", "c…"). teaserChips makes sure the
  // short row still carries a missing skill.
  const TEASER_CHIPS = 3;
  const teaserSkills = $derived(
    teaser ? teaserChips(jobSkills, teaser.missing, TEASER_CHIPS) : [],
  );

  const chip = 'rounded-full border px-2 py-0.5 text-xs font-medium';
  const haveChip = `${chip} border-brand/30 bg-brand-muted text-brand-strong`;
  const adjChip = `${chip} border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500`;
  const missChip = `${chip} border-destructive/30 bg-destructive/10 text-destructive`;

  const profileHref = resolve('/my/profile');
</script>

<section
  class="flex flex-col gap-3 border-t border-border pt-4 first:border-t-0 first:pt-0"
  aria-label="Profile match"
>
  <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Profile match</p>

  {#if blockState === 'no-skills'}
    <p class="text-sm text-muted-foreground">Not enough data to compare this job to your profile.</p>
  {:else if blockState === 'guest' || blockState === 'no-profile'}
    <!-- Locked teaser: lightly-blurred figures (not a real score) over the job's own
         skills + a footer CTA. A single-skill job has no teaser (nothing to contrast),
         and then the call-to-action stands alone under the heading. -->
    {#if teaser}
      <div class="pointer-events-none select-none space-y-3 opacity-90 blur-[1.5px]" aria-hidden="true">
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-2xl font-bold tabular-nums leading-none">{teaser.percent}%</span>
          <span class="shrink-0 text-xs text-muted-foreground">
            {teaser.matched} of {teaser.total} skills
          </span>
        </div>
        <div class="flex h-2 overflow-hidden rounded bg-secondary">
          <div class="h-full bg-brand" style="width: {teaser.percent}%"></div>
        </div>
        <!-- Chips keep their natural width and the row clips at the panel edge, rather
             than every name being ellipsised to fit — a clipped last chip still reads,
             an "anal…" does not. -->
        <div class="flex flex-nowrap gap-1.5 overflow-hidden">
          {#each teaserSkills as skill (skill)}
            <span class={`${teaser.missing.has(skill) ? missChip : haveChip} whitespace-nowrap`}>
              {skill}
            </span>
          {/each}
        </div>
      </div>
    {/if}
    <!-- The dashed rule divides the teaser from the call-to-action; with no teaser above
         it there is nothing to divide, so it goes. -->
    <div
      class={[
        'flex items-center justify-between gap-2',
        teaser && 'border-t border-dashed border-border pt-3',
      ]}
    >
      {#if blockState === 'guest'}
        <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Lock class="size-3.5 shrink-0" />Sign in to see your match
        </span>
        <Button variant="primary" size="sm" onclick={() => openAuthDialog('login')}>Sign in</Button>
      {:else}
        <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
          <FileText class="size-3.5 shrink-0" />Add your skills or upload a CV
        </span>
        <Button variant="primary" size="sm" href={profileHref}>Upload CV</Button>
      {/if}
    </div>
  {:else if blockState === 'ready' && match}
    <!-- Real match: percent + two-colour bar + three skill groups. -->
    <div class="flex items-baseline justify-between gap-2">
      <span class="text-2xl font-bold tabular-nums leading-none">{match.coverage_percent}%</span>
      <span class="text-xs text-muted-foreground">
        {match.exact_count} of {match.total} skills{#if match.adjacent_count}
          · {match.adjacent_count} close{/if}
      </span>
    </div>
    <div class="flex h-2 overflow-hidden rounded bg-secondary">
      <div class="h-full bg-brand transition-all" style="width: {segments.exact}%"></div>
      <div class="h-full bg-amber-500 transition-all" style="width: {segments.adjacent}%"></div>
    </div>

    {#if match.matched.length}
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-brand"></span>You have
        </span>
        <div class="flex flex-wrap gap-1.5">
          {#each match.matched as skill (skill)}<span class={haveChip}>{skill}</span>{/each}
        </div>
      </div>
    {/if}

    {#if match.adjacent.length}
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-amber-500"></span>Close — you have a related skill
        </span>
        <div class="flex flex-wrap gap-1.5">
          {#each match.adjacent as a (a.name)}
            <span class={adjChip}>{a.name} <span class="opacity-70">· you have {a.via}</span></span>
          {/each}
        </div>
      </div>
    {/if}

    {#if match.missing.length}
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-destructive"></span>Missing
        </span>
        <div class="flex flex-wrap gap-1.5">
          {#each match.missing as skill (skill)}<span class={missChip}>{skill}</span>{/each}
        </div>
      </div>
    {/if}

    {#if blockers.unmet.length || blockers.met.length}
      <!-- Deterministic hard-constraint checks (years, education, certs, work auth,
           location) — advisory warnings, never hiding the job. -->
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-muted-foreground"></span>Requirements
        </span>
        <ul class="flex flex-col gap-1">
          {#each blockers.unmet as b (b.category + b.reason)}
            <li class="flex items-start gap-1.5 text-xs {toneText(b.severity)}">
              <TriangleAlert class="mt-0.5 size-3.5 shrink-0" />
              <span>{b.reason}</span>
            </li>
          {/each}
          {#each blockers.met as b (b.category + b.reason)}
            <li class="flex items-start gap-1.5 text-xs text-muted-foreground">
              <Check class="mt-0.5 size-3.5 shrink-0 text-brand" />
              <span>{b.reason}</span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <!-- The deterministic bar above is instant and free; the LLM deep-dive is opt-in
         below it, computed only on an explicit action and cached per (user, job). -->
    <MatchSummary slug={job.public_slug} />
  {:else}
    <!-- Skeleton: the profile is still loading, or the match is in flight (ready but
         not yet fetched). A signed-in profiled viewer never sees the locked teaser. -->
    <div class="h-2 animate-pulse rounded bg-secondary"></div>
    <div class="flex gap-1.5">
      <div class="h-5 w-14 animate-pulse rounded-full bg-secondary"></div>
      <div class="h-5 w-16 animate-pulse rounded-full bg-secondary"></div>
    </div>
  {/if}
</section>
