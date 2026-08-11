<script lang="ts">
  import { TrendingUp, TrendingDown, Minus, Search } from '@lucide/svelte';
  import { api, type SkillPulse } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { buildSparkline } from '$lib/skillPulseSparkline';
  import { Card, Button } from '$lib/ui';
  import { resolve } from '$app/paths';
  import States from './States.svelte';

  // The signed-in caller's own skill-demand trend: one card per profile skill that
  // has at least one retained weekly snapshot (GET /me/market-pulse already omits
  // skills with none — see internal/handler/me_market_pulse.go). An empty result is
  // ambiguous between "no skills on the profile" and "skills too new to have
  // history yet" — both read the same way here: point at the profile, not an error.
  let data = $state<SkillPulse[]>([]);
  let status = $state<'loading' | 'error' | 'ready'>('loading');

  // A profile can hold up to 200 skills (see userprofile.maxSkills), so once a
  // caller has more than a handful of cards, finding one by scrolling stops
  // scaling — a plain substring filter over the skill name.
  let query = $state('');
  const trimmedQuery = $derived(query.trim());
  const filtered = $derived(
    trimmedQuery ? data.filter((s) => s.skill.toLowerCase().includes(trimmedQuery.toLowerCase())) : data,
  );

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

  // Sign, then magnitude — a true minus sign (not a hyphen), matching CreditsView's
  // fmtDelta. Rounded to the nearest whole percent; a fractional point is noise here.
  function fmtPct(rounded: number): string {
    return rounded > 0 ? `+${rounded}%` : rounded < 0 ? `−${Math.abs(rounded)}%` : '0%';
  }

  // The sparkline's accent dot uses the same up/good, down/bad semantic as the delta
  // badge below — one place for the sign check both share. Both take the ROUNDED
  // percent, matching what fmtPct prints — branching on the raw value would let a
  // sub-0.5% change pick the up/down color+icon while the badge still reads "0%".
  function dotClass(rounded: number | null): string {
    if (rounded === null || rounded === 0) return 'fill-foreground';
    return rounded > 0 ? 'fill-emerald-500' : 'fill-rose-500';
  }
</script>

{#snippet deltaBadge(pct: number | null)}
  {@const rounded = pct === null ? null : Math.round(pct)}
  {#if rounded === null}
    <span
      class="inline-flex items-center gap-1 rounded-md bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground"
    >
      New
    </span>
  {:else if rounded > 0}
    <span
      class="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-1.5 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400"
    >
      <TrendingUp class="size-3" />{fmtPct(rounded)}
    </span>
  {:else if rounded < 0}
    <span
      class="inline-flex items-center gap-1 rounded-md bg-rose-500/10 px-1.5 py-0.5 text-xs font-medium text-rose-600 dark:text-rose-400"
    >
      <TrendingDown class="size-3" />{fmtPct(rounded)}
    </span>
  {:else}
    <span
      class="inline-flex items-center gap-1 rounded-md bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground"
    >
      <Minus class="size-3" />0%
    </span>
  {/if}
{/snippet}

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to view your market pulse.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Market pulse</h1>
      <p class="text-sm text-muted-foreground">
        How demand for your own profile skills is moving, week over week.
      </p>
    </div>

    {#if status === 'loading'}
      <States state="loading" rows={3} />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your market pulse." />
    {:else if data.length === 0}
      <div class="flex flex-col items-center gap-3 py-12 text-center">
        <p class="text-sm font-medium text-foreground">No skill trend yet</p>
        <p class="max-w-sm text-sm text-muted-foreground">
          Add skills to your profile, or check back in a week — a trend needs at least one skill
          that has shown up in an open role.
        </p>
        <Button variant="primary" href={resolve('/my/profile')}>Go to profile</Button>
      </div>
    {:else}
      <div class="relative max-w-xs">
        <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          type="search"
          placeholder="Find a skill…"
          bind:value={query}
          aria-label="Filter skills"
          class="w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand-ring/40"
        />
      </div>

      {#if filtered.length === 0}
        <p class="py-8 text-center text-sm text-muted-foreground">
          No skill matches "{trimmedQuery}".
        </p>
      {:else}
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {#each filtered as skill (skill.skill)}
            {@const model = buildSparkline(skill.series)}
            {@const rounded = skill.change_pct === null ? null : Math.round(skill.change_pct)}
            <Card class="flex flex-col gap-3 p-4">
              <div class="flex items-start justify-between gap-2">
                <span class="text-sm font-medium">{skill.skill}</span>
                {@render deltaBadge(skill.change_pct)}
              </div>
              <div class="flex items-baseline gap-1.5">
                <span class="text-2xl font-semibold tabular-nums">{skill.open_count}</span>
                <span class="text-xs text-muted-foreground">open roles</span>
              </div>
              <svg
                viewBox="0 0 {model.width} {model.height}"
                class="h-8 w-full"
                role="img"
                aria-label="{skill.skill} demand over the retained history"
              >
                {#if model.points}
                  <polyline
                    points={model.points}
                    fill="none"
                    class="stroke-muted-foreground/50"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                {/if}
                <circle cx={model.lastX} cy={model.lastY} r="2.5" class={dotClass(rounded)} />
              </svg>
            </Card>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
{/if}
