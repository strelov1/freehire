<script lang="ts">
  import { goto, afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Search, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import type { Job, CompanyListItem } from '$lib/types';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import { cn } from '$lib/utils';
  import CompanyLogo from './CompanyLogo.svelte';
  import HeaderLocationFilter from './HeaderLocationFilter.svelte';

  // The global launcher: jumps straight to a job/company or hands a free-text
  // query to /jobs. TopBar renders it on every page EXCEPT the list pages
  // (/jobs, /companies), which swap in HeaderListSearch — a thin proxy that
  // filters that page's list instead.

  const JOBS_LIMIT = 6;
  const COMPANIES_LIMIT = 4;
  const DEBOUNCE_MS = 250;

  let query = $state('');
  let jobs = $state.raw<Job[]>([]);
  let companies = $state.raw<CompanyListItem[]>([]);
  let open = $state(false);
  let loading = $state(false);
  let noResults = $state(false);
  // The active index runs over the flat [jobs…, companies…] list so the arrow
  // keys move continuously across both sections; -1 means "no selection".
  let activeIndex = $state(-1);

  let inputEl = $state<HTMLInputElement | null>(null);
  let wrapEl = $state<HTMLElement | null>(null);

  // Each search bumps the token; a response for an older token is stale and
  // dropped, so a slow request can never overwrite a fresher one.
  let reqToken = 0;

  // A flat view over both sections so the arrow keys move continuously; the
  // route is resolved at navigation time (below) to satisfy the resolve() lint.
  type Flat = { kind: 'job'; job: Job } | { kind: 'company'; company: CompanyListItem };

  const flat = $derived<Flat[]>([
    ...jobs.map((job): Flat => ({ kind: 'job', job })),
    ...companies.map((company): Flat => ({ kind: 'company', company })),
  ]);

  // Shared classes for a result row; the keyboard/hover-active row is highlighted.
  const itemClass = (active: boolean) =>
    cn(
      'flex items-center gap-3 px-3 py-2 text-sm transition-colors',
      active ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50',
    );

  async function runSearch(q: string) {
    // Bump the token on every call — including the empty-query dismissal — so a
    // request still in flight from a prior keystroke is marked stale and can't
    // reopen the dropdown after the user has cleared the field.
    const mine = ++reqToken;
    if (!q) {
      jobs = [];
      companies = [];
      noResults = false;
      loading = false;
      open = false;
      return;
    }
    loading = true;
    // allSettled, not all: the two sections are independent, so one endpoint
    // failing (e.g. search down while the companies list is up) still shows the
    // section that succeeded instead of wiping the whole dropdown.
    const [jr, cr] = await Promise.allSettled([
      api.searchJobs(new URLSearchParams({ q }), JOBS_LIMIT, 0),
      api.listCompanies(q, COMPANIES_LIMIT, 0),
    ]);
    if (mine !== reqToken) return; // superseded by a newer query
    jobs = jr.status === 'fulfilled' ? jr.value.items : [];
    companies = cr.status === 'fulfilled' ? cr.value.items : [];
    noResults = jobs.length === 0 && companies.length === 0;
    activeIndex = -1;
    open = true;
    loading = false;
  }

  // Debounce: re-armed on every keystroke; the cleanup cancels a pending run so
  // only the settled query fires.
  $effect(() => {
    const q = query.trim();
    const id = setTimeout(() => runSearch(q), DEBOUNCE_MS);
    return () => clearTimeout(id);
  });

  // Lock body scroll only while the dropdown covers the screen on mobile; on
  // desktop the dropdown is a small overlay and the page stays scrollable.
  // ($effect is client-only, so no SSR guard is needed around matchMedia.)
  $effect(() => {
    if (!open) return;
    if (window.matchMedia('(min-width: 640px)').matches) return;
    lockScroll();
    return () => unlockScroll();
  });

  // Any navigation (a result click, the full search, or an unrelated route
  // change) dismisses the dropdown.
  afterNavigate(() => {
    open = false;
  });

  function reset() {
    open = false;
    query = '';
    jobs = [];
    companies = [];
    noResults = false;
    activeIndex = -1;
  }

  function selectFlat(item: Flat) {
    reset();
    if (item.kind === 'job') void goto(resolve('/jobs/[slug]', { slug: item.job.public_slug }));
    else void goto(resolve('/companies/[slug]', { slug: item.company.slug }));
  }

  function runFullSearch() {
    const q = query.trim();
    if (!q) return;
    reset();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query string appended to a resolved path
    void goto(`${resolve('/')}?q=${encodeURIComponent(q)}`);
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      open = false;
      inputEl?.blur();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (flat.length) activeIndex = activeIndex < flat.length - 1 ? activeIndex + 1 : 0;
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (flat.length) activeIndex = activeIndex > 0 ? activeIndex - 1 : flat.length - 1;
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      const active = activeIndex >= 0 ? flat[activeIndex] : undefined;
      if (active) selectFlat(active);
      else runFullSearch();
    }
  }

  // Global hotkeys: Cmd/Ctrl+K always focuses; `/` focuses only when the user
  // isn't already typing in a field (so it stays a literal slash there).
  function onWindowKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      inputEl?.focus();
      return;
    }
    if (e.key === '/' && !e.metaKey && !e.ctrlKey) {
      const tag = (document.activeElement as HTMLElement | null)?.tagName;
      if (tag !== 'INPUT' && tag !== 'TEXTAREA') {
        e.preventDefault();
        inputEl?.focus();
      }
    }
  }

  function onWindowClick(e: MouseEvent) {
    if (open && wrapEl && !wrapEl.contains(e.target as Node)) open = false;
  }
</script>

<svelte:window onkeydown={onWindowKeydown} onclick={onWindowClick} />

<div bind:this={wrapEl} class="relative flex-1">
  <!-- Search box -->
  <div
    class="flex h-11 items-center gap-2 rounded-md border border-border bg-background px-3 text-sm focus-within:ring-2 focus-within:ring-ring"
  >
    <!-- Launcher mode: listless pages have no list to filter, so the location trigger
         scopes the jobs feed — a pick navigates to /jobs with that facet. -->
    <HeaderLocationFilter variant="launcher" />
    <div class="h-5 w-px shrink-0 bg-border"></div>
    <Search class="size-4 shrink-0 text-muted-foreground" />
    <input
      bind:this={inputEl}
      bind:value={query}
      onkeydown={onKeydown}
      onfocus={() => {
        if (flat.length || noResults) open = true;
      }}
      type="text"
      placeholder="Search jobs and companies…"
      aria-label="Search jobs and companies"
      autocomplete="off"
      spellcheck="false"
      class="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
    />
    {#if loading}
      <span
        class="size-3.5 shrink-0 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground"
        aria-hidden="true"
      ></span>
    {/if}
    {#if query}
      <button
        type="button"
        onclick={reset}
        aria-label="Clear search"
        class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
      >
        <X class="size-4" />
      </button>
    {:else}
      <kbd
        class="hidden shrink-0 rounded border border-border px-1.5 text-xs text-muted-foreground sm:inline"
        >/</kbd
      >
    {/if}
  </div>

  {#if open}
    <!-- Mobile backdrop: dims the page behind the full-width dropdown. -->
    <button
      class="fixed inset-0 z-40 bg-black/40 sm:hidden"
      aria-label="Close search"
      onclick={() => (open = false)}
    ></button>

    <div
      class="absolute inset-x-0 top-full z-50 mt-2 max-h-[70vh] overflow-y-auto rounded-md border border-border bg-background py-1 shadow-lg"
    >
      {#if noResults}
        <div class="flex items-center gap-2 px-3 py-6 text-sm text-muted-foreground">
          <Search class="size-4" />
          No matches for “{query.trim()}”
        </div>
      {:else}
        {#if jobs.length > 0}
          <p class="px-3 pb-1 pt-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Jobs
          </p>
          {#each jobs as job, i (job.public_slug)}
            <a
              href={resolve('/jobs/[slug]', { slug: job.public_slug })}
              onclick={reset}
              onmouseenter={() => (activeIndex = i)}
              class={itemClass(activeIndex === i)}
            >
              <CompanyLogo name={job.company} size="size-5" />
              <span class="min-w-0 flex-1">
                <span class="block truncate font-medium">{job.title}</span>
                <span class="block truncate text-xs text-muted-foreground">
                  {job.company}{#if job.location}&nbsp;·&nbsp;{job.location}{/if}
                </span>
              </span>
            </a>
          {/each}
        {/if}

        {#if companies.length > 0}
          <p class="px-3 pb-1 pt-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Companies
          </p>
          {#each companies as company, i (company.slug)}
            {@const idx = jobs.length + i}
            <a
              href={resolve('/companies/[slug]', { slug: company.slug })}
              onclick={reset}
              onmouseenter={() => (activeIndex = idx)}
              class={itemClass(activeIndex === idx)}
            >
              <CompanyLogo name={company.name} size="size-5" />
              <span class="min-w-0 flex-1 truncate font-medium">{company.name}</span>
              <span class="shrink-0 text-xs text-muted-foreground">
                {company.job_count}
                {company.job_count === 1 ? 'job' : 'jobs'}
              </span>
            </a>
          {/each}
        {/if}

        <button
          type="button"
          onclick={runFullSearch}
          class="mt-1 flex w-full items-center gap-2 border-t border-border px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:bg-accent/50"
        >
          <kbd class="rounded border border-border px-1.5">Enter</kbd>
          Search all jobs for “{query.trim()}”
        </button>
      {/if}
    </div>
  {/if}
</div>
