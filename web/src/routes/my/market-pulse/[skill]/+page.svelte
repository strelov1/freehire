<script lang="ts">
  import { ArrowLeft } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api, type SkillPulse } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { buildSkillDetailChart } from '$lib/skillDetailChart';
  import { pickTickIndices, formatCount } from '$lib/activityChart';
  import { Button } from '$lib/ui';
  import States from '$lib/components/States.svelte';
  import SkillDeltaBadge from '$lib/components/SkillDeltaBadge.svelte';
  import { must } from '$lib/utils';

  // One skill's full retained history, zoomed in from the /my/market-pulse card
  // grid. Reuses the same GET /me/market-pulse call (already returns every
  // profile skill's full series — no dedicated per-skill endpoint needed) and
  // finds the one matching the route param client-side.
  let data = $state<SkillPulse[]>([]);
  let status = $state<'loading' | 'error' | 'ready'>('loading');

  $effect(() => {
    if (!isAuthenticated()) return;
    status = 'loading';
    api
      .marketPulse()
      .then((d) => {
        data = d;
        status = 'ready';
      })
      .catch(() => {
        status = 'error';
      });
  });

  const skill = $derived(data.find((s) => s.skill === page.params.skill) ?? null);
  const model = $derived(skill ? buildSkillDetailChart(skill.series) : null);
  const ticks = $derived(model ? pickTickIndices(model.points.length) : []);
  // Points are evenly spaced from the first point's x — reused as the left/right
  // plot padding rather than re-deriving the chart module's PAD constant here.
  const padX = $derived(model && model.points.length > 0 ? must(model.points[0]).x : 0);

  let hovered = $state<number | null>(null);
  let tipX = $state(0);
  let tipY = $state(0);
  const hoveredPoint = $derived(hovered === null || !model ? null : (model.points[hovered] ?? null));

  // Nearest-point-by-x rather than a slot-index formula: the series is short
  // (≤26 points) so an O(n) scan is trivial, and it stays correct regardless
  // of how the chart module spaces points.
  function onMove(e: PointerEvent) {
    if (!model || model.points.length === 0) return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (rect.width === 0) return;
    const vbX = ((e.clientX - rect.left) / rect.width) * model.width;
    let nearest = 0;
    let nearestDist = Infinity;
    model.points.forEach((p, i) => {
      const d = Math.abs(p.x - vbX);
      if (d < nearestDist) {
        nearestDist = d;
        nearest = i;
      }
    });
    hovered = nearest;
    tipX = e.clientX - rect.left;
    tipY = e.clientY - rect.top;
  }

  /** Short axis label, e.g. "Jun 1". */
  function shortDate(weekStart: string): string {
    return new Date(weekStart + 'T00:00:00Z').toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      timeZone: 'UTC',
    });
  }

  /** Full tooltip date, e.g. "1 Jun 2026". */
  function fullDate(weekStart: string): string {
    return new Date(weekStart + 'T00:00:00Z').toLocaleDateString(undefined, {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      timeZone: 'UTC',
    });
  }
</script>

<svelte:head>
  <title>{page.params.skill} · Market pulse — freehire</title>
</svelte:head>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to view your market pulse.</p>
{:else}
  <div class="flex flex-col gap-4">
    <a
      href={resolve('/my/market-pulse')}
      class="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft class="size-4" /> Market pulse
    </a>

    {#if status === 'loading'}
      <States state="loading" rows={1} />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load this skill's trend." />
    {:else if !skill || !model}
      <div class="flex flex-col items-center gap-3 py-16 text-center">
        <p class="text-sm font-medium text-foreground">No trend for "{page.params.skill}"</p>
        <p class="max-w-sm text-sm text-muted-foreground">
          Either it isn't one of your profile skills, or it hasn't shown up in an open role yet.
        </p>
        <Button variant="primary" href={resolve('/my/market-pulse')}>Back to market pulse</Button>
      </div>
    {:else}
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight">{skill.skill}</h1>
          <div class="mt-1 flex items-baseline gap-2">
            <span class="text-3xl font-semibold tabular-nums">{skill.open_count}</span>
            <span class="text-sm text-muted-foreground">open roles</span>
          </div>
        </div>
        <SkillDeltaBadge pct={skill.change_pct} />
      </div>

      <div
        class="relative rounded-lg border border-border p-4"
        role="img"
        aria-label="{skill.skill} demand over the retained history"
        onpointermove={onMove}
        onpointerleave={() => (hovered = null)}
      >
        <svg viewBox="0 0 {model.width} {model.height}" class="w-full">
          <!-- y-axis max reference line + label -->
          <line
            x1={padX}
            y1={model.topY}
            x2={model.width - padX}
            y2={model.topY}
            class="stroke-border"
            stroke-dasharray="2 3"
          />
          <text x={padX} y={model.topY - 4} class="fill-muted-foreground" font-size="11">
            {formatCount(model.max)}
          </text>

          {#if model.points.length > 1}
            <polyline
              points={model.points.map((p) => `${p.x},${p.y}`).join(' ')}
              fill="none"
              class="stroke-brand"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          {/if}
          {#each model.points as p, i (p.weekStart)}
            <circle cx={p.x} cy={p.y} r={hovered === i ? 4 : 2.5} class="fill-brand transition-[r]" />
          {/each}

          <!-- baseline -->
          <line
            x1="0"
            y1={model.baselineY}
            x2={model.width}
            y2={model.baselineY}
            class="stroke-border"
            stroke-width="1"
          />

          <!-- x-axis date labels (thinned for a long series) -->
          {#each ticks as i (i)}
            {@const p = model.points[i]}
            {#if p}
              <text
                x={p.x}
                y={model.baselineY + 16}
                text-anchor="middle"
                class="fill-muted-foreground"
                font-size="11"
              >
                {shortDate(p.weekStart)}
              </text>
            {/if}
          {/each}
        </svg>

        {#if hoveredPoint}
          <div
            class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md"
            style="left: {tipX}px; top: {tipY - 8}px;"
          >
            <div class="font-medium text-foreground">{fullDate(hoveredPoint.weekStart)}</div>
            <div class="text-muted-foreground">
              <span class="font-medium text-foreground">{hoveredPoint.openCount.toLocaleString()}</span> open roles
            </div>
          </div>
        {/if}
      </div>

      {#if model.points.length === 1}
        <p class="text-center text-xs text-muted-foreground">
          Only one snapshot so far — check back next week for a trend line.
        </p>
      {/if}
    {/if}
  </div>
{/if}
