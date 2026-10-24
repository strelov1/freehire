<script lang="ts">
  import { api } from '#lib/api.js';
  import type { CompanyListItem } from '#lib/types.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import { EntityLogo } from '#lib/ui/index.js';
  import { ComboboxNav } from '#lib/comboboxNav.svelte.js';

  // A typeahead over the company catalogue: type a name, pick from a logo'd list, and
  // the chosen company's slug flows back through onSelect. Replaces a raw slug input so
  // the offerer never has to guess the slug.
  //
  // `value` is the typed text itself, bindable so a consumer whose field is always free
  // text (no catalogue slug — the experience bank's Company field) can track it live,
  // including a query that never resolves to a pick. `onSelect` stays the channel for a
  // *resolved* company, and is optional for a consumer that only needs the text.
  let {
    value = $bindable(''),
    onSelect,
    id,
    'aria-describedby': ariaDescribedby,
  }: {
    value?: string;
    onSelect?: (company: { slug: string; name: string } | null) => void;
    id?: string;
    'aria-describedby'?: string;
  } = $props();

  let results = $state.raw<CompanyListItem[]>([]);
  let loading = $state(false);
  let picked = $state<{ slug: string; name: string } | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  // Bumped on every onInput — including the too-short-query dismissal — so a
  // response for a superseded keystroke can't overwrite a fresher one (mirrors
  // reqToken in HeaderSearch.svelte / gen in facets/RemoteSearchSelect.svelte).
  let reqToken = 0;

  // Per instance, so two pickers on one page don't share a listbox id.
  const uid = $props.id();
  const nav = new ComboboxNav<CompanyListItem>({
    id: uid,
    items: () => results,
    loading: () => loading,
    pick,
  });

  const DEBOUNCE_MS = 200;
  const LIMIT = 8;

  function onInput() {
    // Typing invalidates any prior pick until a new one is chosen.
    if (picked) {
      picked = null;
      onSelect?.(null);
    }
    nav.open = true;
    nav.active = -1;
    clearTimeout(timer);
    const q = value.trim();
    if (q.length < 2) {
      reqToken++;
      results = [];
      loading = false;
      return;
    }
    loading = true;
    const mine = ++reqToken;
    timer = setTimeout(async () => {
      try {
        const slice = await api.listCompanies(q, LIMIT, 0);
        if (mine !== reqToken) return; // superseded by a newer query
        results = slice.items;
      } catch {
        if (mine !== reqToken) return;
        results = [];
      }
      nav.active = -1;
      if (mine === reqToken) loading = false;
    }, DEBOUNCE_MS);
  }

  function pick(c: CompanyListItem) {
    picked = { slug: c.slug, name: c.name };
    value = c.name;
    nav.close();
    results = [];
    onSelect?.(picked);
  }
</script>

<div class="relative">
  <input
    {id}
    aria-describedby={ariaDescribedby}
    type="text"
    bind:value
    oninput={onInput}
    onfocus={() => value.trim().length >= 2 && (nav.open = true)}
    onblur={() => setTimeout(nav.close, 120)}
    onkeydown={nav.onKeydown}
    placeholder="Search your company…"
    autocomplete="off"
    role="combobox"
    aria-expanded={nav.listOpen}
    aria-controls={nav.listId}
    aria-autocomplete="list"
    aria-activedescendant={nav.activeDescendant}
    class="w-full rounded-md border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {picked
      ? 'border-brand'
      : 'border-border'}"
  />

  {#if nav.listOpen}
    <ul
      id={nav.listId}
      role="listbox"
      class="absolute z-10 mt-1 max-h-64 w-full overflow-auto rounded-md border border-border bg-popover shadow-lg"
    >
      {#if loading && results.length === 0}
        <li role="option" aria-selected="false" aria-disabled="true" class="px-3 py-2 text-sm text-muted-foreground">
          Searching…
        </li>
      {/if}
      {#each results as c, i (c.slug)}
        <!-- mousedown (not click) so the pick lands before the input's blur closes the list;
             tabindex -1 keeps the rows out of the Tab order (the input owns the keyboard). -->
        <li
          id={nav.optionId(i)}
          role="option"
          tabindex="-1"
          aria-selected={nav.active === i}
          onmouseenter={() => (nav.active = i)}
          onmousedown={(e) => {
            e.preventDefault();
            pick(c);
          }}
          class="flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left hover:bg-accent {nav.active === i
            ? 'bg-accent'
            : ''}"
        >
          <EntityLogo name={c.name} src={companyLogoUrl(c.name) ?? undefined} shape="square" size="xs" />
          <span class="flex min-w-0 flex-col">
            <span class="truncate text-sm font-medium">{c.name}</span>
            <span class="truncate text-xs text-muted-foreground">/{c.slug} · {c.job_count} open</span>
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</div>
