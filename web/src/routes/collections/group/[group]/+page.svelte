<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { ChevronLeft } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import CollectionRowMark from '#lib/components/CollectionRowMark.svelte';
  import { breadcrumbJsonLd, collectionPageJsonLd, jsonLdScript } from '#lib/seo.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/collections/group/${data.section.slug}`);
  const heading = $derived(`${data.section.title} collections`);
  // A CollectionPage whose items are the collections themselves, plus the trail
  // freehire → Collections → this section, mirroring a collection's own landing.
  const jsonLd = $derived(
    jsonLdScript([
      collectionPageJsonLd(
        heading,
        data.section.description,
        canonical,
        data.section.rows.map((row) => ({ name: row.title, url: `${origin}/collections/${row.slug}` })),
      ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Collections', url: `${origin}/collections` },
        { name: data.section.title, url: canonical },
      ]),
    ]),
  );
</script>

<Seo title="{heading} · freehire" description={data.section.description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <a
    href={resolve('/collections')}
    class="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
  >
    <ChevronLeft class="size-4" />
    All collections
  </a>

  <header class="mt-4 mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">{data.section.title}</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      {data.section.description}
    </p>
  </header>

  <ul class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
    {#each data.section.rows as row (row.slug)}
      <li>
        <a
          href={resolve('/collections/[slug]', { slug: row.slug })}
          class="flex h-full flex-col gap-3 rounded-xl border border-border bg-background p-5 transition-colors hover:bg-secondary/40"
        >
          <div class="flex items-center gap-3">
            <CollectionRowMark {row} />
            <span class="min-w-0 flex-1">
              <span class="block truncate font-semibold text-foreground">{row.title}</span>
              {#if row.count !== null}
                <span class="block font-mono text-xs text-muted-foreground">
                  {row.count.toLocaleString()} jobs
                </span>
              {/if}
            </span>
          </div>
          <p class="text-sm leading-relaxed text-muted-foreground">{row.description}</p>
        </a>
      </li>
    {/each}
  </ul>
</div>
