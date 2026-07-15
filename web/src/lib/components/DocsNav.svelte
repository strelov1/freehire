<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { CONCEPTS, NAV } from '$lib/docs/nav';
  import { METHOD_TEXT } from '$lib/docs/format';

  // `sticky` styles the rail as a pinned desktop sidebar; false is the inline
  // form used inside the mobile drawer, which scrolls within its own bounds.
  let { sticky = true }: { sticky?: boolean } = $props();

  // Client-side filter: a plain substring match over the visible label. The
  // endpoint set is small, so contains() is the honest tool (no fuzzy scoring).
  let query = $state('');
  const needle = $derived(query.trim().toLowerCase());
  const hit = (s: string) => !needle || s.toLowerCase().includes(needle);

  const visibleConcepts = $derived(CONCEPTS.filter((c) => hit(c.title)));
  const visibleGroups = $derived(
    NAV.map((g) => {
      if (hit(g.title)) return g;
      const endpoints = g.endpoints.filter((e) => hit(e.path) || hit(e.method));
      return endpoints.length ? { ...g, endpoints } : null;
    }).filter((g): g is (typeof NAV)[number] => g !== null),
  );

  const pathname = $derived(page.url.pathname);
  const onLanding = $derived(pathname === '/docs/api');

  // Scroll-spy for the concept sections — only ever fires on the landing page,
  // where the `[data-spy]` sections exist. Endpoint pages have none, so no
  // concept lights up there (the active endpoint route does instead).
  let activeSpy = $state('');
  onMount(() => {
    const targets = Array.from(document.querySelectorAll<HTMLElement>('[data-spy]'));
    if (!targets.length) return;
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) if (e.isIntersecting) activeSpy = e.target.id;
      },
      { rootMargin: '-96px 0px -66% 0px', threshold: 0 },
    );
    for (const t of targets) io.observe(t);
    return () => io.disconnect();
  });
</script>

<nav
  class={`space-y-4 overflow-y-auto text-sm ${
    sticky ? 'sticky top-20 max-h-[calc(100vh-6rem)] pb-10' : 'max-h-[70vh] pb-4'
  }`}
>
  <div class="relative">
    <input
      type="search"
      bind:value={query}
      placeholder="Filter endpoints…"
      aria-label="Filter endpoints"
      class="w-full rounded-lg border border-border bg-secondary/40 py-1.5 pl-8 pr-3 text-sm text-foreground placeholder:text-muted-foreground/70 focus:border-brand-ring focus:outline-none focus:ring-2 focus:ring-brand-ring/40"
    />
    <svg
      class="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      aria-hidden="true"
    >
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3-3" stroke-linecap="round" />
    </svg>
  </div>

  {#if visibleConcepts.length}
    <div class="space-y-0.5">
      {#each visibleConcepts as item (item.id)}
        <a
          href={`/docs/api#${item.id}`}
          class={`block rounded-md px-2 py-1 transition-colors ${
            onLanding && activeSpy === item.id
              ? 'bg-brand-muted/60 text-brand-strong'
              : 'text-muted-foreground hover:bg-secondary hover:text-foreground'
          }`}
        >
          {item.title}
        </a>
      {/each}
    </div>
  {/if}

  {#each visibleGroups as group (group.slug)}
    <div class="space-y-0.5">
      <p class="mt-2 px-2 pb-1 pt-2 font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground/80">
        {group.title}
      </p>
      {#each group.endpoints as ep (ep.slug)}
        <a
          href={ep.href}
          class={`flex items-center gap-2 rounded-md px-2 py-1 transition-colors ${
            pathname === ep.href
              ? 'bg-brand-muted/60 text-brand-strong'
              : 'text-muted-foreground hover:bg-secondary hover:text-foreground'
          }`}
          aria-current={pathname === ep.href ? 'page' : undefined}
        >
          <span class={`w-9 shrink-0 text-[9px] font-bold tracking-wide ${METHOD_TEXT[ep.method]}`}>{ep.method}</span>
          <span class="truncate font-mono text-[13px]">{ep.path}</span>
        </a>
      {/each}
    </div>
  {/each}

  {#if !visibleConcepts.length && !visibleGroups.length}
    <p class="px-2 py-4 text-xs text-muted-foreground">No endpoints match “{query}”.</p>
  {/if}
</nav>
