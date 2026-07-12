<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import type { PostType } from '$lib/blog';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/blog`);

  // Client-side type filter over the already-loaded list — one feed, two tiers
  // (see design D6). No route/query round-trip; the default view shows everything.
  type Filter = 'all' | PostType;
  const filters: { value: Filter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'changelog', label: 'Changelog' },
    { value: 'article', label: 'Articles' },
  ];
  let active = $state<Filter>('all');
  const posts = $derived(
    active === 'all' ? data.posts : data.posts.filter((post) => post.type === active),
  );

  // UTC so a date-only frontmatter value renders as itself, not shifted a day by
  // the viewer's timezone (mirrors the OG card).
  const dateFmt = new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeZone: 'UTC' });
  const formatDate = (iso: string) => dateFmt.format(new Date(iso));
</script>

<Seo
  title="Blog · freehire"
  description="Product updates, changelog, and longer write-ups from the freehire team — new job sources, search improvements, and how the aggregator works."
  {canonical}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <header class="mb-6">
    <div class="flex flex-wrap items-baseline justify-between gap-3">
      <h1 class="text-2xl font-semibold tracking-tight">Blog</h1>
      <a href={resolve('/blog/rss.xml')} class="text-sm text-muted-foreground hover:text-foreground">
        RSS
      </a>
    </div>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Product updates and the occasional deeper dive.
    </p>
  </header>

  <div class="mb-6 flex gap-1 rounded-lg border border-border p-1 text-sm">
    {#each filters as filter (filter.value)}
      <button
        type="button"
        class="flex-1 rounded-md px-3 py-1.5 font-medium transition-colors {active === filter.value
          ? 'bg-secondary text-foreground'
          : 'text-muted-foreground hover:text-foreground'}"
        aria-pressed={active === filter.value}
        onclick={() => (active = filter.value)}
      >
        {filter.label}
      </button>
    {/each}
  </div>

  {#if posts.length === 0}
    <p class="py-12 text-center text-sm text-muted-foreground">Nothing here yet.</p>
  {:else}
    <ul class="divide-y divide-border">
      {#each posts as post (post.slug)}
        <li class="py-5">
          <div class="mb-1 flex items-center gap-2 text-xs text-muted-foreground">
            <span
              class="rounded-full border border-border px-2 py-0.5 font-medium uppercase tracking-wide"
            >
              {post.type}
            </span>
            <time datetime={post.date}>{formatDate(post.date)}</time>
          </div>
          <h2 class="text-lg font-semibold tracking-tight">
            <a href={resolve('/blog/[slug]', { slug: post.slug })} class="hover:underline">
              {post.title}
            </a>
          </h2>
          <p class="mt-1 text-sm leading-relaxed text-muted-foreground">{post.summary}</p>
          {#if post.tags.length > 0}
            <div class="mt-2 flex flex-wrap gap-1.5">
              {#each post.tags as tag (tag)}
                <span class="text-xs text-muted-foreground">#{tag}</span>
              {/each}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
