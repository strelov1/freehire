<script lang="ts">
  import { Globe, RotateCcw } from '@lucide/svelte';
  import { Button, Dialog } from '#lib/ui/index.js';
  import { api } from '#lib/api.js';
  import { dynamicLabel } from '#lib/facets.js';
  import { loadCityTable } from '#lib/geoCities.js';
  import { WORLDWIDE_REGION } from '#lib/geoScope.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import { COUNTRY_CENTROIDS, placeCities, type CityTable, type MapPlace } from '#lib/worldMap.js';
  import type { FilterStore } from '#lib/filters.js';
  import type { FacetCounts } from '#lib/types.js';
  import BubbleMap from './BubbleMap.svelte';
  import { messages } from './JobsMapDialog.messages';

  // The /jobs list's current search on a world map, in a dialog over the list. It reads
  // and writes the list's OWN filter store, so a click on the map narrows the list behind
  // it — there is no second copy of the filters to reconcile when the dialog closes.
  //
  // Counts are the dialog's own (the facets endpoint for the store's live params), not
  // the list's total: the list reloads on its own schedule, and a "Show N jobs" read off
  // it would be one click behind.
  //
  // The server ORs the location facets together (countries=br&cities=Campinas is all of
  // Brazil), so a city click REPLACES the country rather than narrowing within it — and
  // the dialog remembers which country it drilled into, to keep drawing that country's
  // cities while only a city is selected.
  let { open = $bindable(false), store }: { open?: boolean; store: FilterStore } = $props();

  const FACETS = ['countries', 'cities', 'regions'];
  // Long enough to fold the two writes of a country click (clear, then add) and a burst
  // of clicks into one request; short enough that the map answers a click at once.
  const DEBOUNCE_MS = 150;

  const loc = $derived(locale() ?? 'en');
  const s = $derived(t(messages, loc));
  const fmt = (n: number) => n.toLocaleString(loc);

  // The country a city click drilled into; null once the view is a country view again.
  let drilled = $state<string | null>(null);

  const countries = $derived(store.value.facets.countries?.include ?? []);
  const cities = $derived(store.value.facets.cities?.include ?? []);
  const worldwideOn = $derived((store.value.facets.regions?.include ?? []).includes(WORLDWIDE_REGION));
  // A city set elsewhere (the filter modal) must not inherit a country drilled long ago.
  $effect(() => {
    if (cities.length === 0) drilled = null;
  });
  const focusCountry = $derived(
    countries.length === 1 ? countries[0] : countries.length === 0 && cities.length > 0 ? drilled : null,
  );

  const byCount = (facet: Record<string, number> | undefined) =>
    Object.entries(facet ?? {}).sort((a, b) => b[1] - a[1]);

  let counts = $state.raw<FacetCounts | null>(null);
  let loading = $state(false);
  let failed = $state(false);
  // Every request takes the next number; only the latest may land. Without it a slow
  // answer for the previous click arrives last and repaints the map, and the button,
  // with filters that are no longer selected.
  let seq = 0;

  // The cities of the last single-country answer. Once a city replaces its country the
  // facet only holds the selected cities (and their co-listed neighbours), so the city
  // layer is drawn from this instead: each city's count within the country is unchanged
  // by which city is selected.
  let cityLayer = $state.raw<{ country: string; cities: [string, number][] } | null>(null);

  $effect(() => {
    if (!open) return;
    const params = store.params;
    const single = countries.length === 1 ? countries[0] : null;
    const mine = ++seq;
    loading = true;
    const timer = setTimeout(async () => {
      try {
        const res = await api.facetCounts(params, { facets: FACETS });
        if (mine !== seq) return;
        counts = res;
        failed = false;
        if (single) cityLayer = { country: single, cities: byCount(res.facets.cities) };
      } catch {
        if (mine !== seq) return;
        failed = true;
      } finally {
        if (mine === seq) loading = false;
      }
    }, DEBOUNCE_MS);
    return () => clearTimeout(timer);
  });

  // The city table is ~200 KB: asked for only once a single country is in focus, the one
  // state that draws cities. `null` once it has failed — the map then stays on countries.
  let cityTable = $state.raw<CityTable | null | undefined>(undefined);
  $effect(() => {
    if (!open || !focusCountry || cityTable !== undefined) return;
    void loadCityTable().then((table) => (cityTable = table));
  });

  // A multi-country posting also counts toward countries nobody selected; drawing those
  // beside the selection would put Mexico on a search for Brazil.
  const countryPlaces = $derived(
    byCount(counts?.facets.countries)
      .filter(([code]) => countries.length === 0 || countries.includes(code))
      .flatMap(([code, count]): MapPlace[] => {
        const at = COUNTRY_CENTROIDS[code];
        return at ? [{ key: `country:${code}`, label: dynamicLabel('countries', code), count, lat: at[0], lng: at[1] }] : [];
      }),
  );

  const cityPlaces = $derived(
    focusCountry && cityTable && cityLayer?.country === focusCountry
      ? placeCities(focusCountry, cityLayer.cities, cityTable).map(
          (c): MapPlace => ({ key: `city:${c.name}`, label: c.name, count: c.count, lat: c.lat, lng: c.lng }),
        )
      : [],
  );

  const places = $derived(cityPlaces.length ? cityPlaces : countryPlaces);
  const total = $derived(counts?.total ?? null);
  const worldwide = $derived(counts?.facets.regions?.[WORLDWIDE_REGION] ?? 0);
  const countryTotal = $derived((focusCountry && counts?.facets.countries?.[focusCountry]) || total || 0);
  // A posting listing two cities counts in both, so the sum can pass the country's total.
  const placed = $derived(Math.min(countryTotal, cityPlaces.reduce((n, c) => n + c.count, 0)));

  const maxCount = $derived(Math.max(1, ...places.slice(0, 10).map((p) => p.count)));
  // The dialog's subtitle: how much is on the map, for the place in focus when there is one.
  // Hidden while the map catches up with a click, like the button's count: the old figure
  // would be labelled with the newly selected place.
  const summary = $derived.by(() => {
    if (total === null || loading || failed) return undefined;
    if (focusCountry) {
      return format(plural(loc, total, s.summaryIn), { count: fmt(total), country: dynamicLabel('countries', focusCountry) });
    }
    const n = countryPlaces.length;
    const places = format(plural(loc, n, s.countries), { count: fmt(n) });
    return format(plural(loc, total, s.summary), { count: fmt(total), places });
  });

  function pick(place: MapPlace) {
    // `kind:value`; a city name may itself hold a colon, so split at the first one only.
    const sep = place.key.indexOf(':');
    const value = place.key.slice(sep + 1);
    // Location filters OR together on the server, so every click REPLACES the places —
    // worldwide included — rather than adding one beside them, which would widen the list.
    store.remove('regions', WORLDWIDE_REGION);
    if (place.key.slice(0, sep) === 'city') {
      drilled = focusCountry ?? null;
      store.clearFacet('countries');
      store.clearFacet('cities');
      store.add('cities', value);
      return;
    }
    const wasOnlySelection = countries.length === 1 && countries[0] === value;
    worldView();
    if (!wasOnlySelection) store.add('countries', value);
  }

  function worldView() {
    drilled = null;
    store.clearFacet('countries');
    store.clearFacet('cities');
  }

  // "Anywhere" ORs with the places like any location facet, so leaving them set would
  // widen the list to both; turning worldwide on drops them instead.
  function toggleWorldwide() {
    if (worldwideOn) {
      store.remove('regions', WORLDWIDE_REGION);
      return;
    }
    store.add('regions', WORLDWIDE_REGION);
    worldView();
  }
</script>

<Dialog bind:open title={s.title} description={summary} class="sm:max-w-6xl">
  {#if open}
    <!-- Top to bottom: the controls that change what is counted, the map, the busiest
         places as a list (each a filter), and the way back to the list pinned to the
         bottom edge so it never scrolls out of reach. On a phone the list is what fills
         the sheet; centring a short map in it left the screen mostly empty. -->
    <div class="flex flex-col gap-4">
      <div class="flex flex-wrap items-center gap-2">
        {#if countries.length || cities.length}
          <Button variant="outline" size="sm" onclick={worldView}>
            <RotateCcw class="size-3.5" />
            {s.worldView}
          </Button>
        {/if}
        {#if worldwide > 0 || worldwideOn}
          <Button
            variant={worldwideOn ? 'primary' : 'outline'}
            size="sm"
            aria-pressed={worldwideOn}
            onclick={toggleWorldwide}
          >
            <Globe class="size-3.5" />
            {format(s.remoteWorldwide, { count: fmt(worldwide) })}
          </Button>
        {/if}
        <span class="ml-auto text-xs text-muted-foreground" aria-live="polite">
          {#if loading}
            {s.updating}
          {:else if failed}
            {s.loadFailed}
          {:else if focusCountry && cityTable === null}
            {s.citiesFailed}
          {:else if cityPlaces.length && countries.length === 1}
            {format(s.placedShare, { placed: fmt(placed), total: fmt(countryTotal) })}
          {/if}
        </span>
      </div>

      <BubbleMap {places} label={s.mapAria} onpick={pick} busy={loading} class="sm:max-h-96" />

      <!-- The busiest places as rows with their share drawn as a bar, built from what the
           map draws, so a city list holds only cities in the focus country. Two columns
           where there is room. -->
      {#if places.length}
        <ul class="grid gap-x-6 sm:grid-cols-2" aria-label={s.busiestPlaces}>
          {#each places.slice(0, 10) as place (place.key)}
            <li class="border-b border-border last:border-b-0 sm:[&:nth-last-child(2)]:border-b-0">
              <button
                type="button"
                class="flex w-full items-center gap-3 py-2.5 text-left text-sm transition-colors hover:text-brand-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label={`${place.label} · ${fmt(place.count)}`}
                onclick={() => pick(place)}
              >
                <span class="min-w-0 flex-1 truncate">{place.label}</span>
                <span class="h-1.5 w-16 shrink-0 overflow-hidden rounded-full bg-muted" aria-hidden="true">
                  <span class="block h-full rounded-full bg-brand" style:width="{(place.count / maxCount) * 100}%"></span>
                </span>
                <span class="w-14 shrink-0 text-right tabular-nums text-muted-foreground">{fmt(place.count)}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}

      <div class="sticky bottom-0 -mx-6 -mb-6 mt-2 border-t border-border bg-card px-6 py-4 pb-safe sm:flex sm:justify-end">
        <Button variant="cta" size="lg" class="w-full sm:w-auto" onclick={() => (open = false)}>
          <!-- No figure while the map catches up with a click: the count from before it
               would describe a list that is no longer the one behind the dialog. -->
          {total === null || loading || failed
            ? s.showJobsPending
            : format(plural(loc, total, s.showJobs), { count: fmt(total) })}
        </Button>
      </div>
    </div>
  {/if}
</Dialog>
