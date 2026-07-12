<script lang="ts">
  import { ArrowLeft, ArrowRight, FileUp, LoaderCircle, X } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import {
    FACETS,
    CATEGORY_OPTIONS,
    REGION_UNSPECIFIED,
    dynamicOptions,
    type FacetOption,
  } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import { emptySelection, selectionsToQuery, type OnboardingSelection } from '$lib/onboarding';
  import SearchSelect from '../facets/SearchSelect.svelte';

  // The /jobs onboarding wizard: a skippable overlay that captures coarse feed
  // preferences and emits the equivalent filter query. It owns only its staged
  // selection and step — it knows nothing about the URL, the filter store, or
  // localStorage; the parent (JobsView) applies the query and records lifecycle.
  // Two selection steps; the reconfigured feed after `onComplete` is the payoff.
  let {
    open = false,
    counts = null,
    onComplete,
    onCancel,
  }: {
    open?: boolean;
    /** Live facet distribution — feeds the stack picker's skill suggestions. */
    counts?: FacetCounts | null;
    /** Confirmed: the equivalent filter query string (empty if nothing was picked). */
    onComplete: (query: string) => void;
    /** Dismissed without confirming (Escape / backdrop / skip) — feed unchanged. */
    onCancel: () => void;
  } = $props();

  // Option vocabularies drawn from the same registry the search understands, so a
  // captured preference is always a value the feed can filter on. Regions drop the
  // "not specified" sentinel — onboarding is about choosing a place, not its absence.
  const facetOptions = (param: string): FacetOption[] => FACETS.find((f) => f.param === param)?.options ?? [];
  const specializationOptions = CATEGORY_OPTIONS;
  const seniorityOptions = facetOptions('seniority');
  const workModeOptions = facetOptions('work_mode');
  const regionOptions = facetOptions('regions').filter((o) => o.value !== REGION_UNSPECIFIED);

  // Stack suggestions come from the live skills distribution (the same builder the
  // filter panel's Skills select uses), so the picker only offers skills that exist in
  // the catalogue — busiest first, with job counts; already-picked values stay listed.
  const skillOptions = $derived.by(() => dynamicOptions('skills', counts?.facets?.skills ?? {}, sel.stack));

  const TOTAL_STEPS = 2;
  let step = $state(1);
  let sel = $state<OnboardingSelection>(emptySelection());

  // CV auto-fill: a résumé pre-fills focus/seniority/stack (the fields our dictionaries
  // resolve) in place, so the user reviews them on step 1 rather than being advanced past
  // it. The extract endpoint is authenticated, so an anonymous visitor signs in first; the
  // file is chosen only once signed in.
  let cvState = $state<'idle' | 'parsing' | 'error'>('idle');
  let cvError = $state<string | null>(null);
  let cvNote = $state<string | null>(null);
  let cvInput = $state<HTMLInputElement>();
  // Bumped on each pick and on wizard re-open, so a parse that resolves late (or after
  // the wizard was closed and reopened) can't clobber a fresh selection.
  let cvGen = 0;

  function pickCv() {
    if (!isAuthenticated()) {
      openAuthDialog();
      return;
    }
    cvInput?.click();
  }

  async function onCvFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file after an error
    if (!file) return;
    const gen = ++cvGen;
    cvState = 'parsing';
    cvError = null;
    cvNote = null;
    try {
      const cv = await api.extractResumeProfile(file);
      if (gen !== cvGen) return; // superseded by another pick or a wizard reset
      const resolved = cv.categories.length > 0 || !!cv.seniority || cv.skills.length > 0;
      // Pre-fill the fields the dictionaries resolved and stay on step 1, so the user
      // reviews (and can correct) the focus/seniority the pills now show — we never skip
      // that review by jumping ahead. Focus and stack merge (a CV can add several) without
      // dropping anything already picked.
      sel = {
        ...sel,
        specializations: cv.categories.length
          ? [...new Set([...sel.specializations, ...cv.categories])]
          : sel.specializations,
        seniorities: cv.seniority ? [...new Set([...sel.seniorities, cv.seniority])] : sel.seniorities,
        stack: cv.skills.length ? [...new Set([...sel.stack, ...cv.skills])] : sel.stack,
      };
      cvState = 'idle';
      cvNote = resolved ? 'Filled in what we found — review below.' : 'Couldn’t read details from that CV — pick below.';
    } catch (err) {
      if (gen !== cvGen) return;
      cvState = 'error';
      cvError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
    }
  }

  // Reset staged state each time the wizard opens, so a re-open starts fresh.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) {
      step = 1;
      sel = emptySelection();
      cvState = 'idle';
      cvError = null;
      cvNote = null;
      cvGen++; // invalidate any parse still in flight from a previous open
    }
    wasOpen = open;
  });

  // Work-mode/region stay single-select in the wizard (the full FilterModal covers
  // multi-select afterward): clicking the active value clears it.
  function pickOne(field: 'workMode' | 'region', value: string) {
    sel = { ...sel, [field]: sel[field] === value ? undefined : value };
  }

  // toggleIn adds/removes a value from one of the selection's array fields — the shared
  // primitive behind the multi-select pill groups (Focus, Seniority, Stack).
  function toggleIn(field: 'specializations' | 'seniorities' | 'stack', value: string) {
    const cur = sel[field];
    sel = { ...sel, [field]: cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value] };
  }

  // Focus is multi-select — a person can be several specializations (a CV can pre-fill
  // more than one). Seniority is multi-select too — a search can span a grade range.
  const toggleSpecialization = (value: string) => toggleIn('specializations', value);
  const toggleSeniority = (value: string) => toggleIn('seniorities', value);
  const toggleStack = (value: string) => toggleIn('stack', value);

  function complete() {
    onComplete(selectionsToQuery(sel));
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onCancel();
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

<!-- One single-select pill group; the active value clears on re-click (see pickOne). -->
{#snippet pills(field: 'workMode' | 'region', options: FacetOption[])}
  <div class="flex flex-wrap gap-2">
    {#each options as o (o.value)}
      <button
        type="button"
        onclick={() => pickOne(field, o.value)}
        aria-pressed={sel[field] === o.value}
        class={[
          'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors',
          sel[field] === o.value ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
        ]}
      >
        {o.label}
      </button>
    {/each}
  </div>
{/snippet}

<!-- A multi-select pill group: each pill toggles independently (Focus, Seniority). -->
{#snippet multiPills(selected: string[], options: FacetOption[], toggle: (value: string) => void)}
  <div class="flex flex-wrap gap-2">
    {#each options as o (o.value)}
      <button
        type="button"
        onclick={() => toggle(o.value)}
        aria-pressed={selected.includes(o.value)}
        class={[
          'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors',
          selected.includes(o.value) ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
        ]}
      >
        {o.label}
      </button>
    {/each}
  </div>
{/snippet}

{#if open}
  <div class="fixed inset-0 z-50 flex items-stretch justify-center sm:items-center sm:p-6">
    <button class="absolute inset-0 bg-foreground/35" aria-label="Close" onclick={onCancel}></button>

    <div
      class="relative flex h-full w-full flex-col overflow-hidden bg-background shadow-2xl sm:h-auto sm:max-h-[calc(100vh-3rem)] sm:max-w-lg sm:rounded-2xl sm:border sm:border-border"
      role="dialog"
      aria-modal="true"
      aria-label="Set up your feed"
    >
      <!-- header: step dots + skip -->
      <div class="flex items-center gap-3 border-b border-border px-5 py-3">
        <div class="flex flex-1 items-center gap-1.5" aria-hidden="true">
          {#each { length: TOTAL_STEPS } as _, i (i)}
            <span class={['h-1 w-7 rounded-full transition-colors', i < step ? 'bg-brand' : 'bg-border']}></span>
          {/each}
        </div>
        <button
          type="button"
          onclick={onCancel}
          class="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Skip
        </button>
        <button
          type="button"
          onclick={onCancel}
          aria-label="Close"
          class="flex size-8 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <!-- body -->
      <div class="min-h-0 flex-1 overflow-y-auto px-5 py-5">
        <div class="mb-1 inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-brand-strong">
          Step {step} of {TOTAL_STEPS}
        </div>
        {#if step === 1}
          <h2 class="text-xl font-semibold tracking-tight">What do you do?</h2>
          <p class="mt-1 text-sm text-muted-foreground">Pick your focus and level — everything's optional.</p>

          <!-- CV auto-fill: pre-fills focus/seniority/stack, then jumps to review. -->
          <input
            type="file"
            accept=".pdf,application/pdf"
            bind:this={cvInput}
            onchange={onCvFile}
            class="hidden"
          />
          <button
            type="button"
            onclick={pickCv}
            disabled={cvState === 'parsing'}
            class="mt-4 flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-card px-4 py-3 text-sm font-medium transition-colors hover:border-brand hover:bg-accent disabled:opacity-60"
          >
            {#if cvState === 'parsing'}
              <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Reading your CV…
            {:else}
              <FileUp class="size-4" aria-hidden="true" /> Upload CV — autofill
            {/if}
          </button>
          {#if cvState === 'error'}
            <p class="mt-2 text-xs text-destructive">{cvError} Or fill it in below.</p>
          {:else if cvNote}
            <p class="mt-2 text-xs text-muted-foreground">{cvNote}</p>
          {/if}
          <div class="my-4 flex items-center gap-3 text-xs text-muted-foreground">
            <span class="h-px flex-1 bg-border"></span> or answer 2 questions <span class="h-px flex-1 bg-border"></span>
          </div>

          <p class="mb-2 text-sm font-medium">Focus <span class="font-normal text-muted-foreground">— pick any</span></p>
          {@render multiPills(sel.specializations, specializationOptions, toggleSpecialization)}

          <p class="mb-2 mt-6 text-sm font-medium">Seniority <span class="font-normal text-muted-foreground">— pick any</span></p>
          {@render multiPills(sel.seniorities, seniorityOptions, toggleSeniority)}
        {:else}
          <h2 class="text-xl font-semibold tracking-tight">Where and how?</h2>
          <p class="mt-1 text-sm text-muted-foreground">Work format, region, and stack.</p>

          <p class="mb-2 mt-5 text-sm font-medium">Work format</p>
          {@render pills('workMode', workModeOptions)}

          <p class="mb-2 mt-6 text-sm font-medium">Region</p>
          {@render pills('region', regionOptions)}

          <p class="mb-2 mt-6 text-sm font-medium">Stack <span class="font-normal text-muted-foreground">— optional</span></p>
          <!-- Suggestions come from the live skills facet: type to filter, click to add. -->
          <SearchSelect
            options={skillOptions}
            include={sel.stack}
            onToggle={toggleStack}
            clearOnSelect
            cap={24}
            placeholder="Search skills…"
          />
        {/if}
      </div>

      <!-- footer: back / next|finish -->
      <div class="flex items-center gap-3 border-t border-border px-5 py-3">
        {#if step > 1}
          <button
            type="button"
            onclick={() => (step -= 1)}
            class="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            <ArrowLeft class="size-4" /> Back
          </button>
        {/if}
        <div class="flex-1"></div>
        <button
          type="button"
          onclick={step < TOTAL_STEPS ? () => (step += 1) : complete}
          class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-brand px-6 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90"
        >
          {step < TOTAL_STEPS ? 'Next' : 'Show jobs'} <ArrowRight class="size-4" />
        </button>
      </div>
    </div>
  </div>
{/if}
