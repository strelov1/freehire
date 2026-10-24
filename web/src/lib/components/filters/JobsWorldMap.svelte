<script lang="ts">
  import { Chip } from '#lib/ui/index.js';
  import { dynamicLabel } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { COUNTRY_CENTROIDS, LAND_PATH, projectToMap } from '#lib/worldMap.js';
  import { messages } from './JobsWorldMap.messages';

  // Where a plan's jobs are: the land outline, one bubble per country sized by the square
  // root of its count (area, not radius, tracks the number), and the busiest countries
  // named underneath. This is the only importer of worldMap.ts, and its host loads it
  // lazily on the Results step, so the outline never rides in the filter modal's bundle.
  //
  // `countries` arrives already limited to what the plan asked for and sorted busiest
  // first (limitToRequestedCountries) — drawing the facet raw would put Mexico beside a
  // search for Brazil.
  let { countries }: { countries: [string, number][] } = $props();

  const s = $derived(t(messages, locale()));
  const max = $derived(countries[0]?.[1] ?? 1);
  const fmt = (n: number) => n.toLocaleString(locale() ?? 'en');
  const countryLabel = (code: string) => dynamicLabel('countries', code);

  const points = $derived(
    countries.flatMap(([code, count]) => {
      const at = COUNTRY_CENTROIDS[code];
      if (!at) return [];
      const { x, y } = projectToMap(at[0], at[1]);
      return [{ code, count, x, y, r: 2 + 10 * Math.sqrt(count / max) }];
    }),
  );
</script>

<svg viewBox="0 0 360 180" class="mx-auto mt-3 h-auto w-full max-w-2xl" role="img" aria-label={s.mapAria}>
  <path d={LAND_PATH} class="fill-muted" />
  {#each points as p (p.code)}
    <circle cx={p.x} cy={p.y} r={p.r} class="fill-brand/40 stroke-brand" stroke-width="0.3">
      <title>{format(s.countryCount, { country: countryLabel(p.code), count: fmt(p.count) })}</title>
    </circle>
  {/each}
</svg>
<div class="mt-3 flex flex-wrap gap-2">
  {#each countries.slice(0, 6) as [code, count] (code)}
    <Chip>{countryLabel(code)} · {fmt(count)}</Chip>
  {/each}
</div>
