<script lang="ts">
  // The tour's feed step: specialization, level, skills and where — the search profile, on
  // one screen. The match count that answers it lives in the page footer, beside Continue.
  import { Check, Search, X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { CATEGORY_GROUPS } from '#lib/filterSections.js';
  import { rankOptionMatches } from '#lib/facets.js';
  import { FACET_ALIASES } from '#lib/generated/contracts.js';
  import { TECH_CATEGORY_VALUES } from '#lib/generated/contracts.js';
  import { orderSpecializations } from '#lib/onboardingSteps.js';
  import { MAX_SPECIALIZATIONS } from '#lib/profileLimits.js';
  import { loadSkillDistribution, searchSkillOptions } from '#lib/skillDictionary.js';
  import { pillClass, pillCountClass } from '#lib/components/facets/pill.js';
  import RemoteSearchSelect from '#lib/components/facets/RemoteSearchSelect.svelte';
  import LocationPreferencesFields from '#lib/components/profile/LocationPreferencesFields.svelte';
  import SeniorityPills from '#lib/components/profile/SeniorityPills.svelte';
  import type { FacetOption } from '#lib/facets.js';
  import type { DerivedLocation, LocationPreferences } from '#lib/types.js';

  interface Props {
    specializations: string[];
    seniorities: string[];
    skills: string[];
    location: LocationPreferences | null;
    derivedLocation: DerivedLocation | null | undefined;
    /** The values above came from a CV parsed on the previous step. */
    fromCv: boolean;
    onSpecializationsChange: (next: string[]) => void;
    onSenioritiesChange: (next: string[]) => void;
    onSkillsChange: (next: string[]) => void;
    onLocationChange: (next: LocationPreferences | null) => void;
  }

  let {
    specializations,
    seniorities,
    skills,
    location,
    derivedLocation,
    fromCv,
    onSpecializationsChange,
    onSenioritiesChange,
    onSkillsChange,
    onLocationChange,
  }: Props = $props();

  // ---- specialization ----

  // How many open jobs each specialization holds. Best-effort: without it the pills show
  // no counts and keep the vocabulary's own order within each half.
  let counts = $state.raw<Record<string, number> | null>(null);
  $effect(() => {
    api
      .facetCounts(new URLSearchParams(), { facets: ['category'] })
      .then((res) => (counts = res.facets?.category ?? null))
      .catch(() => {});
  });

  const TECH = new Set<string>(TECH_CATEGORY_VALUES);
  const ALL = CATEGORY_GROUPS.flatMap((g) => g.options);
  // Twelve pills fit two or three rows on a phone; "Show all" opens the rest.
  const FIRST = 12;

  let query = $state('');
  let showAll = $state(false);
  let capError = $state<string | null>(null);

  const visible = $derived.by(() => {
    // The same aliases the filter panel's specialization search reads ("swe", "devops").
    if (query.trim()) return rankOptionMatches(ALL, query, FACET_ALIASES);
    const ordered = orderSpecializations(ALL, counts, TECH);
    const shown = showAll ? ordered : ordered.slice(0, FIRST);
    // A pick outside the first twelve (from a CV, or from before) must stay visible to be
    // unpicked.
    return [...shown, ...ordered.filter((o) => specializations.includes(o.value) && !shown.includes(o))];
  });

  // The profile save rejects the whole set past the cap, so the picker enforces it.
  function toggleSpecialization(value: string) {
    if (specializations.includes(value)) {
      capError = null;
      onSpecializationsChange(specializations.filter((v) => v !== value));
      return;
    }
    if (specializations.length >= MAX_SPECIALIZATIONS) {
      capError = `You can pick at most ${MAX_SPECIALIZATIONS} specializations. Remove one to add another.`;
      return;
    }
    capError = null;
    onSpecializationsChange([...specializations, value]);
  }

  // ---- skills ----

  let skillDist = $state.raw<FacetOption[]>([]);
  let skillDistReady = $state(false);
  $effect(() => {
    void loadSkillDistribution().then((dist) => {
      skillDist = dist;
      skillDistReady = true;
    });
  });

  function searchSkills(q: string): Promise<FacetOption[]> {
    return Promise.resolve(searchSkillOptions(skillDist, q));
  }

  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  // The location suggestions open downward, so the field scrolls to the top of the pane on
  // focus (the pane keeps room under it) — otherwise the footer covers them.
  function revealLocation(e: FocusEvent) {
    const el = e.target as HTMLElement;
    if (el.tagName === 'INPUT' && el.getAttribute('type') !== 'checkbox') {
      el.scrollIntoView({ block: 'start', behavior: 'smooth' });
    }
  }
</script>

<h2 class="text-xl font-semibold tracking-tight text-balance">What are you looking for?</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">
  Pick your specialization, level, skills and where you want to work. Your feed shows only these jobs.
</p>

{#if fromCv}
  <p class="mt-4 flex items-center gap-1.5 rounded-lg bg-brand-muted/50 px-3 py-2 text-sm text-brand-strong">
    <Check class="size-4 shrink-0" aria-hidden="true" /> Filled in from your CV. Check it and change anything.
  </p>
{/if}

<h3 class="mb-2 mt-6 text-sm font-medium">Specialization</h3>
<div class="mb-3 flex items-center gap-2 rounded-lg border border-input px-3 focus-within:ring-2 focus-within:ring-ring">
  <Search class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
  <input
    bind:value={query}
    placeholder="Search specializations…"
    aria-label="Search specializations"
    class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
  />
  {#if query}
    <button
      type="button"
      onclick={() => (query = '')}
      aria-label="Clear search"
      class="grid size-6 shrink-0 place-items-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"
    >
      <X class="size-3.5" />
    </button>
  {/if}
</div>
{#if capError}
  <p class="mb-2 text-xs text-destructive">{capError}</p>
{/if}
<div class="flex flex-wrap gap-2">
  {#each visible as o (o.value)}
    {@const on = specializations.includes(o.value)}
    <button
      type="button"
      onclick={() => toggleSpecialization(o.value)}
      aria-pressed={on}
      class={pillClass(on, false, 'px-3 py-1.5 text-sm')}
    >
      {o.label}{#if counts}<span class={pillCountClass(on)}>{(counts[o.value] ?? 0).toLocaleString()}</span>{/if}
    </button>
  {:else}
    <p class="text-sm text-muted-foreground">No specialization matches “{query}”.</p>
  {/each}
</div>
{#if !query}
  <button
    type="button"
    onclick={() => (showAll = !showAll)}
    class="mt-2 text-sm font-medium text-brand-strong hover:underline"
  >
    {showAll ? 'Show fewer' : 'Show all specializations'}
  </button>
{/if}

<h3 class="mb-2 mt-7 text-sm font-medium">Level</h3>
<SeniorityPills selected={seniorities} onToggle={(v) => onSenioritiesChange(toggle(seniorities, v))} />

<h3 class="mb-2 mt-7 text-sm font-medium">Skills</h3>
<RemoteSearchSelect
  search={searchSkills}
  include={skills}
  placeholder="Search skills"
  label="Search skills"
  onToggle={(v) => onSkillsChange(toggle(skills, v))}
  fallbackLabel={(v) => v}
  clearOnSelect
  closeOnSelect
  ready={skillDistReady}
  techIcons
/>

<div class="scroll-mt-4 pb-56" onfocusin={revealLocation}>
  <h3 class="mb-2 mt-7 text-sm font-medium">Where</h3>
  <LocationPreferencesFields value={location} {derivedLocation} onChange={onLocationChange} />
</div>
