<script lang="ts">
  import { EXPERIENCE_PRESETS, experienceLabel } from '#lib/filterControls.js';
  import type { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import type { FacetCounts } from '#lib/types.js';
  import ChipFacet from './ChipFacet.svelte';

  // FilterModal's Experience pane: seniority and role type (unless the page `exclude`s
  // them) and the years-of-experience bound with its two modifiers, all on the modal's
  // staged filters (`store`).
  let { store, counts, exclude }: { store: StagedFilters; counts: FacetCounts | null; exclude: string[] } = $props();

  // Same snap-to-Any rule as FilterModal's freshness slider: an off-preset bound (hand-edited or
  // shared URL) has no exact stop, so the handle rests on the rightmost one. The
  // LABEL still reports the real bound — see experienceLabel — so the two do not
  // agree here on purpose: the handle says "no stop matches", the text says what
  // is actually filtering.
  const experienceIndex = $derived.by(() => {
    const i = EXPERIENCE_PRESETS.findIndex((p) => p.years === store.value.experienceYearsMax);
    return i < 0 ? EXPERIENCE_PRESETS.length - 1 : i;
  });
  const showSeniority = $derived(!exclude.includes('seniority'));
  const showRoleType = $derived(!exclude.includes('role_type'));
</script>

{#if showSeniority}
  {@const noLevelPicked = (store.value.facets.seniority?.include.length ?? 0) === 0}
  <ChipFacet {store} param="seniority" label="Seniority" {counts} />
  <!-- Most postings carry no level, and including one drops every one of them
       unless this is ticked. Disabled until a level is included: excluding levels
       already keeps the unlabelled postings, so there is nothing to widen. -->
  <label
    class="mt-3 flex items-center gap-2 text-sm {noLevelPicked ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}"
  >
    <input
      type="checkbox"
      class="size-4 rounded border-border"
      checked={store.value.seniorityIncludeUnspecified}
      disabled={noLevelPicked}
      onchange={(e) => store.setSeniorityIncludeUnspecified(e.currentTarget.checked)}
    />
    <span>Include postings with no level stated</span>
  </label>
{/if}
<!-- Directly beneath seniority on purpose: the two are the axes users conflate.
     "Lead" reads to many as a management grade, while in this catalogue it names
     the IC ladder — of the 116,893 postings at seniority=lead, only 3,303 carry
     any management marker. Adjacency is what makes them read as two questions. -->
{#if showRoleType}
  <div class:mt-6={showSeniority}>
    <ChipFacet {store} param="role_type" label="Role type" {counts} />
  </div>
{/if}
<div class:mt-6={showSeniority || showRoleType}>
  <div class="mb-2 flex items-center justify-between">
    <h3 class="text-sm font-semibold tracking-tight">Years of experience</h3>
    <span class="text-xs font-medium text-muted-foreground">{experienceLabel(store.value.experienceYearsMax)}</span>
  </div>
  <input
    type="range"
    min="0"
    max={EXPERIENCE_PRESETS.length - 1}
    step="1"
    value={experienceIndex}
    oninput={(e) => store.setExperienceYearsMax(EXPERIENCE_PRESETS[Number(e.currentTarget.value)]?.years ?? null)}
    aria-label="Maximum years of experience"
    aria-valuetext={experienceLabel(store.value.experienceYearsMax)}
    class="h-6 w-full accent-primary"
  />
  <!-- Most of the catalogue states no experience requirement at all (about two
       thirds, measured on prod 2026-10-21: 2.42M of 3.67M open postings), and a
       bound excludes every one of those postings unless this is ticked. Shown
       permanently rather than only once bounded: a result count that collapses
       without explanation is read as a broken filter, and by then the user has
       already been misled. Disabled at "Any", where there is no bound to widen. -->
  <label
    class="mt-3 flex items-center gap-2 text-sm {store.value.experienceYearsMax == null
      ? 'cursor-not-allowed opacity-50'
      : 'cursor-pointer'}"
  >
    <input
      type="checkbox"
      class="size-4 rounded border-border"
      checked={store.value.experienceIncludeUnspecified}
      disabled={store.value.experienceYearsMax == null}
      onchange={(e) => store.setExperienceIncludeUnspecified(e.currentTarget.checked)}
    />
    <span>Include postings that don't state experience</span>
  </label>
  <!-- The entry-level profile seeds this (freehire#3655): a posting labelled with a
       picked level is answered by the level, so the limit reads only the postings
       with no level. Shown beside the other modifier so a profile applied to /jobs
       does not narrow the list through a switch nobody can see. -->
  <label
    class="mt-2 flex items-center gap-2 text-sm {store.value.experienceYearsMax == null
      ? 'cursor-not-allowed opacity-50'
      : 'cursor-pointer'}"
  >
    <input
      type="checkbox"
      class="size-4 rounded border-border"
      checked={store.value.experienceOnlySeniorityUnspecified}
      disabled={store.value.experienceYearsMax == null}
      onchange={(e) => store.setExperienceOnlySeniorityUnspecified(e.currentTarget.checked)}
    />
    <span>Apply the limit only to postings with no level stated</span>
  </label>
  <p class="mt-1 text-xs text-muted-foreground">
    Most postings don't state an experience requirement. A limit hides them unless this is ticked.
  </p>
</div>
