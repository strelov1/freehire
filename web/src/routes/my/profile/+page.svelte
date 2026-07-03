<script lang="ts">
  import { Pencil, Sparkles, Trash2 } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { getProfileVerdict } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import { profileStore } from '$lib/profile.svelte';
  import { categoryParams } from '$lib/skillGap';
  import type { Verdict } from '$lib/types';
  import { Button } from '$lib/ui';
  import ProfileEditModal from '$lib/components/ProfileEditModal.svelte';
  import States from '$lib/components/States.svelte';

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let editing = $state(false);
  let actionError = $state<string | null>(null);

  const profile = $derived(profileStore.profile);
  const verdictHref = resolve('/my/profile/verdict');

  // Market-coverage headline over the profile's own role. Best-effort: a failed fetch
  // (or a profile with no specialization) simply omits the coverage block.
  let coverage = $state.raw<Verdict | null>(null);
  let coverageGeneration = 0;

  async function load() {
    status = 'loading';
    try {
      await profileStore.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  async function loadCoverage() {
    const gen = ++coverageGeneration;
    if (!profile || profile.specializations.length === 0) {
      coverage = null;
      return;
    }
    try {
      const v = await getProfileVerdict();
      if (gen === coverageGeneration) coverage = v;
    } catch {
      if (gen === coverageGeneration) coverage = null;
    }
  }

  // Load once the session is confirmed (the boot-time /me resolution may still be in
  // flight when the page is opened directly). Sign-out reset is handled centrally in the
  // root layout, so this only needs to (re)load when authenticated.
  $effect(() => {
    if (isAuthenticated()) void load();
  });

  // Recompute coverage whenever the profile changes (save/clear reassigns it).
  $effect(() => {
    void profile;
    void loadCoverage();
  });

  // Link a missing skill to the job search under the profile's role plus that skill.
  function jobsHref(specializations: string[], skill: string): string {
    const params = categoryParams(specializations);
    params.append('skills', skill);
    return `/jobs?${params}`;
  }

  async function remove() {
    if (!window.confirm('Delete your profile?')) return;
    actionError = null;
    try {
      await profileStore.clear();
    } catch {
      actionError = 'Could not delete the profile. Please try again.';
    }
  }
</script>

<svelte:head>
  <title>Profile — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to set up your profile.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load your profile." />
  {:else}
    <div class="flex flex-col gap-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="flex flex-col gap-1">
          <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
          <p class="text-sm text-muted-foreground">
            Your specializations and skills — reuse them to find relevant work.
          </p>
        </div>
        {#if profile}
          <div class="flex items-center gap-1">
            <Button variant="ghost" size="icon" href={verdictHref} aria-label="Market coverage">
              <Sparkles class="size-4" />
            </Button>
            <Button variant="ghost" size="icon" onclick={() => (editing = true)} aria-label="Edit profile">
              <Pencil class="size-4" />
            </Button>
            <Button variant="ghost" size="icon" onclick={remove} aria-label="Delete profile">
              <Trash2 class="size-4" />
            </Button>
          </div>
        {/if}
      </div>

      {#if actionError}
        <p class="text-sm text-destructive">{actionError}</p>
      {/if}

      {#if !profile}
        <div
          class="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-14 text-center"
        >
          <p class="text-sm text-muted-foreground">You haven't set up your profile yet.</p>
          <Button variant="primary" onclick={() => (editing = true)}>Set up your profile</Button>
        </div>
      {:else}
        <div class="flex flex-col gap-4 rounded-xl border border-border bg-card p-5">
          <div class="flex flex-col gap-2">
            <span class="text-sm font-medium">Specializations</span>
            <div class="flex flex-wrap gap-1.5">
              {#each profile.specializations as spec (spec)}
                <span class="rounded-full border border-border px-2.5 py-1 text-sm text-muted-foreground">
                  {categoryLabel(spec)}
                </span>
              {/each}
            </div>
          </div>
          <div class="flex flex-col gap-2">
            <span class="text-sm font-medium">Skills</span>
            <div class="flex flex-wrap gap-1.5">
              {#each profile.skills as skill (skill)}
                <span class="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">{skill}</span>
              {/each}
            </div>
          </div>

          {#if coverage && coverage.total > 0}
            <div class="mt-1 flex flex-col gap-2 border-t border-border pt-4">
              <div class="flex flex-col gap-1">
                <span class="text-xs text-muted-foreground">
                  <span class="font-semibold text-foreground">{coverage.coverage_percent}% coverage</span>
                  · {coverage.covered.toLocaleString('en-US')} of {coverage.total.toLocaleString('en-US')} open vacancies
                </span>
                <div class="h-1.5 overflow-hidden rounded bg-secondary">
                  <div class="h-full rounded bg-primary" style="width: {coverage.coverage_percent}%"></div>
                </div>
              </div>
              {#if coverage.gaps.length > 0}
                <div class="flex flex-wrap items-center gap-1">
                  <span class="text-xs text-muted-foreground">Add to reach more:</span>
                  {#each coverage.gaps.slice(0, 5) as gap (gap.name)}
                    <a
                      href={jobsHref(profile.specializations, gap.name)}
                      class="rounded border border-dashed border-border px-1.5 py-0.5 text-xs text-muted-foreground hover:text-foreground"
                      title="+{gap.new_vacancies.toLocaleString('en-US')} vacancies"
                    >{gap.name}</a>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

{#if editing}
  <ProfileEditModal {profile} onClose={() => (editing = false)} />
{/if}
