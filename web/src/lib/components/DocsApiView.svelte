<script lang="ts">
  import { Badge } from '$lib/ui';
  import DocsCodeBlock from './DocsCodeBlock.svelte';
  import { BASE_URL, OVERVIEW, GROUPS, AUTH_LABELS, type Auth, type Endpoint, type Param } from '$lib/docs/api-spec';
  import { FILTER_FACETS, FILTER_EXTRAS, FILTER_MODIFIERS, RECIPES } from '$lib/docs/filters';

  // Anchor slug, matching the Markdown generator so in-page links line up with
  // the generated docs/API.md table of contents.
  function slug(s: string): string {
    return s
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '');
  }

  // Table-of-contents entries: overview sections, the filter reference, then
  // each endpoint group.
  const toc = [
    ...OVERVIEW.map((o) => ({ id: slug(o.title), title: o.title })),
    { id: 'filtering-jobs', title: 'Filtering jobs' },
    ...GROUPS.map((g) => ({ id: slug(g.title), title: g.title })),
  ];

  // Public endpoints read as the calmest; everything gated is "outline".
  const authVariant = (a: Auth) => (a === 'none' ? 'secondary' : 'outline');

  // Subtle per-method tint so the verb is scannable.
  const methodClass: Record<Endpoint['method'], string> = {
    GET: 'text-emerald-600 dark:text-emerald-400',
    POST: 'text-sky-600 dark:text-sky-400',
    PATCH: 'text-amber-600 dark:text-amber-400',
    DELETE: 'text-rose-600 dark:text-rose-400',
  };
</script>

<div class="grid gap-10 lg:grid-cols-[16rem_1fr]">
  <!-- Sticky table of contents. -->
  <aside class="hidden lg:block">
    <nav class="sticky top-20 space-y-1 text-sm">
      <p class="mb-2 font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// reference</p>
      {#each toc as item (item.id)}
        <a
          href={`#${item.id}`}
          class="block rounded-md px-2 py-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
        >
          {item.title}
        </a>
      {/each}
    </nav>
  </aside>

  <div class="min-w-0">
    <!-- Header. -->
    <header class="mb-10">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// freehire API</p>
      <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">API reference</h1>
      <p class="mt-5 max-w-2xl text-lg leading-relaxed text-muted-foreground">
        A read-first, open HTTP API over the freehire job catalogue — query jobs by rich filters, read
        companies, and (with a key) track applications. Base URL
        <code class="font-mono text-foreground">{BASE_URL}</code>.
      </p>
    </header>

    <!-- Overview sections. -->
    {#each OVERVIEW as section (section.title)}
      <section id={slug(section.title)} class="mb-10 scroll-mt-20">
        <h2 class="text-xl font-semibold tracking-tight">{section.title}</h2>
        {#each section.paragraphs as p (p)}
          <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">{p}</p>
        {/each}
        {#if section.code}
          <div class="mt-4 max-w-2xl">
            <DocsCodeBlock code={section.code} label="json" />
          </div>
        {/if}
      </section>
    {/each}

    <!-- Filtering jobs. -->
    <section id="filtering-jobs" class="mb-12 scroll-mt-20 border-t border-border pt-10">
      <h2 class="text-2xl font-semibold tracking-tight">Filtering jobs</h2>
      <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">
        These parameters apply to <code class="font-mono text-foreground">GET /jobs/search</code> and
        <code class="font-mono text-foreground">GET /jobs/facets</code>. Combine any of them with full-text
        <code class="font-mono text-foreground">q</code>.
      </p>
      <ul class="mt-4 max-w-2xl space-y-1.5 text-sm leading-relaxed text-muted-foreground">
        {#each FILTER_MODIFIERS as m (m)}
          <li class="flex gap-2"><span class="text-muted-foreground/60">›</span><span>{m}</span></li>
        {/each}
      </ul>

      <h3 class="mt-8 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Facets</h3>
      <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
        Every facet below supports repeat-OR, <code class="font-mono text-foreground">_mode=and</code>, and
        <code class="font-mono text-foreground">_exclude</code> as described above.
      </p>
      <div class="mt-3 overflow-x-auto rounded-lg border border-border">
        <table class="w-full border-collapse text-left text-sm">
          <thead class="bg-secondary/60 text-xs uppercase tracking-wide text-muted-foreground">
            <tr>
              <th class="px-3 py-2 font-medium">Param</th>
              <th class="px-3 py-2 font-medium">Filter</th>
              <th class="px-3 py-2 font-medium">Values</th>
            </tr>
          </thead>
          <tbody>
            {#each FILTER_FACETS as f (f.param)}
              <tr class="border-t border-border align-top">
                <td class="px-3 py-2"><code class="font-mono text-foreground">{f.param}</code></td>
                <td class="px-3 py-2 text-muted-foreground">{f.label}</td>
                <td class="px-3 py-2 text-muted-foreground">{f.values}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <h3 class="mt-8 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
        Numeric &amp; boolean filters
      </h3>
      <div class="mt-3 overflow-x-auto rounded-lg border border-border">
        <table class="w-full border-collapse text-left text-sm">
          <thead class="bg-secondary/60 text-xs uppercase tracking-wide text-muted-foreground">
            <tr>
              <th class="px-3 py-2 font-medium">Param</th>
              <th class="px-3 py-2 font-medium">Filter</th>
              <th class="px-3 py-2 font-medium">Values</th>
            </tr>
          </thead>
          <tbody>
            {#each FILTER_EXTRAS as f (f.param)}
              <tr class="border-t border-border align-top">
                <td class="px-3 py-2"><code class="font-mono text-foreground">{f.param}</code></td>
                <td class="px-3 py-2 text-muted-foreground">{f.label}</td>
                <td class="px-3 py-2 text-muted-foreground">{f.values}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <h3 class="mt-8 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Recipes</h3>
      <dl class="mt-3 max-w-2xl space-y-3">
        {#each RECIPES as r (r.query)}
          <div>
            <dt class="text-sm text-foreground">{r.title}</dt>
            <dd class="mt-1 font-mono text-xs text-muted-foreground">
              <span class="text-muted-foreground/60">{BASE_URL}/jobs/search?</span>{r.query}
            </dd>
          </div>
        {/each}
      </dl>
    </section>

    <!-- Endpoint groups. -->
    {#each GROUPS as group (group.title)}
      <section id={slug(group.title)} class="mb-12 scroll-mt-20 border-t border-border pt-10">
        <h2 class="text-2xl font-semibold tracking-tight">{group.title}</h2>
        <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">{group.intro}</p>

        <div class="mt-6 space-y-8">
          {#each group.endpoints as ep (ep.method + ep.path)}
            <article class="rounded-xl border border-border p-4 sm:p-5">
              <div class="flex flex-wrap items-center gap-3">
                <h3 class="font-mono text-sm sm:text-base">
                  <span class={`font-semibold ${methodClass[ep.method]}`}>{ep.method}</span>
                  <span class="text-foreground">{ep.path}</span>
                </h3>
                <Badge variant={authVariant(ep.auth)} class="ml-auto">{AUTH_LABELS[ep.auth]}</Badge>
              </div>

              <p class="mt-3 leading-relaxed text-foreground">{ep.summary}</p>
              {#if ep.description}
                <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">{ep.description}</p>
              {/if}

              {#if ep.pathParams?.length}
                {@render paramTable('Path parameters', ep.pathParams)}
              {/if}
              {#if ep.query?.length}
                {@render paramTable('Query parameters', ep.query)}
              {/if}
              {#if ep.filterable}
                <p class="mt-3 text-sm text-muted-foreground">
                  Plus every filter in
                  <a href="#filtering-jobs" class="font-medium text-foreground underline-offset-4 hover:underline"
                    >Filtering jobs</a
                  >.
                </p>
              {/if}
              {#if ep.body?.length}
                {@render paramTable('Body', ep.body)}
              {/if}

              <div class="mt-4 space-y-3">
                <DocsCodeBlock code={ep.curl} label="curl" />
                {#if ep.responseExample}
                  <DocsCodeBlock code={ep.responseExample} label="json" />
                {/if}
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/each}
  </div>
</div>

{#snippet paramTable(title: string, params: Param[])}
  <div class="mt-4">
    <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</p>
    <div class="mt-2 overflow-x-auto rounded-lg border border-border">
      <table class="w-full border-collapse text-left text-sm">
        <thead class="bg-secondary/60 text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th class="px-3 py-2 font-medium">Name</th>
            <th class="px-3 py-2 font-medium">Type</th>
            <th class="px-3 py-2 font-medium">Req.</th>
            <th class="px-3 py-2 font-medium">Description</th>
          </tr>
        </thead>
        <tbody>
          {#each params as p (p.name)}
            <tr class="border-t border-border align-top">
              <td class="px-3 py-2"><code class="font-mono text-foreground">{p.name}</code></td>
              <td class="px-3 py-2 text-muted-foreground">{p.type}</td>
              <td class="px-3 py-2 text-muted-foreground">{p.required ? 'yes' : '—'}</td>
              <td class="px-3 py-2 text-muted-foreground">
                {p.description}{#if p.example}
                  <span class="text-muted-foreground/70"> (e.g. <code class="font-mono">{p.example}</code>)</span
                  >{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/snippet}
