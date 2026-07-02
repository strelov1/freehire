<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { facetCounts, getProfileVerdict } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import FiltersPanel from '$lib/components/FiltersPanel.svelte';
  import States from '$lib/components/States.svelte';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import type { FacetCounts, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';

  const id = $derived(Number(page.params.id));
  const profile = $derived(searchProfiles.items.find((p) => p.id === id));

  // Skills are the measured set (from the profile), never a role filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let filters = $state<FilterStore | null>(null);
  let verdict = $state<Verdict | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let loadError = $state(false);

  // Build the filter store once the profile is known, seeding its role from the
  // profile's specializations when the URL carries no category — so the panel opens on
  // the profile's own role, which the user can then change without touching the profile.
  async function init() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      const p = searchProfiles.items.find((x) => x.id === id);
      const seed = new URLSearchParams(page.url.searchParams);
      if (p && !seed.getAll('category').some((c) => c !== '')) {
        for (const spec of p.specializations) seed.append('category', spec);
      }
      filters = new FilterStore(seed);
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    // Re-init when the session resolves or the profile id changes (SvelteKit may reuse
    // this component across [id] navigations).
    void id;
    if (isAuthenticated()) void init();
  });

  // Reload the verdict + facet counts whenever the applied (debounced) filters change.
  $effect(() => {
    const f = filters;
    if (!f) return;
    void f.applied;
    void reload();
  });

  async function reload() {
    if (!filters) return;
    const params = filtersToParams(filters.applied);
    try {
      [verdict, counts] = await Promise.all([getProfileVerdict(id, params), facetCounts(params)]);
      loadError = false;
    } catch {
      loadError = true;
    }
  }

  // Humanized role label: the selected categories, falling back to the profile's own
  // specializations when none are selected.
  const role = $derived.by(() => {
    const selected = filters?.applied.facets['category']?.values ?? [];
    const source = selected.length ? selected : (profile?.specializations ?? []);
    return source.map(categoryLabel).join(', ');
  });

  // Link a gap skill to the job search under the current role filter plus that skill.
  function gapHref(skill: string): string {
    const params = filters ? filtersToParams(filters.applied) : new URLSearchParams();
    params.append('skills', skill);
    return `/jobs?${params}`;
  }
</script>

<svelte:head>
  <title>Market coverage — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-5xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to see your coverage.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load the verdict." />
  {:else if profile === undefined || filters === null}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">This profile no longer exists.</p>
      <Button variant="primary" href={resolve('/my/profiles')}>Back to profiles</Button>
    </div>
  {:else}
    <div class="grid gap-6 md:grid-cols-[260px_1fr]">
      <aside class="md:sticky md:top-6 md:self-start">
        <FiltersPanel store={filters} exclude={excludeFacets} {counts} />
      </aside>
      <main>
        {#if loadError}
          <States state="error" message="Couldn't load the coverage." />
        {:else if verdict === null}
          <States state="loading" />
        {:else}
          <VerdictView {verdict} name={profile.name} {role} {gapHref} />
        {/if}
      </main>
    </div>
  {/if}
</div>
