<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import DocsCodeBlock from '$lib/components/DocsCodeBlock.svelte';
  import { BASE_URL, OVERVIEW } from '$lib/docs/api-spec';
  import { FILTER_FACETS, FILTER_EXTRAS, FILTER_MODIFIERS, RECIPES } from '$lib/docs/filters';
  import { NAV, slugify } from '$lib/docs/nav';
  import { METHOD_TEXT, inlineCode } from '$lib/docs/format';

  let { data } = $props();

  const canonical = $derived(`${page.url.origin}/docs/api`);
</script>

<Seo
  title="freehire API reference — query jobs by filters"
  description="The freehire HTTP API: a read-first, open endpoint set over the job catalogue. Search and filter jobs by seniority, skills, region, salary and more, read companies, and track applications with an API key."
  {canonical}
/>

<!-- Header. -->
<header class="mb-14 border-b border-border pb-10">
  <p class="font-mono text-xs uppercase tracking-[0.24em] text-brand-strong">// freehire API</p>
  <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">API reference</h1>
  <p class="mt-5 max-w-2xl text-lg leading-relaxed text-muted-foreground">
    A read-first, open HTTP API over the freehire job catalogue — query jobs by rich filters, read
    companies, and (with a key) track applications.
  </p>
  <div class="mt-6 inline-flex items-center gap-2 rounded-lg border border-border bg-secondary/40 px-3 py-1.5 font-mono text-sm">
    <span class="text-muted-foreground">Base URL</span>
    <span class="text-foreground">{BASE_URL}</span>
  </div>
</header>

<!-- Overview concept sections (scroll-spy targets for the nav). -->
{#each OVERVIEW as section, i (section.title)}
  <section id={slugify(section.title)} data-spy class="mb-12 scroll-mt-24">
    <h2 class="text-xl font-semibold tracking-tight">{section.title}</h2>
    {#each section.paragraphs as p (p)}
      <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">{@html inlineCode(p)}</p>
    {/each}
    {#if section.code}
      <div class="mt-4 max-w-2xl">
        <DocsCodeBlock code={section.code} label="json" html={data.overviewHtml[i] ?? undefined} />
      </div>
    {/if}
  </section>
{/each}

<!-- Filtering jobs. -->
<section id="filtering-jobs" data-spy class="mb-16 scroll-mt-24 border-t border-border pt-12">
  <h2 class="text-2xl font-semibold tracking-tight">Filtering jobs</h2>
  <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">
    These parameters apply to <code class="rounded bg-secondary px-1 py-0.5 font-mono text-[13px] text-foreground">GET /jobs/search</code>
    and
    <code class="rounded bg-secondary px-1 py-0.5 font-mono text-[13px] text-foreground">GET /jobs/facets</code>.
    Combine any of them with full-text <code class="rounded bg-secondary px-1 py-0.5 font-mono text-[13px] text-foreground">q</code>.
  </p>

  <div class="mt-6 max-w-2xl rounded-xl border border-brand-ring/30 bg-brand-muted/30 p-4">
    <p class="text-xs font-semibold uppercase tracking-wide text-brand-strong">Modifiers — apply to every facet</p>
    <ul class="mt-3 space-y-2 text-sm leading-relaxed text-foreground/90">
      {#each FILTER_MODIFIERS as m (m)}
        <li class="flex gap-2"><span class="mt-0.5 text-brand-strong/70">›</span><span>{@html inlineCode(m)}</span></li>
      {/each}
    </ul>
  </div>

  <h3 class="mt-10 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Facets</h3>
  <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
    Every facet below supports repeat-OR, <code class="rounded bg-secondary px-1 py-0.5 font-mono text-[13px] text-foreground">_mode=and</code>, and
    <code class="rounded bg-secondary px-1 py-0.5 font-mono text-[13px] text-foreground">_exclude</code> as described above.
  </p>
  {@render filterTable(FILTER_FACETS)}

  <h3 class="mt-10 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Numeric &amp; boolean filters</h3>
  {@render filterTable(FILTER_EXTRAS)}

  <h3 class="mt-10 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Recipes</h3>
  <div class="mt-4 grid max-w-3xl gap-3 sm:grid-cols-2">
    {#each RECIPES as r (r.query)}
      <div class="rounded-lg border border-border bg-secondary/30 p-3">
        <p class="text-sm text-foreground">{r.title}</p>
        <p class="mt-1.5 break-all font-mono text-xs text-muted-foreground">
          <span class="text-muted-foreground/60">?</span>{r.query}
        </p>
      </div>
    {/each}
  </div>
</section>

<!-- Endpoint index: one card per group, each endpoint links to its own page. -->
<section id="endpoints" class="scroll-mt-24 border-t border-border pt-12">
  <h2 class="text-2xl font-semibold tracking-tight">Endpoints</h2>
  <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">
    Every endpoint has its own page. Pick one below or from the sidebar.
  </p>

  <div class="mt-8 grid gap-8 sm:grid-cols-2">
    {#each NAV as group (group.slug)}
      <div>
        <h3 class="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground/80">{group.title}</h3>
        <ul class="mt-3 space-y-1">
          {#each group.endpoints as ep (ep.slug)}
            <li>
              <a
                href={ep.href}
                class="group flex items-baseline gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-secondary"
              >
                <span class={`w-10 shrink-0 text-[10px] font-bold tracking-wide ${METHOD_TEXT[ep.method]}`}>{ep.method}</span>
                <span class="min-w-0">
                  <span class="font-mono text-[13px] text-foreground">{ep.path}</span>
                  <span class="block truncate text-xs text-muted-foreground">{ep.endpoint.summary}</span>
                </span>
              </a>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
</section>

{#snippet filterTable(rows: { param: string; label: string; values: string }[])}
  <div class="mt-3 overflow-x-auto rounded-xl border border-border">
    <table class="w-full border-collapse text-left text-sm">
      <thead class="bg-secondary/60 text-xs uppercase tracking-wide text-muted-foreground">
        <tr>
          <th class="px-3 py-2 font-medium">Param</th>
          <th class="px-3 py-2 font-medium">Filter</th>
          <th class="px-3 py-2 font-medium">Values</th>
        </tr>
      </thead>
      <tbody>
        {#each rows as f (f.param)}
          <tr class="border-t border-border align-top transition-colors hover:bg-secondary/30">
            <td class="px-3 py-2"><code class="font-mono text-brand-strong">{f.param}</code></td>
            <td class="px-3 py-2 text-foreground/80">{f.label}</td>
            <td class="px-3 py-2 text-muted-foreground">{f.values}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/snippet}
