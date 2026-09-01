<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import JobRow from '$lib/components/JobRow.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, definedTermJsonLd, jsonLdScript } from '$lib/seo';
  import { Badge } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/skills/${data.slug}`);
  const count = (n: number) => n.toLocaleString('en-US');
  // The description is the page's own sentence, so it is also the meta description —
  // one claim, not a summary of a summary. The posting count trails it as a fact about
  // the catalogue, stated whether or not the list below is shown.
  const metaDescription = $derived(
    `${data.description} ${count(data.total)} open ${data.label} jobs on freehire.`,
  );
  // A query string on a resolve()d base — no dynamic segment for resolve() to fill.
  const jobsHref = $derived(`${resolve('/jobs')}?skills=${encodeURIComponent(data.slug)}`);

  const jsonLd = $derived(
    jsonLdScript([
      // The page's subject, said in a form a machine can read: this heading names a
      // term, that paragraph defines it, and both belong to one glossary.
      definedTermJsonLd(
        { slug: data.slug, label: data.label, description: data.description },
        origin,
      ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Skills', url: `${origin}/skills` },
        { name: data.label, url: canonical },
      ]),
    ]),
  );
</script>

<Seo title={`What is ${data.label}? · freehire`} description={metaDescription} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <nav class="mb-4 flex items-center gap-2 text-sm text-muted-foreground" aria-label="Breadcrumb">
    <a href={resolve('/')} class="hover:underline">freehire</a>
    <span>/</span>
    <a href={resolve('/skills')} class="hover:underline">Skills</a>
  </nav>

  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">What is {data.label}?</h1>
    <p class="mt-3 max-w-2xl leading-relaxed">{data.description}</p>

    <!-- Conditional because two canonicals in three have no spelling but their own
         name, and "also written as: javascript" under a heading reading JavaScript is
         filler. See displayAliases. -->
    {#if data.aliases.length > 0}
      <p class="mt-3 text-sm text-muted-foreground">
        Also written as
        {#each data.aliases as alias, i (alias)}<span class="font-medium text-foreground"
            >{alias}</span
          >{i < data.aliases.length - 1 ? ', ' : ''}{/each}.
      </p>
    {/if}

    <!-- The count is stated whether or not the list below it is: "3 open postings" is a
         fact, and a page that hid it while showing a definition would look like it had
         nothing to say about hiring. -->
    <p class="mt-4 text-sm">
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- query-only /jobs filter, no route segment to resolve -->
      <a href={jobsHref} class="font-medium underline"
        >{count(data.total)} open {data.label} jobs</a
      >
      on freehire.
    </p>
  </header>

  {#if data.neighbours.length > 0}
    <section class="mb-8">
      <h2 class="mb-3 text-sm font-semibold tracking-tight">Named alongside {data.label}</h2>
      <ul class="flex flex-wrap gap-1.5">
        {#each data.neighbours as neighbour (neighbour.slug)}
          <li>
            <a href={resolve('/skills/[slug]', { slug: neighbour.slug })}>
              <Badge variant="secondary" class="transition hover:bg-accent">{neighbour.label}</Badge>
            </a>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if data.showPostings}
    <section>
      <h2 class="mb-3 text-sm font-semibold tracking-tight">Jobs asking for {data.label}</h2>
      <ul class="flex flex-col gap-2">
        {#each data.postings as job (job.public_slug)}
          <li><JobRow {job} /></li>
        {/each}
      </ul>
      <p class="mt-4 text-sm">
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- query-only /jobs filter, no route segment to resolve -->
        <a href={jobsHref} class="font-medium underline">See all {count(data.total)} →</a>
      </p>
    </section>
  {:else}
    <!-- Below the gate the list is withheld and the reason is said out loud, rather
         than the page quietly looking like nobody is hiring. -->
    <p class="text-sm text-muted-foreground">
      Fewer than {data.minPostings} open jobs name {data.label} right now, so there is no listing
      here — a handful of postings describes a moment, not a market.
    </p>
  {/if}
</div>
