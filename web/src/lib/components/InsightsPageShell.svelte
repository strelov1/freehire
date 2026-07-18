<script lang="ts">
  // Shared chrome for the per-category insights pages (salary / skills / roles):
  // a visible breadcrumb, the H1, the data-driven intro, an "updated" line, the
  // data section (passed as a snippet), and the internal-link rail (the other two
  // insight types for this category, a link into the filtered /jobs feed, and
  // sibling-category chips). Purely presentational — the route loads the data.
  import { resolve } from '$app/paths';
  import type { Snippet } from 'svelte';
  import type { CoveredCategory } from '$lib/insights';

  type Kind = 'salary' | 'skills' | 'roles';

  // resolve() needs a literal route id, so map the dynamic kind explicitly.
  function kindHref(k: Kind, cat: string): string {
    if (k === 'salary') return resolve('/insights/salary/[category]', { category: cat });
    if (k === 'skills') return resolve('/insights/skills/[category]', { category: cat });
    return resolve('/insights/roles/[category]', { category: cat });
  }

  let {
    category,
    label,
    kind,
    title,
    intro,
    updated,
    covered,
    children,
  }: {
    category: string;
    label: string;
    kind: Kind;
    title: string;
    intro: string;
    updated: string;
    covered: CoveredCategory[];
    children: Snippet;
  } = $props();

  const KIND_LABEL: Record<Kind, string> = {
    salary: 'Salaries',
    skills: 'Skills',
    roles: 'Roles',
  };

  // The other two insight types for this same category.
  const otherKinds = $derived(
    (['salary', 'skills', 'roles'] as Kind[]).filter((k) => k !== kind),
  );
  // A few sibling categories to cross-link (excluding the current one).
  const siblings = $derived(covered.filter((c) => c.category !== category).slice(0, 8));
</script>

<article class="mx-auto w-full max-w-4xl px-4 py-8">
  <nav aria-label="Breadcrumb" class="mb-4 text-sm text-muted-foreground">
    <a href={resolve('/')} class="hover:underline">freehire</a>
    <span aria-hidden="true"> › </span>
    <a href={resolve('/insights')} class="hover:underline">Insights</a>
    <span aria-hidden="true"> › </span>
    <span class="text-foreground">{label} {KIND_LABEL[kind]}</span>
  </nav>

  <h1 class="text-3xl font-bold tracking-tight text-foreground">{title}</h1>
  <p class="mt-3 text-lg text-muted-foreground">{intro}</p>
  <p class="mt-1 text-xs text-muted-foreground">Updated {updated}</p>

  <section class="mt-8">
    {@render children()}
  </section>

  <aside class="mt-10 border-t border-border pt-6">
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
      <span class="font-medium text-foreground">More on {label}:</span>
      {#each otherKinds as k (k)}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href is resolve()d inside kindHref; the linter can't trace it through the helper -->
        <a href={kindHref(k, category)} class="text-blue-600 hover:underline">{KIND_LABEL[k]}</a>
      {/each}
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string, no dynamic route to resolve -->
      <a href={`${resolve('/jobs')}?category=${encodeURIComponent(category)}`} class="text-blue-600 hover:underline">
        Browse {label} jobs →
      </a>
    </div>

    {#if siblings.length > 0}
      <div class="mt-4 flex flex-wrap items-center gap-2 text-sm">
        <span class="font-medium text-foreground">Other categories:</span>
        {#each siblings as s (s.category)}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href is resolve()d inside kindHref; the linter can't trace it through the helper -->
          <a href={kindHref(kind, s.category)} class="rounded-full bg-secondary px-3 py-1 text-secondary-foreground hover:bg-accent">
            {s.label}
          </a>
        {/each}
      </div>
    {/if}
  </aside>
</article>
