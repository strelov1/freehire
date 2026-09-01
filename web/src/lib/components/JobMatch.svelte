<script lang="ts">
  import { resolve } from '$app/paths';
  import { Ban, Check, FileText, Lock, RotateCcw, TriangleAlert } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import {
    resolveMatchState,
    matchBarSegments,
    matchTeaser,
    teaserChips,
    partitionBlockers,
    claimSkill,
    toneText,
    haveChipClass,
    adjacentChipClass,
    missingChipClass,
  } from '$lib/jobMatch';
  import { profileStore } from '$lib/profile.svelte';
  import type { Job, JobMatchResult } from '$lib/types';
  import { Button } from '$lib/ui';
  import MatchSummary from './MatchSummary.svelte';
  import SkillIcon from './SkillIcon.svelte';
  import { skillLabel } from '$lib/facets';

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

  // A write in progress or just made. `overlay` is the match as it reads once a skill is
  // claimed, rendered in place of the fetched one until the server's own answer lands.
  // `done` is the last write, which the confirmation line names and undoes: only a claim
  // moved the match, so only it carries the reading to put back.
  type Done =
    | { kind: 'claim'; skill: string; before: JobMatchResult }
    | { kind: 'avoid'; skill: string }
    | { kind: 'unavoid'; skill: string };

  let claiming = $state<string | null>(null);
  let overlay = $state.raw<JobMatchResult | null>(null);
  let done = $state.raw<Done | null>(null);
  let failed = $state<string | null>(null);
  let pending = $state(false);

  // The skills the viewer has said they want to avoid. Read straight off the profile the
  // block already holds, so an avoided chip is marked on every job that asks for it without
  // a request, and the mark updates the moment a write applies.
  const avoided = $derived(
    new Set((profileStore.profile?.excluded_skills ?? []).map((s) => s.toLowerCase())),
  );
  const isAvoided = (skill: string) => avoided.has(skill.toLowerCase());

  // Fetch the real match only when ready; re-fetch on navigation to another job.
  $effect(() => {
    const slug = job.public_slug; // track the current job
    match = null;
    overlay = null;
    claiming = null;
    done = null;
    failed = null;
    if (blockState !== 'ready') return;
    api.getJobMatch(slug)
      .then((m) => {
        if (job.public_slug === slug) match = m;
      })
      .catch(() => {});
  });

  // What the block renders: the optimistic reading while a claim is unreconciled, the
  // fetched match otherwise.
  const view = $derived(overlay ?? match);

  /** Put the block back to a reading it showed before — dropping the overlay when that
   *  reading is the fetched match itself, so an abandoned claim leaves no overlay behind. */
  function restore(reading: JobMatchResult | null) {
    overlay = reading === match ? null : reading;
  }

  /** Replace the optimistic reading with the server's own, which also accounts for any
   *  skill the claim newly made adjacent. A failed refetch keeps the overlay: the write
   *  landed, so reverting would misreport the profile. */
  async function reconcile(slug: string) {
    try {
      const fresh = await api.getJobMatch(slug);
      if (job.public_slug !== slug) return;
      match = fresh;
      overlay = null;
    } catch {
      // Keep showing the optimistic reading.
    }
  }

  async function claim(skill: string) {
    const before = view;
    if (!before || pending) return;
    const slug = job.public_slug;
    pending = true;
    failed = null;
    claiming = null;
    overlay = claimSkill(before, skill);
    try {
      await profileStore.addSkill(skill);
      // Navigated on mid-write: the claim stands, but its confirmation belongs to the job
      // that was open, and the next job's block has already reset itself.
      if (job.public_slug !== slug) return;
      done = { kind: 'claim', skill, before };
      await reconcile(slug);
    } catch {
      if (job.public_slug !== slug) return;
      restore(before);
      failed = skill;
    } finally {
      pending = false;
    }
  }

  /** Record (or lift) an avoid, answering whether it landed on the job still open. Nothing
   *  about the match can change — the server scores against the profile's skills, not its
   *  exclusions — so there is no overlay to apply and no refetch to make. The chip's mark
   *  follows the store, which the write updates on success. */
  async function writeAvoid(skill: string, avoid: boolean): Promise<boolean> {
    if (pending) return false;
    const slug = job.public_slug;
    pending = true;
    failed = null;
    claiming = null;
    try {
      await (avoid ? profileStore.avoidSkill(skill) : profileStore.unavoidSkill(skill));
      return job.public_slug === slug;
    } catch {
      if (job.public_slug === slug) failed = skill;
      return false;
    } finally {
      pending = false;
    }
  }

  async function setAvoided(skill: string, avoid: boolean) {
    if (await writeAvoid(skill, avoid)) done = { kind: avoid ? 'avoid' : 'unavoid', skill };
  }

  /** Reverse the last write. Only a claim moved the match, so only it restores a reading. */
  async function undoLast() {
    if (!done || pending) return;
    const last = done;
    if (last.kind !== 'claim') {
      // Undoing an avoid is the opposite write; a successful one leaves nothing to undo again.
      if (await writeAvoid(last.skill, last.kind === 'unavoid')) done = null;
      return;
    }
    const shown = view;
    const slug = job.public_slug;
    pending = true;
    failed = null;
    overlay = last.before;
    done = null;
    try {
      await profileStore.removeSkill(last.skill);
      await reconcile(slug);
    } catch {
      if (job.public_slug !== slug) return;
      restore(shown);
      done = last;
      failed = last.skill;
    } finally {
      pending = false;
    }
  }

  const segments = $derived(view ? matchBarSegments(view) : { exact: 0, adjacent: 0 });
  const blockers = $derived(partitionBlockers(view?.blockers));

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

  const haveChip = haveChipClass;
  const adjChip = adjacentChipClass;
  const missChip = missingChipClass;
  // A not-held chip is a control: pressing it asks whether the viewer holds the skill after
  // all, or wants to be shown less of it. The ring marks which one the open row belongs to.
  // An avoided skill keeps its place in the group — the job still asks for it and the score
  // still counts it — but reads as struck out rather than merely unmet.
  const claimable = (base: string, open: boolean, skill: string) =>
    [
      base,
      'transition-shadow hover:brightness-95 disabled:opacity-60',
      open && 'ring-2 ring-foreground/30',
      isAvoided(skill) && 'line-through opacity-55',
    ]
      .filter(Boolean)
      .join(' ');

  const chipLabel = (skill: string) => (isAvoided(skill) ? `${skill} — you avoid this skill` : undefined);

  const profileHref = resolve('/my/profile');

  /** Open the claim row for a skill, or close it if it is already the open one. Only one
   *  is open at a time — the row names its skill, which is what keeps it tied to the chip
   *  without anchoring a floating layer inside a narrow column. */
  function toggleClaimRow(skill: string) {
    claiming = claiming === skill ? null : skill;
  }
</script>

{#snippet claimRow(skill: string)}
  <!-- The row expands under the group rather than floating over it: the sidebar column is
       too narrow for an anchored popover, and naming the skill in the row's own text keeps
       it tied to the chip that opened it. Styled as a bare row of pills, not a panel, so
       every inline disclosure in the app reads the same.
       It names the skill instead of asking "do you have it?": that question fitted one
       answer, and the row now carries two. -->
  <div class="flex flex-wrap items-center gap-1.5">
    <span class="flex items-center gap-1 text-xs font-medium text-foreground">
      <SkillIcon slug={skill} />{skillLabel(skill)}
    </span>
    <button
      type="button"
      disabled={pending}
      onclick={() => claim(skill)}
      class="inline-flex items-center gap-1.5 rounded-full border border-transparent bg-brand-muted px-2.5 py-1 text-xs font-medium text-brand-strong transition-colors hover:brightness-95 disabled:opacity-50"
    >
      <Check class="size-3.5" aria-hidden="true" /> I have it
    </button>
    {#if isAvoided(skill)}
      <button
        type="button"
        disabled={pending}
        onclick={() => setAvoided(skill, false)}
        class="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-muted-foreground/40 disabled:opacity-50"
      >
        <RotateCcw class="size-3.5" aria-hidden="true" /> Stop avoiding
      </button>
    {:else}
      <button
        type="button"
        disabled={pending}
        onclick={() => setAvoided(skill, true)}
        class="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-muted-foreground/40 disabled:opacity-50"
      >
        <Ban class="size-3.5" aria-hidden="true" /> Avoid
      </button>
    {/if}
  </div>
{/snippet}

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && claiming) claiming = null;
  }}
/>

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
              <SkillIcon slug={skill} />{skillLabel(skill)}
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

    {#if blockState === 'guest'}
      <!-- The deep-dive offer stands on its own for a guest: it needs no match to make
           sense, and it is the stronger pitch of the two. Its button opens sign-in rather
           than the analysis page — MatchSummary handles that. Withheld from the no-profile
           state, whose own "Upload CV" call-to-action sits directly above and would simply
           be repeated. -->
      <MatchSummary slug={job.public_slug} />
    {/if}
  {:else if blockState === 'ready' && view}
    <!-- Real match: percent + two-colour bar + three skill groups. -->
    <div class="flex items-baseline justify-between gap-2">
      <span class="text-2xl font-bold tabular-nums leading-none">{view.coverage_percent}%</span>
      <span class="text-xs text-muted-foreground">
        {view.exact_count} of {view.total} skills{#if view.adjacent_count}
          · {view.adjacent_count} close{/if}
      </span>
    </div>
    <div class="flex h-2 overflow-hidden rounded bg-secondary">
      <div class="h-full bg-brand transition-all" style="width: {segments.exact}%"></div>
      <div class="h-full bg-warning transition-all" style="width: {segments.adjacent}%"></div>
    </div>

    {#if view.matched.length}
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-brand"></span>You have
        </span>
        <div class="flex flex-wrap gap-1.5">
          {#each view.matched as skill (skill)}<span class={haveChip}><SkillIcon slug={skill} />{skillLabel(skill)}</span>{/each}
        </div>
      </div>
    {/if}

    {#if view.adjacent.length}
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-warning"></span>Close — you have a related skill
        </span>
        <div class="flex flex-wrap gap-1.5">
          {#each view.adjacent as a (a.name)}
            <button
              type="button"
              class={claimable(adjChip, claiming === a.name, a.name)}
              aria-expanded={claiming === a.name}
              aria-label={chipLabel(a.name)}
              disabled={pending}
              onclick={() => toggleClaimRow(a.name)}
            >
              <SkillIcon slug={a.name} />{skillLabel(a.name)} <span class="opacity-70">· you have {a.via}</span>
            </button>
          {/each}
        </div>
        {#if claiming && view.adjacent.some((a) => a.name === claiming)}
          {@render claimRow(claiming)}
        {/if}
      </div>
    {/if}

    {#if view.missing.length}
      <div class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <span class="size-1.5 rounded-full bg-destructive"></span>Missing
        </span>
        <div class="flex flex-wrap gap-1.5">
          {#each view.missing as skill (skill)}
            <button
              type="button"
              class={claimable(missChip, claiming === skill, skill)}
              aria-expanded={claiming === skill}
              aria-label={chipLabel(skill)}
              disabled={pending}
              onclick={() => toggleClaimRow(skill)}
            >
              <SkillIcon slug={skill} />{skillLabel(skill)}
            </button>
          {/each}
        </div>
        {#if claiming && view.missing.includes(claiming)}
          {@render claimRow(claiming)}
        {/if}
      </div>
    {/if}

    {#if done}
      <p class="flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
        {#if done.kind === 'claim'}
          <Check class="size-3.5 shrink-0 text-brand" />
        {:else if done.kind === 'avoid'}
          <Ban class="size-3.5 shrink-0" />
        {:else}
          <RotateCcw class="size-3.5 shrink-0" />
        {/if}
        <span>
          <span class="font-medium text-foreground">{done.skill}</span>
          {#if done.kind === 'claim'}
            added to your profile.
          {:else if done.kind === 'avoid'}
            added to skills you avoid.
          {:else}
            is no longer avoided.
          {/if}
        </span>
        <button
          type="button"
          class="font-medium underline underline-offset-2 hover:text-foreground disabled:opacity-60"
          disabled={pending}
          onclick={undoLast}>Undo</button
        >
      </p>
    {/if}

    {#if failed}
      <p class="text-xs text-destructive">Could not update {failed} in your profile. Try again.</p>
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
