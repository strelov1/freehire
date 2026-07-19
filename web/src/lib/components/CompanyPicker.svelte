<script lang="ts">
  import { api } from '$lib/api';
  import type { CompanyListItem } from '$lib/types';
  import CompanyLogo from './CompanyLogo.svelte';

  // A typeahead over the company catalogue: type a name, pick from a logo'd list, and
  // the chosen company's slug flows back through onSelect. Replaces a raw slug input so
  // the offerer never has to guess the slug.
  let {
    onSelect,
  }: {
    onSelect: (company: { slug: string; name: string } | null) => void;
  } = $props();

  let query = $state('');
  let results = $state.raw<CompanyListItem[]>([]);
  let open = $state(false);
  let loading = $state(false);
  let picked = $state<{ slug: string; name: string } | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  const DEBOUNCE_MS = 200;
  const LIMIT = 8;

  function onInput() {
    // Typing invalidates any prior pick until a new one is chosen.
    if (picked) {
      picked = null;
      onSelect(null);
    }
    open = true;
    clearTimeout(timer);
    const q = query.trim();
    if (q.length < 2) {
      results = [];
      loading = false;
      return;
    }
    loading = true;
    timer = setTimeout(async () => {
      try {
        const slice = await api.listCompanies(q, LIMIT, 0);
        results = slice.items;
      } catch {
        results = [];
      }
      loading = false;
    }, DEBOUNCE_MS);
  }

  function pick(c: CompanyListItem) {
    picked = { slug: c.slug, name: c.name };
    query = c.name;
    open = false;
    results = [];
    onSelect(picked);
  }
</script>

<div class="relative">
  <input
    type="text"
    bind:value={query}
    oninput={onInput}
    onfocus={() => query.trim().length >= 2 && (open = true)}
    onblur={() => setTimeout(() => (open = false), 120)}
    placeholder="Search your company…"
    autocomplete="off"
    role="combobox"
    aria-expanded={open}
    aria-controls="company-picker-list"
    class="w-full rounded-md border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {picked
      ? 'border-brand'
      : 'border-border'}"
  />

  {#if open && (results.length > 0 || loading)}
    <ul
      id="company-picker-list"
      role="listbox"
      class="absolute z-10 mt-1 max-h-64 w-full overflow-auto rounded-md border border-border bg-popover shadow-lg"
    >
      {#if loading && results.length === 0}
        <li class="px-3 py-2 text-sm text-muted-foreground">Searching…</li>
      {/if}
      {#each results as c (c.slug)}
        <li>
          <!-- mousedown (not click) so the pick lands before the input's blur closes the list -->
          <button
            type="button"
            role="option"
            aria-selected="false"
            onmousedown={(e) => {
              e.preventDefault();
              pick(c);
            }}
            class="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-accent"
          >
            <CompanyLogo name={c.name} size="size-6" />
            <span class="flex min-w-0 flex-col">
              <span class="truncate text-sm font-medium">{c.name}</span>
              <span class="truncate text-xs text-muted-foreground">/{c.slug} · {c.job_count} open</span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
