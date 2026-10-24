<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import { LAND_PATH, bubbleRadius, fitView, projectToMap, type MapPlace } from '#lib/worldMap.js';
  import { messages } from './BubbleMap.messages';

  // Where jobs are, drawn as one bubble per place over the land outline, the view fitted
  // to what is drawn (worldMap.ts holds the arithmetic). Two hosts: the AI search's
  // read-only map of a plan, and the /jobs map dialog, which passes `onpick` — only then
  // is a bubble a button, reachable from the keyboard, with a tooltip on hover AND on
  // focus, so a keyboard user hears what a mouse user sees. A read-only bubble names
  // itself through the platform's own <title> tooltip: it is not focusable, and a static
  // element carrying pointer handlers is an accessibility smell for nothing gained.
  //
  // `class` sizes the svg; the host decides how much height the map may take.
  let {
    places,
    label,
    onpick,
    busy = false,
    class: className = '',
  }: {
    places: MapPlace[];
    label: string;
    onpick?: (place: MapPlace) => void;
    busy?: boolean;
    class?: string;
  } = $props();

  const loc = $derived(locale() ?? 'en');
  const s = $derived(t(messages, loc));
  const jobs = (n: number) => format(plural(loc, n, s.jobs), { count: n.toLocaleString(loc) });

  // Busiest first, so a small bubble is drawn over the big one it overlaps, not under it.
  const points = $derived(
    [...places].sort((a, b) => b.count - a.count).map((p) => ({ place: p, ...projectToMap(p.lat, p.lng) })),
  );
  const view = $derived(fitView(points));
  const max = $derived(Math.max(1, ...places.map((p) => p.count)));

  let wrap = $state<HTMLDivElement>();
  let tip = $state<{ place: MapPlace; left: number; top: number } | null>(null);

  // Anchored above the bubble rather than at the pointer, so hover and focus put it in
  // the same place.
  function show(place: MapPlace, el: Element) {
    const box = wrap?.getBoundingClientRect();
    const at = el.getBoundingClientRect();
    if (!box) return;
    tip = { place, left: at.left + at.width / 2 - box.left, top: at.top - box.top };
  }

  function hide() {
    tip = null;
  }

  function pick(place: MapPlace) {
    tip = null;
    onpick?.(place);
  }

  function onkeydown(e: KeyboardEvent, place: MapPlace) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    // Space would otherwise scroll whatever contains the map.
    e.preventDefault();
    pick(place);
  }
</script>

<div class="relative" bind:this={wrap}>
  <svg
    viewBox="{view.x} {view.y} {view.w} {view.h}"
    class="mx-auto block w-full transition-opacity {busy ? 'opacity-60' : ''} {className}"
    role={onpick ? 'group' : 'img'}
    aria-label={label}
    aria-busy={busy}
  >
    <path d={LAND_PATH} class="fill-muted stroke-border" stroke-width="0.4" vector-effect="non-scaling-stroke" />
    {#each points as { place, x, y } (place.key)}
      {#if onpick}
        <circle
          cx={x}
          cy={y}
          r={bubbleRadius(place.count, max, view.w)}
          class="cursor-pointer fill-brand/40 stroke-brand transition-colors hover:fill-brand/70 focus-visible:fill-brand/70 focus-visible:stroke-foreground focus-visible:stroke-2 focus-visible:outline-none"
          stroke-width="1"
          vector-effect="non-scaling-stroke"
          role="button"
          tabindex="0"
          aria-label={format(s.placeCount, { place: place.label, jobs: jobs(place.count) })}
          onclick={() => pick(place)}
          onkeydown={(e) => onkeydown(e, place)}
          onpointerenter={(e) => show(place, e.currentTarget)}
          onpointerleave={hide}
          onfocus={(e) => show(place, e.currentTarget)}
          onblur={hide}
        />
      {:else}
        <circle
          cx={x}
          cy={y}
          r={bubbleRadius(place.count, max, view.w)}
          class="fill-brand/40 stroke-brand"
          stroke-width="1"
          vector-effect="non-scaling-stroke"
        >
          <title>{format(s.placeCount, { place: place.label, jobs: jobs(place.count) })}</title>
        </circle>
      {/if}
    {/each}
  </svg>

  {#if tip}
    <div
      role="tooltip"
      class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs text-popover-foreground shadow-md"
      style="left:{tip.left}px; top:{tip.top - 4}px"
    >
      <p class="font-medium">{tip.place.label}</p>
      <p class="text-muted-foreground">
        {jobs(tip.place.count)} · {s.clickToFilter}
      </p>
    </div>
  {/if}
</div>
