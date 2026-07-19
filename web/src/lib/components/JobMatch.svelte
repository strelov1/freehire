<script lang="ts">
  import { resolve } from '$app/paths';
  import { FileText, Lock } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { resolveMatchState, matchBarSegments } from '$lib/jobMatch';
  import { profileStore } from '$lib/profile.svelte';
  import type { Job, JobMatch } from '$lib/types';
  import { Button } from '$lib/ui';
  import MatchSummary from './MatchSummary.svelte';

  // The job is server-rendered; only this personal signal hydrates client-side.
  let { job }: { job: Job } = $props();

  // The fetched match — set only in the `ready` state. Read by the template/segments
  // but never by `state`, so setting it can't re-trigger the fetch effect below.
  let match = $state.raw<JobMatch | null>(null);

  // One-time profile load for signed-in viewers (SSR-safe no-op otherwise); the
  // block resolves to `loading` until it settles so the CTA doesn't flash.
  $effect(() => {
    if (isAuthenticated()) profileStore.ensureLoaded();
  });

  const blockState = $derived(
    resolveMatchState({
      jobSkills: job.skills ?? [],
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

  // Static teaser for the locked states — deliberately not a real score.
  const teaser = { percent: 76, have: ['React', 'Docker', 'SQL'], missing: ['Kafka'] };

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
    <!-- Locked teaser: lightly-blurred static content (not a real score) + a footer CTA. -->
    <div class="pointer-events-none select-none space-y-3 opacity-90 blur-[1.5px]" aria-hidden="true">
      <div class="flex items-baseline justify-between gap-2">
        <span class="text-2xl font-bold tabular-nums leading-none">{teaser.percent}%</span>
        <span class="text-xs text-muted-foreground">4 of 5 skills</span>
      </div>
      <div class="flex h-2 overflow-hidden rounded bg-secondary">
        <div class="h-full bg-brand" style="width: {teaser.percent}%"></div>
      </div>
      <div class="flex flex-wrap gap-1.5">
        {#each teaser.have as skill (skill)}<span class={haveChip}>{skill}</span>{/each}
        {#each teaser.missing as skill (skill)}<span class={missChip}>{skill}</span>{/each}
      </div>
    </div>
    <div class="flex items-center justify-between gap-2 border-t border-dashed border-border pt-3">
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
