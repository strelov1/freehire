<script lang="ts">
  import { Pencil, Plus, Sparkles, Trash2 } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { getProfileVerdict } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import { categoryParams } from '$lib/skillGap';
  import type { SearchProfile, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';
  import States from './States.svelte';

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let listError = $state<string | null>(null);
  const profiles = $derived(searchProfiles.items);

  const newHref = resolve('/my/profiles/new');
  const editHref = (id: number) => resolve('/my/profiles/[id]/edit', { id: String(id) });
  const verdictHref = (id: number) => resolve('/my/profiles/[id]/verdict', { id: String(id) });

  // Per-profile market coverage headline, keyed by profile id. coverageGeneration guards
  // against out-of-order async runs: only the latest loadCoverage commits.
  let coverage = $state.raw<Record<number, Verdict>>({});
  let coverageGeneration = 0;

  async function loadProfiles() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Fetch each profile's coverage verdict (over its own specializations). Best-effort:
  // a failed fetch simply omits that profile's coverage block.
  async function loadCoverage(list: SearchProfile[]) {
    const gen = ++coverageGeneration;
    const next: Record<number, Verdict> = {};
    for (const p of list) {
      if (p.specializations.length === 0) continue;
      try {
        next[p.id] = await getProfileVerdict(p.id);
      } catch {
        continue;
      }
    }
    // Discard a stale run: a newer loadCoverage started while we awaited, so it owns state.
    if (gen === coverageGeneration) coverage = next;
  }

  // Build a /jobs link filtered to a profile's specialization(s) plus one missing skill.
  function jobsHref(specializations: string[], skill: string): string {
    const params = categoryParams(specializations);
    params.append('skills', skill);
    return `/jobs?${params}`;
  }

  // Load once the session is confirmed (the boot-time /me resolution may still be in
  // flight when the page is opened directly), mirroring ApiKeysView. Reset the per-user
  // cache on sign-out so a different user does not see the previous one's profiles.
  $effect(() => {
    if (isAuthenticated()) {
      void loadProfiles();
    } else {
      searchProfiles.reset();
    }
  });

  // Recompute coverage whenever the profile list changes (create/edit/delete reassigns items).
  $effect(() => {
    const list = profiles;
    if (list.length) void loadCoverage(list);
    else coverage = {};
  });

  async function remove(p: SearchProfile) {
    if (!window.confirm(`Delete profile “${p.name}”?`)) return;
    listError = null;
    try {
      await searchProfiles.remove(p.id);
    } catch {
      listError = 'Could not delete the profile. Please try again.';
    }
  }
</script>

{#if !isAuthenticated()}
  <div class="flex flex-col items-center gap-3 py-12 text-center">
    <p class="text-sm text-muted-foreground">Sign in to create profiles.</p>
    <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
  </div>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="flex flex-col gap-1">
        <h1 class="text-2xl font-semibold tracking-tight">Profiles</h1>
        <p class="text-sm text-muted-foreground">
          Named sets of specializations and skills you can reuse to find relevant work.
        </p>
      </div>
      <Button variant="primary" href={newHref}>
        <Plus class="size-4" />
        New profile
      </Button>
    </div>

    {#if listError}
      <p class="text-sm text-destructive">{listError}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your profiles." />
    {:else if profiles.length === 0}
      <div
        class="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-14 text-center"
      >
        <p class="text-sm text-muted-foreground">No profiles yet.</p>
        <Button variant="primary" href={newHref}>
          <Plus class="size-4" />
          Create your first profile
        </Button>
      </div>
    {:else}
      <ul class="flex flex-col gap-3">
        {#each profiles as profile (profile.id)}
          {@const cov = coverage[profile.id]}
          <li
            class="flex items-start justify-between gap-3 rounded-xl border border-border bg-card p-4 transition-colors hover:border-foreground/20"
          >
            <div class="flex min-w-0 flex-col gap-2">
              <span class="truncate text-sm font-semibold">{profile.name}</span>
              <div class="flex flex-wrap gap-1">
                {#each profile.specializations as spec (spec)}
                  <span
                    class="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground"
                  >
                    {categoryLabel(spec)}
                  </span>
                {/each}
              </div>
              <div class="flex flex-wrap gap-1">
                {#each profile.skills as skill (skill)}
                  <span
                    class="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground"
                  >
                    {skill}
                  </span>
                {/each}
              </div>
              {#if cov && cov.total > 0}
                <div class="mt-1 flex flex-col gap-1">
                  <div class="flex items-center gap-2">
                    <span
                      class="whitespace-nowrap text-xs text-muted-foreground"
                      title="{cov.covered} of {cov.total} open vacancies"
                    >
                      Coverage {cov.coverage_percent}% · {cov.covered.toLocaleString('en-US')}/{cov.total.toLocaleString('en-US')}
                    </span>
                    <div class="h-1.5 flex-1 overflow-hidden rounded bg-secondary">
                      <div class="h-full rounded bg-primary" style="width: {cov.coverage_percent}%"></div>
                    </div>
                  </div>
                  {#if cov.gaps.length > 0}
                    <div class="flex flex-wrap items-center gap-1">
                      <span class="text-xs text-muted-foreground">Add:</span>
                      {#each cov.gaps.slice(0, 5) as gap (gap.name)}
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
            <div class="flex shrink-0 items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                href={verdictHref(profile.id)}
                aria-label="Market coverage"
              >
                <Sparkles class="size-4" />
              </Button>
              <Button variant="ghost" size="icon" href={editHref(profile.id)} aria-label="Edit profile">
                <Pencil class="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                onclick={() => remove(profile)}
                aria-label="Delete profile"
              >
                <Trash2 class="size-4" />
              </Button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
