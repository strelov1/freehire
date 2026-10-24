<script lang="ts">
  import { Chip } from '#lib/ui/index.js';
  import { dynamicLabel } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { COUNTRY_CENTROIDS, type MapPlace } from '#lib/worldMap.js';
  import BubbleMap from '../map/BubbleMap.svelte';
  import { messages } from './JobsWorldMap.messages';

  // Where a plan's jobs are: the shared bubble map, read-only, and the busiest countries
  // named underneath. Its host loads it lazily on the Results step, so the land outline
  // in worldMap.ts never rides in the filter modal's bundle. (The /jobs map dialog is the
  // other host of the same BubbleMap, behind its own lazy boundary.)
  //
  // `countries` arrives already limited to what the plan asked for and sorted busiest
  // first (limitToRequestedCountries) — drawing the facet raw would put Mexico beside a
  // search for Brazil.
  let { countries }: { countries: [string, number][] } = $props();

  const loc = $derived(locale() ?? 'en');
  const s = $derived(t(messages, loc));
  const fmt = (n: number) => n.toLocaleString(loc);
  const countryLabel = (code: string) => dynamicLabel('countries', code);

  const places = $derived(
    countries.flatMap(([code, count]): MapPlace[] => {
      const at = COUNTRY_CENTROIDS[code];
      return at ? [{ key: code, label: countryLabel(code), count, lat: at[0], lng: at[1] }] : [];
    }),
  );
</script>

<div class="mx-auto mt-3 max-w-2xl">
  <BubbleMap {places} label={s.mapAria} />
</div>
<div class="mt-3 flex flex-wrap gap-2">
  {#each countries.slice(0, 6) as [code, count] (code)}
    <Chip>{countryLabel(code)} · {fmt(count)}</Chip>
  {/each}
</div>
