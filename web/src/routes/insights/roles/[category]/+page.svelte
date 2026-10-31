<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '#lib/components/Seo.svelte';
  import InsightsPageShell from '#lib/components/InsightsPageShell.svelte';
  import { breadcrumbJsonLd, datasetJsonLd, jsonLdScript } from '#lib/seo.js';
  import { seniorityLabel } from '#lib/insights.js';
  import { rolePulseFigures } from '#lib/rolePulse.js';
  import { Card, Table } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/insights/roles/${data.category}`);
  // The heading is the page's own name; the title is that plus the brand suffix a
  // browser tab and a SERP need. Kept apart deliberately — one variable serving both
  // is how "· freehire" ended up inside the <h1>.
  const heading = $derived(`Most-Hiring ${data.label} Roles`);
  const title = $derived(`${heading} · freehire`);
  const updated = $derived(
    new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' }),
  );
  const figures = $derived(rolePulseFigures(data.pulse));
  // The direction is stated in words beside its arrow: whether more competition or a
  // longer time to close is good news depends on who is reading, so no colour judges it.
  const MOVES = { up: ['↑', 'up from'], down: ['↓', 'down from'], flat: ['→', 'about the same as'] } as const;
  const jsonLd = $derived(
    jsonLdScript([
      datasetJsonLd(`${data.label} roles by demand`, data.intro, canonical, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Insights', url: `${origin}/insights` },
        { name: `${data.label} Roles`, url: canonical },
      ]),
    ]),
  );
</script>

<Seo {title} description={data.intro} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<InsightsPageShell
  category={data.category}
  label={data.label}
  kind="roles"
  {heading}
  intro={data.intro}
  {updated}
  covered={data.covered}
>
  <section aria-label="Market pulse" class="mb-8">
    <div class="grid gap-4 sm:grid-cols-3">
      {#each figures as f (f.key)}
        <Card class="p-4">
          <p class="text-sm text-muted-foreground">{f.label}</p>
          {#if f.value === null}
            <p class="mt-1 text-sm text-muted-foreground">Not enough data yet.</p>
          {:else}
            <p class="mt-1 text-2xl font-semibold tabular-nums text-foreground">{f.value}</p>
            <p class="mt-1 text-sm text-muted-foreground">{f.detail}</p>
            {#if f.direction && f.previous !== null}
              <p class="mt-1 text-sm tabular-nums text-muted-foreground">
                <span aria-hidden="true">{MOVES[f.direction][0]}</span>
                {MOVES[f.direction][1]} {f.previous} the 30 days before
              </p>
            {/if}
          {/if}
        </Card>
      {/each}
    </div>
    {#if data.pulse.competition.views_through}
      <p class="mt-2 text-xs text-muted-foreground">
        Views counted through {data.pulse.competition.views_through} (UTC); closures that only
        reflect our crawler, such as an unreachable board, are left out of the close time.
      </p>
    {/if}
  </section>

  {#if data.roles.length === 0}
    <p class="text-muted-foreground">No role data for this category yet.</p>
  {:else}
    <Table>
      {#snippet header()}
        <tr class="border-b border-border text-muted-foreground">
          <th class="py-2 pr-4 text-left font-medium">Level</th>
          <th class="py-2 pr-4 font-medium text-right">Open roles</th>
          <th class="py-2 font-medium text-right">30-day growth</th>
        </tr>
      {/snippet}
      {#each data.roles as r (r.seniority)}
        {@const growthTone =
          r.growth > 0
            ? 'text-success-strong'
            : r.growth < 0
              ? 'text-red-600 dark:text-red-400'
              : 'text-muted-foreground'}
        <tr class="border-b border-border">
          <td class="py-2 pr-4 font-medium text-foreground">
            {#if data.leaves.has(r.seniority)}
              <a
                class="text-primary hover:underline"
                href={resolve('/insights/roles/[category]/[seniority]', {
                  category: data.category,
                  seniority: r.seniority,
                })}>{seniorityLabel(r.seniority)}</a
              >
            {:else}
              {seniorityLabel(r.seniority)}
            {/if}
          </td>
          <td class="py-2 pr-4 text-right tabular-nums">{r.open_count.toLocaleString('en-US')}</td>
          <td class="py-2 text-right tabular-nums {growthTone}">
            {r.growth > 0 ? '+' : ''}{r.growth.toLocaleString('en-US')}
          </td>
        </tr>
      {/each}
    </Table>
  {/if}
</InsightsPageShell>
