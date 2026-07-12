<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import { articleJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/blog/${data.meta.slug}`);
  const ogImage = $derived(`${page.url.origin}/blog/${data.meta.slug}/og.png`);
  const jsonLd = $derived(jsonLdScript(articleJsonLd(data.meta, page.url.origin)));

  // The compiled mdsvex body component, passed through from the universal load.
  const Body = $derived(data.component);

  // UTC so a date-only frontmatter value renders as itself, not shifted a day by
  // the viewer's timezone (mirrors the OG card).
  const dateFmt = new Intl.DateTimeFormat('en-US', { dateStyle: 'long', timeZone: 'UTC' });
  const formatDate = (iso: string) => dateFmt.format(new Date(iso));
</script>

<Seo
  title="{data.meta.title} · freehire"
  description={data.meta.summary}
  {canonical}
  ogType="article"
  image={ogImage}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-2xl px-4 py-6">
  <a
    href={resolve('/blog')}
    class="text-sm text-muted-foreground hover:text-foreground"
  >
    ← Blog
  </a>

  <article class="mt-4">
    <header class="mb-6">
      <div class="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
        <span class="rounded-full border border-border px-2 py-0.5 font-medium uppercase tracking-wide">
          {data.meta.type}
        </span>
        <time datetime={data.meta.date}>{formatDate(data.meta.date)}</time>
      </div>
      <h1 class="text-3xl font-semibold tracking-tight">{data.meta.title}</h1>
    </header>

    <div class="prose prose-neutral max-w-none dark:prose-invert">
      <Body />
    </div>
  </article>
</div>
