<script lang="ts">
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import { onDestroy, onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { X } from '@lucide/svelte';
  import { ApiError, api } from '#lib/api.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import type { WatchedCompany } from '#lib/types.js';
  import { Button, EntityLogo, Input, LoadMore } from '#lib/ui/index.js';
  import CompanyPicker from '#lib/components/CompanyPicker.svelte';
  import States from '#lib/components/States.svelte';
  import WatchlistAlert from '#lib/components/WatchlistAlert.svelte';
  import WatchlistImport from '#lib/components/WatchlistImport.svelte';

  const PAGE = 50;
  const DEBOUNCE_MS = 250;

  let items = $state.raw<WatchedCompany[]>([]);
  let matches = $state(0);
  let size = $state(0);
  let max = $state(5000);
  let hasMore = $state(false);
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let moreBusy = $state(false);
  let moreFailed = $state(false);
  let filter = $state('');
  let applied = $state('');
  let notice = $state<string | null>(null);
  let addError = $state<string | null>(null);
  let pickerKey = $state(0);

  // A response for a superseded query must not overwrite a newer one.
  let token = 0;

  async function load(append = false) {
    const mine = ++token;
    const q = applied;
    const offset = append ? items.length : 0;
    if (append) {
      moreBusy = true;
      moreFailed = false;
    } else {
      status = 'loading';
    }
    try {
      const page = await api.listWatchlist(q, PAGE, offset);
      if (mine !== token) return;
      items = append ? [...items, ...page.items] : page.items;
      matches = page.total ?? items.length;
      if (q === '') size = matches;
      max = page.max;
      hasMore = page.hasMore;
      status = 'ready';
    } catch {
      if (mine !== token) return;
      if (append) moreFailed = true;
      else status = 'error';
    } finally {
      if (mine === token) moreBusy = false;
    }
  }

  onMount(() => void load());

  let timer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(timer));

  function onFilter() {
    clearTimeout(timer);
    timer = setTimeout(() => {
      applied = filter.trim();
      void load();
    }, DEBOUNCE_MS);
  }

  async function add(company: { slug: string; name: string } | null) {
    if (!company) return;
    addError = null;
    notice = null;
    try {
      await api.watchCompany(company.slug);
      notice = `${company.name} is on your watchlist.`;
      pickerKey += 1;
      await load();
    } catch (e) {
      addError = e instanceof ApiError ? e.message : 'Could not add the company. Please try again.';
    }
  }

  async function remove(company: WatchedCompany) {
    notice = null;
    try {
      await api.unwatchCompany(company.slug);
      items = items.filter((c) => c.slug !== company.slug);
      matches = Math.max(0, matches - 1);
      size = Math.max(0, size - 1);
      if (items.length === 0 && hasMore) await load();
    } catch (e) {
      notice = e instanceof ApiError ? e.message : 'Could not remove the company. Please try again.';
    }
  }
</script>

<svelte:head>
  <title>Watchlist — freehire</title>
</svelte:head>

<div class="flex max-w-3xl flex-col gap-6">
  <div class="flex flex-col gap-1">
    <AccountPageHeading>Watchlist</AccountPageHeading>
    <p class="text-sm text-muted-foreground max-lg:sr-only">
      Companies you follow. Filter the job list down to them, or get an alert when they post.
    </p>
  </div>

  <WatchlistAlert />

  <section class="flex flex-col gap-3 rounded-2xl border border-border bg-card p-5" aria-labelledby="watchlist-add">
    <h2 id="watchlist-add" class="text-base font-semibold tracking-tight">Add a company</h2>
    {#key pickerKey}
      <CompanyPicker onSelect={add} aria-describedby={addError ? 'watchlist-add-error' : undefined} />
    {/key}
    {#if addError}
      <p id="watchlist-add-error" class="text-sm text-destructive">{addError}</p>
    {/if}
    <details class="group">
      <summary class="cursor-pointer text-sm font-medium text-primary hover:underline">Import many at once</summary>
      <div class="mt-3">
        <WatchlistImport {max} onImported={() => void load()} />
      </div>
    </details>
  </section>

  <section class="flex flex-col gap-3" aria-labelledby="watchlist-list">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 id="watchlist-list" class="text-base font-semibold tracking-tight">
        Your companies
        <span class="ml-1 text-sm font-normal text-muted-foreground" data-testid="watchlist-count">{size} of {max}</span
        >
      </h2>
      <Button variant="outline" size="sm" href={`${resolve('/jobs')}?watchlist=true`}>Show jobs</Button>
    </div>
    {#if size > 0 || applied !== ''}
      <Input
        type="search"
        bind:value={filter}
        oninput={onFilter}
        placeholder="Filter by name"
        aria-label="Filter by name"
        class="w-full"
      />
    {/if}
    {#if notice}
      <p role="status" class="text-sm text-muted-foreground">{notice}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Failed to load your watchlist." onRetry={() => void load()} />
    {:else if items.length === 0 && applied !== ''}
      <States state="empty" message="No company on your watchlist matches that name." />
    {:else if items.length === 0}
      <States state="empty" message="Your watchlist is empty." />
      <p class="text-center text-sm text-muted-foreground">
        Add a company above, import a list of names, or press Watch on any company page.
      </p>
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
        {#each items as company (company.slug)}
          <li class="flex items-center gap-3 px-3 py-2">
            <EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="xs" />
            <a
              href={resolve('/companies/[slug]', { slug: company.slug })}
              class="min-w-0 flex-1 truncate text-sm font-medium hover:underline"
            >
              {company.name}
            </a>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Remove {company.name} from watchlist"
              title="Remove"
              onclick={() => remove(company)}
            >
              <X class="size-4" aria-hidden="true" />
            </Button>
          </li>
        {/each}
      </ul>
      {#if hasMore}
        <LoadMore loading={moreBusy} error={moreFailed} onclick={() => void load(true)} />
      {/if}
    {/if}
  </section>
</div>
