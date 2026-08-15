<script lang="ts">
  import { Search } from '@lucide/svelte';
  import { api, type SkillPulse } from '$lib/api';
  import { skillLabel } from '$lib/facets';
  import { buildSparkline } from '$lib/skillPulseSparkline';
  import { trendDotClass } from '$lib/skillPulseFormat';
  import { Card, Button } from '$lib/ui';
  import { resolve } from '$app/paths';
  import States from './States.svelte';
  import SkillDeltaBadge from './SkillDeltaBadge.svelte';

  // The signed-in caller's own skill-demand trend: one card per profile skill that
  // has at least one retained weekly snapshot (GET /me/market-pulse already omits
  // skills with none — see internal/handler/me_market_pulse.go). An empty result is
  // ambiguous between "no skills on the profile" and "skills too new to have
  // history yet" — both read the same way here: point at the profile, not an error.
  let data = $state<SkillPulse[]>([]);
  let status = $state<'loading' | 'error' | 'ready'>('loading');

  // A profile can hold up to 200 skills (see userprofile.maxSkills), so once a
  // caller has more than a handful of cards, finding one by scrolling stops
  // scaling — a plain substring filter over the skill name. Matched against both
  // the slug and the display label, so typing what is on the card works and so does
  // typing the slug ("nodejs" finds the "Node.js" card).
  let query = $state('');
  // Kept as typed for the "no match" message, lowercased only for the comparison.
  const trimmedQuery = $derived(query.trim());
  const matchesQuery = (s: SkillPulse) => {
    const needle = trimmedQuery.toLowerCase();
    return (
      s.skill.toLowerCase().includes(needle) || skillLabel(s.skill).toLowerCase().includes(needle)
    );
  };
  const filtered = $derived(trimmedQuery ? data.filter(matchesQuery) : data);

  $effect(() => {
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
</script>

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
  <div class="flex flex-col gap-6">
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
          <a href={resolve('/my/market-pulse/[skill]', { skill: skill.skill })}>
            <Card class="flex flex-col gap-3 p-4 transition-colors hover:border-brand/50 hover:bg-accent/40">
              <div class="flex items-start justify-between gap-2">
                <span class="text-sm font-medium">{skillLabel(skill.skill)}</span>
                <SkillDeltaBadge pct={skill.change_pct} />
              </div>
              <div class="flex items-baseline gap-1.5">
                <span class="text-2xl font-semibold tabular-nums">{skill.open_count}</span>
                <span class="text-xs text-muted-foreground">open roles</span>
              </div>
              <svg
                viewBox="0 0 {model.width} {model.height}"
                class="h-8 w-full"
                role="img"
                aria-label="{skillLabel(skill.skill)} demand over the retained history"
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
                <circle cx={model.lastX} cy={model.lastY} r="2.5" class={trendDotClass(rounded)} />
              </svg>
            </Card>
          </a>
        {/each}
      </div>
    {/if}
  </div>
{/if}
