<script lang="ts">
  import { Pencil, Plus, Sparkles, Trash2 } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { facetCounts } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import { categoryParams, computeGap, sortSkillsByCount, type SkillGap } from '$lib/skillGap';
  import type { SearchProfile } from '$lib/types';
  import { Button } from '$lib/ui';
  import States from './States.svelte';

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let listError = $state<string | null>(null);
  const profiles = $derived(searchProfiles.items);

  const newHref = resolve('/my/profiles/new');
  const editHref = (id: number) => resolve('/my/profiles/[id]/edit', { id: String(id) });
  const verdictHref = (id: number) => resolve('/my/profiles/[id]/verdict', { id: String(id) });

  // Per-profile market skill-gap, keyed by profile id. Market skills are cached by the
  // sorted specialization set, so profiles sharing specializations reuse one facet fetch.
  // gapGeneration guards against out-of-order async runs: only the latest loadGaps commits.
  let gaps = $state.raw<Record<number, SkillGap>>({});
  const marketCache = new Map<string, string[]>();
  let gapGeneration = 0;

  async function loadProfiles() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // For each profile with a specialization, fetch its market skills (cached by spec set)
  // and compute the gap against the profile's skills. Best-effort: a failed facet fetch
  // simply omits that profile's gap block.
  async function loadGaps(list: SearchProfile[]) {
    const gen = ++gapGeneration;
    const next: Record<number, SkillGap> = {};
    for (const p of list) {
      if (p.specializations.length === 0) continue;
      const key = [...p.specializations].toSorted().join(',');
      let market = marketCache.get(key);
      if (!market) {
        try {
          const counts = await facetCounts(categoryParams(p.specializations));
          market = sortSkillsByCount(counts.facets?.skills ?? {});
          marketCache.set(key, market);
        } catch {
          continue;
        }
      }
      next[p.id] = computeGap(market, p.skills);
    }
    // Discard a stale run: a newer loadGaps started while we awaited, so it owns `gaps`.
    if (gen === gapGeneration) gaps = next;
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

  // Recompute gaps whenever the profile list changes (create/edit/delete reassigns items).
  $effect(() => {
    const list = profiles;
    if (list.length) void loadGaps(list);
    else gaps = {};
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
          {@const gap = gaps[profile.id]}
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
              {#if gap && gap.total > 0}
                <div class="mt-1 flex flex-col gap-1">
                  <div class="flex items-center gap-2">
                    <span
                      class="whitespace-nowrap text-xs text-muted-foreground"
                      title="{gap.coverage} of {gap.total} top market skills"
                    >
                      Market fit {Math.round((gap.coverage / gap.total) * 100)}%
                    </span>
                    <div class="h-1.5 flex-1 overflow-hidden rounded bg-secondary">
                      <div class="h-full rounded bg-primary" style="width: {(gap.coverage / gap.total) * 100}%"></div>
                    </div>
                  </div>
                  {#if gap.missing.length > 0}
                    <div class="flex flex-wrap items-center gap-1">
                      <span class="text-xs text-muted-foreground">Missing:</span>
                      {#each gap.missing as skill (skill)}
                        <a
                          href={jobsHref(profile.specializations, skill)}
                          class="rounded border border-dashed border-border px-1.5 py-0.5 text-xs text-muted-foreground hover:text-foreground"
                        >{skill}</a>
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
                aria-label="Résumé verdict"
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
