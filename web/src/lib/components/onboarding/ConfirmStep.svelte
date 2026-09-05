<script lang="ts">
  // Specialization, level, and the candidate's profile links. Three fields on one screen
  // because they are all "who you are" — the two facets the feed filters on, and the two
  // addresses that say the same thing to a human reader.
  //
  // The specialization picker is the job filter's grouped-sections-with-counts control,
  // rebuilt locally rather than reused: that component is wired to a FacetStore (URL and
  // localStorage-synced filter state) this page has no use for — the same reason the
  // profile's own Roles editor does not reuse it either.
  import { ChevronDown, Search, X } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { api } from '$lib/api';
  import { FACETS } from '$lib/facets';
  import { CATEGORY_GROUPS } from '$lib/filterSections';
  import { pillClass, pillTitle } from '$lib/components/facets/pill';
  import ProfileLinksFields from './ProfileLinksFields.svelte';
  import type { ProfileLinks } from '$lib/profileLinks';

  interface Props {
    specializations: string[];
    seniorities: string[];
    links: ProfileLinks;
    /** True when at least one link arrived from the CV or the LinkedIn import rather than
     *  being typed here — the only reason to explain where the text came from. */
    linksPrefilled: boolean;
    onSpecializationsChange: (next: string[]) => void;
    onSenioritiesChange: (next: string[]) => void;
    onLinksChange: (next: ProfileLinks) => void;
  }

  let {
    specializations,
    seniorities,
    links,
    linksPrefilled,
    onSpecializationsChange,
    onSenioritiesChange,
    onLinksChange,
  }: Props = $props();

  const seniorityOptions = FACETS.find((f) => f.param === 'seniority')?.options ?? [];

  let specQuery = $state('');
  const specCollapsed = new SvelteSet<string>();

  // Counts come from the live, unfiltered category distribution — best-effort: a failed or
  // unavailable fetch (search down) just shows the picker without counts, the same way
  // CategoryPane already tolerates a null `counts` prop.
  let categoryDist = $state.raw<Record<string, number> | null>(null);
  $effect(() => {
    api
      .facetCounts(new URLSearchParams(), { facets: ['category'] })
      .then((res) => {
        categoryDist = res.facets?.category ?? null;
      })
      .catch(() => {});
  });

  const specGroups = $derived.by(() => {
    const q = specQuery.trim().toLowerCase();
    return CATEGORY_GROUPS.map((g) => ({
      ...g,
      options: q ? g.options.filter((o) => o.label.toLowerCase().includes(q)) : g.options,
    })).filter((g) => g.options.length > 0);
  });

  function toggleSpecialization(value: string) {
    onSpecializationsChange(
      specializations.includes(value)
        ? specializations.filter((v) => v !== value)
        : [...specializations, value],
    );
  }

  function toggleSeniority(value: string) {
    onSenioritiesChange(
      seniorities.includes(value) ? seniorities.filter((v) => v !== value) : [...seniorities, value],
    );
  }
</script>

<!-- A field label with a "Clear" X — same pattern as FacetSection's section header: shown
     only once something is selected, and clearing that field's whole selection at once
     (separate from removing one pill at a time). -->
{#snippet fieldLabel(text: string, count: number, onClear: () => void)}
  <div class="mb-2 flex min-h-6 items-center justify-between gap-2">
    <span class="text-sm font-medium">{text}</span>
    {#if count > 0}
      <button
        type="button"
        onclick={onClear}
        title="Clear {text}"
        aria-label="Clear {text}"
        class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-3.5" />
      </button>
    {/if}
  </div>
{/snippet}

<h2 class="text-xl font-semibold tracking-tight">Confirm your details</h2>
<p class="mt-1 text-sm text-muted-foreground">Everything's optional — pick as many as apply.</p>

<div class="mt-5">
  {@render fieldLabel('Specialization', specializations.length, () => onSpecializationsChange([]))}
</div>
<div class="mb-4 flex items-center gap-2 rounded-lg border border-input px-3 focus-within:ring-2 focus-within:ring-ring">
  <Search class="size-4 shrink-0 text-muted-foreground" />
  <input
    bind:value={specQuery}
    placeholder="Search specializations…"
    aria-label="Search specializations"
    class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
  />
</div>
{#each specGroups as g (g.name)}
  {@const isCollapsed = specCollapsed.has(g.name) && !specQuery}
  <div class="border-t border-border first:border-t-0">
    <button
      type="button"
      class="flex w-full items-center gap-2 py-3"
      onclick={() => (specCollapsed.has(g.name) ? specCollapsed.delete(g.name) : specCollapsed.add(g.name))}
    >
      <ChevronDown class={['size-4 text-muted-foreground transition-transform', isCollapsed && '-rotate-90']} />
      <h3 class="text-sm font-semibold tracking-tight">{g.name}</h3>
    </button>
    {#if !isCollapsed}
      <div class="flex flex-wrap gap-2 pb-3">
        {#each g.options as o (o.value)}
          {@const included = specializations.includes(o.value)}
          <button
            type="button"
            onclick={() => toggleSpecialization(o.value)}
            title={pillTitle(included, false, false)}
            class={pillClass(included, false, 'px-3 py-1.5 text-sm')}
          >
            {o.label}{#if categoryDist}<span class="ml-1 opacity-60 tabular-nums">{(categoryDist[o.value] ?? 0).toLocaleString()}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/each}

<div class="mt-6">
  {@render fieldLabel('Level', seniorities.length, () => onSenioritiesChange([]))}
</div>
<div class="flex flex-wrap gap-2">
  {#each seniorityOptions as o (o.value)}
    {@const selected = seniorities.includes(o.value)}
    <button
      type="button"
      onclick={() => toggleSeniority(o.value)}
      aria-pressed={selected}
      class={[
        'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors',
        selected ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
      ]}
    >
      {o.label}
    </button>
  {/each}
</div>

<div class="mt-6">
  <ProfileLinksFields value={links} onChange={onLinksChange} prefilled={linksPrefilled} />
</div>
