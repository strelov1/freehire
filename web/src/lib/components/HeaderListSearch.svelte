<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { Search, SlidersHorizontal, Tag, X } from '@lucide/svelte';
  import { listSearchTarget } from '$lib/listSearch.svelte';
  import { headerFilterTrigger } from '$lib/headerFilterTrigger';
  import { suggestRoles } from '$lib/roleSuggest';
  import { commit, edit, emptyDraft, reconcile, type SearchDraft } from '$lib/searchDraft';
  import { cn } from '$lib/ui';
  import HeaderLocationFilter from './HeaderLocationFilter.svelte';

  // The header's list-mode input: on /jobs and /companies it IS the page's text
  // search, proxying into the active list store (registered by the view).
  //
  // Typing does NOT run the search. What you type is a draft; Enter or choosing a
  // suggestion commits it. The box used to push every keystroke into the store, so
  // the feed refetched while the visitor was still composing the query — and the
  // half-typed word it searched for was rarely the one they meant.
  //
  // It shows ONE dropdown, and only where the registered target published a
  // `roleSuggest` capability — the jobs-backed lists. Free text is otherwise the
  // whole interaction here, but most of what people type into this box names a role
  // the `role` facet already tags, and the facet is otherwise only reachable through
  // the filter modal.
  let { placeholder }: { placeholder: string } = $props();

  // How long the draft must sit still before the suggestions are recomputed. A pass
  // costs ~10 ms over the catalogue on a warm desktop, more on a phone. Short enough
  // to read as instant, long enough that a fast typist pays for it once rather than
  // per letter.
  const SUGGEST_DEBOUNCE_MS = 120;

  let inputEl = $state<HTMLInputElement | null>(null);
  let wrapEl = $state<HTMLDivElement | null>(null);
  // -1 means nothing is highlighted, which is the state the dropdown opens in: Enter
  // then falls through to the free-text search it has always run.
  let activeIndex = $state(-1);
  let dismissed = $state(false);
  let settledQuery = $state('');

  const target = $derived(listSearchTarget());
  // Fall back to the URL's `q` before the view registers (SSR + first paint), so a
  // shared /jobs?q=… link shows its query immediately.
  const q = $derived(target?.value.q ?? page.url.searchParams.get('q') ?? '');

  // What the box shows, which is only the committed query until someone types.
  //
  // Seeded from `q` rather than from an empty string: `$effect` does not run during
  // SSR, so an empty seed would render `/jobs?q=java` with an empty box on the server
  // and only fill it once the client hydrated. Capturing just the initial value is
  // the intent — every later move of `q` arrives through the reconcile below.
  // svelte-ignore state_referenced_locally
  let draft = $state<SearchDraft>(emptyDraft(q));

  // Fold the committed query back in whenever it moves on its own: history
  // navigation, a filter chip removed, a suggestion applied. `untrack` reads the
  // current draft without subscribing to it — this effect writes `draft`, so
  // tracking the read would make it re-run itself forever.
  $effect(() => {
    const committed = q;
    draft = reconcile(untrack(() => draft), committed);
  });

  // The All-filters trigger: shown (with its active-filter badge) only on list pages
  // that published `openFilters`; the count getter is called inside this $derived so
  // the badge tracks the view's live filter state.
  const filterTrigger = $derived(headerFilterTrigger(target));

  // Roles are a jobs facet, so the companies list publishes no `roleSuggest` and this
  // stays null there — the header never asks which page it is on.
  const roleSuggest = $derived(target?.roleSuggest ?? null);

  // Suggestions follow the DRAFT, not the committed query — they are what helps the
  // visitor decide what to commit, so waiting for the commit would be circular.
  $effect(() => {
    const typed = draft.text;
    const timer = setTimeout(() => (settledQuery = typed), SUGGEST_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  });

  const suggestions = $derived(
    roleSuggest ? suggestRoles(settledQuery, roleSuggest.counts(), roleSuggest.active()) : [],
  );
  const suggestOpen = $derived(suggestions.length > 0 && !dismissed);
  // The last row runs the typed text as a free-text search. It used to only dismiss
  // the dropdown, because the list below was already showing those results as you
  // typed; now that typing commits nothing, that row IS how free text gets searched.
  const rowCount = $derived(suggestOpen ? suggestions.length + 1 : 0);

  function close() {
    dismissed = true;
    activeIndex = -1;
  }

  /** Run what is in the box, and close over it. The store owns the URL write and the
   *  reload from here. Every path that searches free text goes through this — Enter,
   *  the dropdown's last row, and the clear button, which is a search for nothing. */
  function runSearch() {
    draft = commit(draft);
    target?.setQuery(draft.committed);
    close();
  }

  function choose(index: number) {
    const picked = suggestions[index];
    if (picked) roleSuggest?.apply(picked.slug);
    close();
  }

  function onKeydown(e: KeyboardEvent) {
    // Enter is handled whether or not the dropdown is open — it is the only way typing
    // reaches the list now, so it cannot sit behind the dropdown's guard. A highlighted
    // ROLE row applies its facet; anything else (nothing highlighted, or the last row,
    // which is the free-text one) searches the text.
    if (e.key === 'Enter') {
      e.preventDefault();
      if (suggestOpen && activeIndex >= 0 && activeIndex < suggestions.length) {
        choose(activeIndex);
      } else {
        runSearch();
      }
      return;
    }
    // Every other key belongs to the dropdown, so with it closed this handler owns
    // nothing — which keeps the input behaving as it does where no suggestions exist.
    if (!suggestOpen) return;
    if (e.key === 'Escape') {
      e.preventDefault(); // keep the typed text; only the dropdown closes
      close();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = activeIndex < rowCount - 1 ? activeIndex + 1 : 0;
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = activeIndex > 0 ? activeIndex - 1 : rowCount - 1;
      return;
    }
  }

  const rowClass = (active: boolean) =>
    cn(
      'flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors',
      active ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50',
    );

  function onWindowClick(e: MouseEvent) {
    if (suggestOpen && wrapEl && !wrapEl.contains(e.target as Node)) close();
  }

  // Same global hotkeys as the launcher: Cmd/Ctrl+K always, `/` unless typing.
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
</script>

<svelte:window onkeydown={onWindowKeydown} onclick={onWindowClick} />

<!-- `min-w-0` lets this flex item shrink below its content's intrinsic width (flex-1
     alone keeps min-width:auto), so the box narrows to fit the header row instead of
     overflowing it — the inner input (also min-w-0) absorbs the shrink. -->
<div bind:this={wrapEl} class="relative min-w-0 flex-1">
  <div
    class="flex h-11 items-center gap-2 rounded-md border border-border bg-background px-3 text-sm focus-within:ring-2 focus-within:ring-ring"
  >
    <!-- List pages expose a filter scope: surface the Location quick-filter as a
         scope-prefix, divided from the search icon. `variant` picks the popover body
         (jobs work-format+location vs the company region/remote-hiring pills). -->
    <!-- Always rendered, launcher-shaped until the list view registers its filter
         scope. That registration happens in the view's `onMount`, ~300ms after first
         paint, and rendering nothing until then made the trigger pop into existence and
         shove this search box 114px to the right — on every load of /jobs and
         /companies. Launcher is the honest stand-in rather than a dead placeholder box:
         it needs no store, it renders the identical neutral label, and a pick during
         those few hundred milliseconds navigates to the feed with that scope instead of
         doing nothing. -->
    <HeaderLocationFilter
      variant={target?.filterScope?.variant ?? 'launcher'}
      store={target?.filterScope?.store}
      counts={target?.filterScope?.counts() ?? null}
      inferred={target?.filterScope?.inferred?.() ?? false}
    />
    <div class="h-5 w-px shrink-0 bg-border"></div>
    <Search class="size-4 shrink-0 text-muted-foreground" />
    <input
      bind:this={inputEl}
      value={draft.text}
      oninput={(e) => {
        dismissed = false;
        activeIndex = -1;
        draft = edit(draft, e.currentTarget.value);
      }}
      onkeydown={onKeydown}
      type="text"
      {placeholder}
      aria-label={placeholder}
      autocomplete="off"
      spellcheck="false"
      role="combobox"
      aria-expanded={suggestOpen}
      aria-controls="role-suggestions"
      aria-activedescendant={activeIndex >= 0 ? `role-suggestion-${activeIndex}` : undefined}
      class="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
    />
    {#if draft.text}
      <!-- Clearing is an explicit act, not typing, so it commits at once: the visitor
           asked for the unfiltered list, not for an empty box over the old results. -->
      <button
        type="button"
        onclick={() => {
          draft = edit(draft, '');
          runSearch();
        }}
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
    <!-- All-filters trigger, mirroring the Location scope-prefix on the left: divided
         from the input and pinned to the right edge. Opens the active page's own filter
         modal; the badge shows the active-filter count. Hidden where no list owns a
         modal (launcher/listless pages register no `openFilters`). -->
    {#if filterTrigger.visible}
      <div class="h-5 w-px shrink-0 bg-border"></div>
      <button
        type="button"
        onclick={() => target?.openFilters?.()}
        aria-label={filterTrigger.count > 0
          ? `Filters (${filterTrigger.count} active)`
          : 'Filters'}
        title="Filters"
        class="relative flex shrink-0 items-center text-muted-foreground transition-colors hover:text-foreground"
      >
        <SlidersHorizontal class="size-4 shrink-0" />
        {#if filterTrigger.count > 0}
          <span
            aria-hidden="true"
            class="absolute -right-2 -top-2 flex h-4 min-w-4 items-center justify-center rounded-full bg-brand px-1 text-[10px] font-semibold leading-none text-brand-foreground"
          >
            {filterTrigger.count}
          </span>
        {/if}
      </button>
    {/if}
  </div>

  <!-- Role suggestions. Rendered only where the list published the capability, so
       /companies needs no exclusion here. Each row applies the `role` facet; the last
       row runs the typed text as a free-text search. -->
  {#if suggestOpen}
    <ul
      id="role-suggestions"
      role="listbox"
      aria-label="Matching roles"
      class="absolute inset-x-0 top-full z-50 mt-2 overflow-hidden rounded-md border border-border bg-background py-1 shadow-lg"
    >
      {#each suggestions as suggestion, i (suggestion.slug)}
        <li role="option" id="role-suggestion-{i}" aria-selected={activeIndex === i}>
          <button
            type="button"
            onmouseenter={() => (activeIndex = i)}
            onclick={() => choose(i)}
            class={rowClass(activeIndex === i)}
          >
            <Tag class="size-4 shrink-0 text-muted-foreground" />
            <span class="min-w-0 flex-1 truncate">{suggestion.label}</span>
            {#if suggestion.count !== undefined}
              <span class="shrink-0 text-xs text-muted-foreground"
                >{suggestion.count.toLocaleString()}</span
              >
            {/if}
          </button>
        </li>
      {/each}
      <li
        role="option"
        id="role-suggestion-{suggestions.length}"
        aria-selected={activeIndex === suggestions.length}
      >
        <button
          type="button"
          onmouseenter={() => (activeIndex = suggestions.length)}
          onclick={runSearch}
          class={cn(rowClass(activeIndex === suggestions.length), 'border-t border-border')}
        >
          <Search class="size-4 shrink-0 text-muted-foreground" />
          <span class="min-w-0 flex-1 truncate text-muted-foreground"
            >Search “{draft.text}” as text</span
          >
        </button>
      </li>
    </ul>
  {/if}
</div>
