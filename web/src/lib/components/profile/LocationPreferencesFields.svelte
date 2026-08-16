<script lang="ts">
  // The "where & how I want to work" fields, shared by the set-up form (ProfileForm, where
  // they're part of one batched Save) and the Location view (where each change autosaves
  // immediately, the same way the Skills view's chips do). This component owns no
  // persistence itself — every edit recomputes the whole block and hands it to `onChange`;
  // the caller decides whether that means "buffer until Save" or "write it now".
  import {
    COUNTRY_OPTIONS,
    countryLabel,
    REGION_OPTIONS,
    searchCities,
    WORK_MODE_OPTIONS,
  } from '$lib/facets';
  import { api } from '$lib/api';
  import { buildLocationPreferences } from '$lib/profileLocation';
  import type { DerivedLocation, LocationPreferences } from '$lib/types';
  import { Input } from '$lib/ui';
  import RemoteSearchSelect from '../facets/RemoteSearchSelect.svelte';
  import SearchSelect from '../facets/SearchSelect.svelte';

  let {
    value,
    derivedLocation = null,
    onChange,
  }: {
    value: LocationPreferences | null;
    derivedLocation?: DerivedLocation | null;
    onChange: (next: LocationPreferences | null) => void;
  } = $props();

  // Seeded once from the incoming value — the caller keys this component on profile
  // identity (or mounts it fresh inside a popover), so a genuinely different value means a
  // remount, not a live reseed that would clobber an in-progress edit.
  // svelte-ignore state_referenced_locally
  let workModes = $state.raw<string[]>(value?.work_modes ?? []);
  // Optional chaining past `value?.`: the block is stored whole as JSONB and echoed back
  // verbatim (see internal/userprofile/location.go), so a row written before a sub-object
  // existed in the schema can still lack it entirely.
  // svelte-ignore state_referenced_locally
  let remoteRegions = $state.raw<string[]>(value?.remote?.regions ?? []);
  // svelte-ignore state_referenced_locally
  let remoteCountries = $state.raw<string[]>(value?.remote?.countries ?? []);
  // Where the user IS. Seeded from what they stated, falling back to what their CV was read
  // to say — so someone who has uploaded a CV confirms a fact rather than retyping it. The
  // derivation only ever fills an UNSTATED field: a saved base always wins, and an ambiguous
  // derivation (more than one country) offers nothing rather than guessing.
  // svelte-ignore state_referenced_locally
  const derived0 = derivedLocation;
  const derivedCountry = (derived0?.countries.length === 1 ? derived0.countries[0] : '') ?? '';
  const derivedCity = (derived0?.cities.length === 1 ? derived0.cities[0] : '') ?? '';
  // svelte-ignore state_referenced_locally
  let baseCountry = $state<string>(value?.base?.country ?? derivedCountry);
  // svelte-ignore state_referenced_locally
  let baseCity = $state<string>(value?.base?.city ?? derivedCity);
  // svelte-ignore state_referenced_locally
  let relocOpen = $state<boolean>(value?.relocation?.open ?? false);
  // svelte-ignore state_referenced_locally
  let relocRegions = $state.raw<string[]>(value?.relocation?.regions ?? []);
  // svelte-ignore state_referenced_locally
  let relocCountries = $state.raw<string[]>(value?.relocation?.countries ?? []);
  // svelte-ignore state_referenced_locally
  let relocCities = $state.raw<string[]>(value?.relocation?.cities ?? []);

  // "Where you're based": one combined search field (city + country in one pick, like a
  // maps-style autocomplete) instead of a country dropdown beside a separate city search
  // — picking a result sets both at once. Mirrors CompanyPicker.svelte's typeahead pattern.
  // svelte-ignore state_referenced_locally
  let baseQuery = $state(baseCity && baseCountry ? `${baseCity}, ${countryLabel(baseCountry)}` : baseCity);
  let baseResults = $state.raw<{ value: string; country: string; label: string }[]>([]);
  let baseOpen = $state(false);
  let baseLoading = $state(false);
  let baseReqToken = 0;
  let baseTimer: ReturnType<typeof setTimeout> | undefined;

  function onBaseInput() {
    // Typing invalidates whatever was picked before — half a city/country pair is
    // worse than none, so both clear together until a fresh pick lands.
    if (baseCity || baseCountry) {
      baseCity = '';
      baseCountry = '';
      emit();
    }
    baseOpen = true;
    clearTimeout(baseTimer);
    const q = baseQuery.trim();
    if (q.length < 2) {
      baseReqToken++;
      baseResults = [];
      baseLoading = false;
      return;
    }
    baseLoading = true;
    const mine = ++baseReqToken;
    baseTimer = setTimeout(async () => {
      try {
        const rows = await api.searchCities(q);
        if (mine !== baseReqToken) return;
        baseResults = rows.map((r) => ({ value: r.value, country: r.country, label: `${r.value}, ${countryLabel(r.country)}` }));
      } catch {
        if (mine !== baseReqToken) return;
        baseResults = [];
      } finally {
        if (mine === baseReqToken) baseLoading = false;
      }
    }, 250);
  }

  function pickBase(row: { value: string; country: string; label: string }) {
    baseCity = row.value;
    baseCountry = row.country;
    baseQuery = row.label;
    baseOpen = false;
    baseResults = [];
    emit();
  }

  // Work format gates the two "where would you take work" sub-forms: remote reach shows
  // only when Remote is accepted, relocation only for On-site/Hybrid. Hidden fields linger
  // in local state (re-selecting the format restores the draft) but are not saved —
  // buildLocationPreferences reads the same gates.
  //
  // The physical BASE is deliberately not among them — see buildLocationPreferences's doc
  // comment for why.
  const wantsRemote = $derived(workModes.includes('remote'));
  const wantsPhysical = $derived(workModes.includes('onsite') || workModes.includes('hybrid'));

  function toggleIn(list: string[], v: string): string[] {
    return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
  }

  function pillCls(active: boolean): string {
    return active
      ? 'rounded-full bg-brand px-3 py-1 text-sm font-medium text-brand-foreground'
      : 'rounded-full border border-border px-3 py-1 text-sm transition-colors hover:border-brand/60';
  }

  function emit() {
    onChange(
      buildLocationPreferences({
        workModes,
        remoteRegions,
        remoteCountries,
        baseCountry,
        baseCity,
        relocOpen,
        relocRegions,
        relocCountries,
        relocCities,
      }),
    );
  }
</script>

{#snippet regionPills(selected: string[], onToggle: (v: string) => void)}
  <div class="flex flex-wrap gap-1.5">
    {#each REGION_OPTIONS as opt (opt.value)}
      <button type="button" onclick={() => onToggle(opt.value)} class={pillCls(selected.includes(opt.value))}>
        {opt.label}
      </button>
    {/each}
  </div>
{/snippet}

{#snippet geoReach(
  regions: string[],
  onRegion: (v: string) => void,
  countries: string[],
  onCountry: (v: string) => void,
)}
  {@render regionPills(regions, onRegion)}
  <SearchSelect
    options={COUNTRY_OPTIONS}
    include={countries}
    placeholder="Add specific countries"
    onToggle={onCountry}
    cap={8}
    clearOnSelect
  />
{/snippet}

<div class="flex flex-col gap-4">
  <span class="text-xs text-muted-foreground">All optional — used to tailor your job filters.</span>

  <!-- Work format -->
  <div class="flex flex-col gap-1.5">
    <span class="text-xs font-medium text-muted-foreground">Work format</span>
    <div class="flex flex-wrap gap-1.5">
      {#each WORK_MODE_OPTIONS as opt (opt.value)}
        <button
          type="button"
          onclick={() => {
            workModes = toggleIn(workModes, opt.value);
            emit();
          }}
          class={pillCls(workModes.includes(opt.value))}
        >
          {opt.label}
        </button>
      {/each}
    </div>
  </div>

  <!-- Where you are now. Asked of EVERY user, whatever work formats they accept: it is a
       fact about the person, not a preference, and it is what the visa-sponsorship and
       onsite-country checks compare a job against. One combined field — city and country
       in a single pick — instead of a country dropdown beside a separate city search. -->
  <div class="flex flex-col gap-1.5">
    <span class="text-xs font-medium text-muted-foreground">Where you're based</span>
    <div class="relative">
      <Input
        bind:value={baseQuery}
        oninput={onBaseInput}
        onfocus={() => baseQuery.trim().length >= 2 && (baseOpen = true)}
        onblur={() => setTimeout(() => (baseOpen = false), 120)}
        placeholder="City or country"
        autocomplete="off"
        role="combobox"
        aria-expanded={baseOpen}
        aria-controls="base-location-list"
        class="w-full"
      />
      {#if baseOpen && (baseResults.length > 0 || baseLoading)}
        <ul
          id="base-location-list"
          role="listbox"
          class="absolute inset-x-0 top-full z-10 mt-1 max-h-60 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-lg"
        >
          {#if baseLoading && baseResults.length === 0}
            <li class="px-2 py-1.5 text-sm text-muted-foreground">Searching…</li>
          {/if}
          {#each baseResults as row (row.value + row.country)}
            <li>
              <button
                type="button"
                role="option"
                aria-selected="false"
                onmousedown={(e) => {
                  e.preventDefault();
                  pickBase(row);
                }}
                class="flex w-full items-center rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent"
              >
                {row.label}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  {#if workModes.length === 0}
    <span class="text-xs text-muted-foreground">Pick a work format above to set where you can work.</span>
  {/if}

  <!-- Remote reach — only relevant once Remote is accepted. -->
  {#if wantsRemote}
    <div class="flex flex-col gap-1.5">
      <span class="text-xs font-medium text-muted-foreground">Remote — regions you can work for (empty = worldwide)</span>
      {@render geoReach(
        remoteRegions,
        (v) => {
          remoteRegions = toggleIn(remoteRegions, v);
          emit();
        },
        remoteCountries,
        (v) => {
          remoteCountries = toggleIn(remoteCountries, v);
          emit();
        },
      )}
    </div>
  {/if}

  <!-- Relocation — only meaningful for someone who would take physical work. -->
  {#if wantsPhysical}
    <div class="flex flex-col gap-2">
      <label class="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={relocOpen}
          onchange={(e) => {
            relocOpen = e.currentTarget.checked;
            emit();
          }}
          class="size-4 rounded border-input"
        />
        Open to relocation
      </label>
      {#if relocOpen}
        <span class="text-xs font-medium text-muted-foreground">Where you'd relocate (empty = anywhere)</span>
        {@render geoReach(
          relocRegions,
          (v) => {
            relocRegions = toggleIn(relocRegions, v);
            emit();
          },
          relocCountries,
          (v) => {
            relocCountries = toggleIn(relocCountries, v);
            emit();
          },
        )}
        <RemoteSearchSelect
          search={(q) => searchCities(q)}
          include={relocCities}
          onToggle={(v) => {
            relocCities = toggleIn(relocCities, v);
            emit();
          }}
          fallbackLabel={(v) => v}
          placeholder="Add a city"
          clearOnSelect
        />
      {/if}
    </div>
  {/if}
</div>
