<script lang="ts">
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { LayoutGrid, Search, SlidersHorizontal, Tag, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { browseQuery, planForSuggestion } from '$lib/browseTarget';
  import { dropdownRows, namedCompanies } from '$lib/dropdownRows';
  import { companyLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import type { Job, CompanyListItem, ApiSuggestionPart, FacetCounts } from '$lib/types';
  import { listSearchTarget, type ListSearchTarget } from '$lib/listSearch.svelte';
  import { headerFilterTrigger } from '$lib/headerFilterTrigger';
  import { fromApi, applyParams, type ApplyPlan } from '$lib/apiSuggestions';
  import { commit, edit, emptyDraft, reconcile, type SearchDraft } from '$lib/searchDraft';
  import { starterSuggestions, type Suggestion } from '$lib/suggestions';
  import { cn } from '$lib/ui';
  import HeaderLocationFilter from './HeaderLocationFilter.svelte';

  // The header's search box — the ONE of them, on every page.
  //
  // There were two: this, which filtered the list under it, and a launcher on every
  // other page, which navigated to the feed. They shared the debounce, the
  // stale-response token, the arrow keys, the hotkeys, the dismissal and the row
  // rendering, in two copies, and differed in exactly one thing: what a pick DOES. So
  // that one thing is a target now (see `target` below), and everything else is
  // written once.
  //
  // Typing does NOT run the search. What you type is a draft; Enter or choosing a row
  // commits it. The box used to push every keystroke into the store, so the feed
  // refetched while the visitor was still composing — and the half-typed word it
  // searched for was rarely the one they meant.
  let { placeholder }: { placeholder: string } = $props();

  // How long the draft must sit still before the suggestions are recomputed. A pass
  // costs ~10 ms over the catalogue on a warm desktop, more on a phone. Short enough
  // to read as instant, long enough that a fast typist pays for it once rather than
  // per letter.
  const SUGGEST_DEBOUNCE_MS = 120;

  // Section caps. The dropdown is a shortcut, not a results page: past a handful each
  // section stops being scannable and the whole thing stops fitting on a phone.
  const jobsLimit = 5;
  const completionsLimit = 5;
  const companiesLimit = 3;
  // Asked for, before the relevance filter takes its cut.
  const companiesFetch = 12;

  let inputEl = $state<HTMLInputElement | null>(null);
  let wrapEl = $state<HTMLDivElement | null>(null);
  // -1 means nothing is highlighted, which is the state the dropdown opens in: Enter
  // then falls through to the free-text search it has always run.
  let activeIndex = $state(-1);
  // Starts TRUE, which is what keeps the dropdown shut on a cold page. An empty box
  // now has rows to offer, so without this the starter list would hang open under the
  // header on every load of the feed, focused or not.
  let dismissed = $state(true);
  let settledQuery = $state('');

  // The distribution behind the empty box on a page with no list of its own. Fetched
  // once, lazily, the first time somebody focuses the box — a page that nobody searches
  // from should not pay for it, and a page that has a list already has the numbers.
  let browseCounts = $state.raw<FacetCounts | null>(null);
  let browseCountsAsked = false;

  function loadBrowseCounts() {
    if (browseCountsAsked) return;
    browseCountsAsked = true;
    void api
      .facetCounts(new URLSearchParams(), { facets: ['category'] })
      .then((c) => (browseCounts = c))
      // A missing distribution is not an error here: the empty box simply offers
      // nothing, which is what it did on these pages before.
      .catch(() => {});
  }

  /** Open the feed with a filter. What a pick does off a list page — the ONE thing the
   *  launcher ever did differently, and the reason there were two of these components. */
  function browse(plan: ApplyPlan) {
    const query = browseQuery(plan);
    // A plan that names nothing navigates nowhere: landing on an unfiltered feed is
    // not what "search for nothing" should do.
    if (query === '') return;
    close();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query string appended to a resolved path
    void goto(`${resolve('/')}?${query}`);
  }

  // The list page's own store, or — on every other page — a target that navigates to
  // the feed instead of filtering in place. Never null, so nothing below has to ask
  // which kind of page it is on.
  const registered = $derived(listSearchTarget());
  const target = $derived<ListSearchTarget>(
    registered ?? {
      value: { q: '' },
      commitQuery: (q) => browse({ facets: [], q }),
      suggest: {
        counts: () => browseCounts,
        apply: (s) => browse(planForSuggestion(s)),
        applyParts: browse,
      },
    },
  );
  // Fall back to the URL's `q` before the view registers (SSR + first paint), so a
  // shared /jobs?q=… link shows its query immediately.
  const q = $derived(target.value.q || (page.url.searchParams.get('q') ?? ''));

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
  // `$effect.pre` rather than `$effect`: this folds external state IN, so it belongs
  // before the render that shows it. After the DOM update, a back/forward or a removed
  // chip would paint one frame of the old text first.
  $effect.pre(() => {
    const committed = q;
    const owner = target;
    draft = reconcile(
      untrack(() => draft),
      committed,
      owner,
    );
  });

  // The All-filters trigger: shown (with its active-filter badge) only on list pages
  // that published `openFilters`; the count getter is called inside this $derived so
  // the badge tracks the view's live filter state.
  const filterTrigger = $derived(headerFilterTrigger(registered));

  // Roles and categories are jobs facets, so the companies list publishes no `suggest`
  // and this stays null there — the header never asks which page it is on.
  const suggest = $derived(target.suggest ?? null);

  // Suggestions follow the DRAFT, not the committed query — they are what helps the
  // visitor decide what to commit, so waiting for the commit would be circular.
  $effect(() => {
    const typed = draft.text;
    const timer = setTimeout(() => (settledQuery = typed), SUGGEST_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  });

  // An empty box offers the catalogue's shape; a typed one offers what matches.
  //
  // The empty case is the whole point of opening on focus, and it is answered LOCALLY:
  // the curated group order lives in the filter modal's own grouping, checked there
  // against the category vocabulary at compile time, so asking a server for it would
  // be a second copy of that order. The typed case is the endpoint's — it completes a
  // phrase against the catalogue's real vocabulary, which no dictionary shipped to the
  // browser can do.
  const starters = $derived(suggest ? starterSuggestions(suggest.counts()) : []);
  let completions = $state.raw<Suggestion[]>([]);
  const suggestions = $derived(settledQuery.trim() === '' ? starters : completions);
  // The parts each completion applies, by row key — kept beside the rows rather than
  // inside them because a Suggestion is what the dropdown RENDERS, and these are what
  // choosing it DOES.
  let completionParts = $state.raw(new Map<string, ApiSuggestionPart[]>());
  // Postings and companies for the typed text, fetched exactly the way the launcher
  // dropdown (HeaderSearch) fetches them — same endpoints, same stale-response token,
  // same row rendering below. A second implementation of "show me matching jobs" is
  // how the two would drift.
  //
  // These matter MORE now than before, not less: the list below no longer narrows as
  // you type, so these rows are the only live evidence the query finds anything.
  let jobs = $state.raw<Job[]>([]);
  let companies = $state.raw<CompanyListItem[]>([]);
  // Bumped on every fetch; a response for an older token is stale and dropped, so a
  // slow request cannot overwrite a fresher one.
  let previewToken = 0;

  $effect(() => {
    const q = settledQuery.trim();
    const mine = ++previewToken;
    if (q === '' || !suggest) {
      completions = [];
      completionParts = new Map();
      jobs = [];
      companies = [];
      return;
    }
    void (async () => {
      // allSettled, not all: the three sections are independent, so one endpoint
      // failing still shows the sections that succeeded instead of blanking all of
      // them. The completions in particular sit behind a dictionary that is rebuilt on
      // a schedule — a cold or missing one must cost the box its completions, not its
      // postings.
      const [s, j, c] = await Promise.allSettled([
        api.suggest(q, completionsLimit),
        api.searchJobs(new URLSearchParams({ q }), jobsLimit, 0),
        // Over-fetch: most of what the fuzzy endpoint returns is discarded below, and
        // asking for exactly three would leave the section empty whenever the fourth
        // was the only real match.
        api.listCompanies(q, companiesFetch, 0),
      ]);
      if (mine !== previewToken) return;
      const rows = s.status === 'fulfilled' ? s.value : [];
      completions = fromApi(rows);
      completionParts = new Map(completions.map((row, i) => [row.slug, rows[i]?.parts ?? []]));
      jobs = j.status === 'fulfilled' ? j.value.items : [];
      companies =
        c.status === 'fulfilled' ? namedCompanies(c.value.items, q, companiesLimit) : [];
    })();
  });

  const rows = $derived(
    dropdownRows({ suggestions, jobs, companies, text: draft.text }),
  );
  const suggestOpen = $derived(rows.length > 0 && !dismissed);
  const rowCount = $derived(suggestOpen ? rows.length : 0);

  function close() {
    dismissed = true;
    activeIndex = -1;
  }

  /** Run what is in the box, and close over it. The store owns the URL write and the
   *  reload from here. Every path that searches free text goes through this — Enter,
   *  the dropdown's last row, and the clear button, which is a search for nothing.
   *
   *  Where that goes depends on the page: a list filters in place, everything else
   *  navigates to the feed carrying the query. The target is never null — off a list
   *  page it is the navigating one — so this path has no "nobody received it" case. */
  function runSearch() {
    draft = commit(draft);
    target.commitQuery(draft.committed);
    close();
  }

  /** Activate a row. Each section does its own thing: a suggestion applies a facet, a
   *  posting or a company navigates to it, the last row searches the typed text. */
  function choose(index: number) {
    const row = rows[index];
    if (!row) return;
    if (row.kind === 'text') {
      runSearch();
      return;
    }
    if (row.kind === 'job') {
      close();
      void goto(resolve('/jobs/[slug]', { slug: row.job.public_slug }));
      return;
    }
    if (row.kind === 'company') {
      close();
      void goto(resolve('/companies/[slug]', { slug: row.company.slug }));
      return;
    }
    // A completion carries the parts the endpoint composed — the recognised prefix plus
    // what this row adds — and every one of them is applied. Applying a subset would
    // silently discard what the visitor typed, which is the composed search this whole
    // feature exists to make possible.
    //
    // A starter row (the empty box) has no parts: it IS its own facet, applied below.
    const parts = completionParts.get(row.suggestion.slug);
    if (parts?.length) suggest?.applyParts(applyParams(parts));
    else suggest?.apply(row.suggestion);
    // The box is cleared because the filters now carry what was in it — the parts
    // above include the free text a `title` row names. Reconcile cannot see this on a
    // feed with no committed query: `q` does not MOVE (already `''`), and an unchanged
    // value is exactly what it reads as "no news", leaving the typed text sitting over
    // a list no longer running it.
    draft = commit(edit(draft, ''));
    close();
  }

  function onKeydown(e: KeyboardEvent) {
    // Mid-composition (CJK/IME) Enter CONFIRMS a candidate and the arrows move through
    // them; the browser must keep those. `oninput` has already fired by then, so the
    // draft holds pre-conversion text — searching it would search a half-typed word.
    if (e.isComposing) return;
    // Enter is handled whether or not the dropdown is open — it is the only way typing
    // reaches the list now, so it cannot sit behind the dropdown's guard. A highlighted
    // ROLE row applies its facet; anything else (nothing highlighted, or the last row,
    // which is the free-text one) searches the text.
    if (e.key === 'Enter') {
      e.preventDefault();
      if (suggestOpen && activeIndex >= 0) choose(activeIndex);
      else runSearch();
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
      variant={target.filterScope?.variant ?? 'launcher'}
      store={target.filterScope?.store}
      counts={target.filterScope?.counts() ?? null}
      inferred={target.filterScope?.inferred?.() ?? false}
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
      onfocus={() => {
        // Off a list page nothing has measured the catalogue for us, so the empty
        // box's starting points are fetched here — on the first focus, never on load.
        if (!registered) loadBrowseCounts();
        // Focus is the question "what can I put here", so it reopens the dropdown —
        // including after a click-away dismissed it, which would otherwise leave the
        // box permanently silent for the rest of the visit.
        dismissed = false;
        activeIndex = -1;
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
        onclick={() => registered?.openFilters?.()}
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

  <!-- Three sections and a free-text row, flattened by `dropdownRows` into the single
       list the keyboard walks. Rendered only where the list published the capability,
       so /companies needs no exclusion here.

       Section headings ride on the row (`first`) rather than living in a second
       structure: one list means one set of indices, and the arrow keys cross a section
       boundary without knowing there was one. -->
  {#if suggestOpen}
    <ul
      id="role-suggestions"
      role="listbox"
      aria-label="Search suggestions"
      class="absolute inset-x-0 top-full z-50 mt-2 max-h-[70vh] overflow-y-auto rounded-md border border-border bg-background py-1 shadow-lg"
    >
      {#each rows as row, i (row.key)}
        {#if row.first && row.kind !== 'text'}
          <li
            class="px-3 pb-1 pt-2 text-xs font-medium uppercase tracking-wide text-muted-foreground"
          >
            {#if row.kind === 'suggestion'}
              {draft.text.trim() === '' ? 'Where to start' : 'Filter by'}
            {:else if row.kind === 'job'}
              Jobs
            {:else}
              Companies
            {/if}
          </li>
        {/if}
        <li role="option" id="role-suggestion-{i}" aria-selected={activeIndex === i}>
          <button
            type="button"
            onmouseenter={() => (activeIndex = i)}
            onclick={() => choose(i)}
            class={cn(rowClass(activeIndex === i), row.kind === 'text' && 'border-t border-border')}
          >
            {#if row.kind === 'suggestion'}
              <!-- The glyph says which vocabulary the row comes from. A company gets its
                   own mark, the same one the postings below carry, because a logo is
                   what makes an employer scannable; everything else is a glyph. -->
              {#if row.suggestion.kind === 'company'}
                <EntityLogo
                  name={row.suggestion.label}
                  src={companyLogoUrl(row.suggestion.label) ?? undefined}
                  shape="square"
                  size="xs"
                />
              {:else if row.suggestion.kind === 'category'}
                <LayoutGrid class="size-4 shrink-0 text-muted-foreground" />
              {:else if row.suggestion.kind === 'title'}
                <Search class="size-4 shrink-0 text-muted-foreground" />
              {:else}
                <Tag class="size-4 shrink-0 text-muted-foreground" />
              {/if}
              <span class="min-w-0 flex-1 truncate">{row.suggestion.label}</span>
              {#if row.suggestion.count !== undefined}
                <span class="shrink-0 text-xs text-muted-foreground"
                  >{row.suggestion.count.toLocaleString()}</span
                >
              {/if}
            {:else if row.kind === 'job'}
              <!-- Same mark the launcher dropdown renders, from the same resolver: the
                   recognisable logo is what makes a row scannable at a glance. -->
              <EntityLogo
                name={row.job.company || 'Unknown company'}
                src={companyLogoUrl(row.job.company) ?? undefined}
                shape="square"
                size="xs"
              />
              <span class="min-w-0 flex-1">
                <span class="block truncate">{row.job.title}</span>
                <span class="block truncate text-xs text-muted-foreground">
                  {row.job.company}{#if row.job.location}&nbsp;·&nbsp;{row.job.location}{/if}
                </span>
              </span>
            {:else if row.kind === 'company'}
              <EntityLogo
                name={row.company.name}
                src={companyLogoUrl(row.company.name) ?? undefined}
                shape="square"
                size="xs"
              />
              <span class="min-w-0 flex-1 truncate">{row.company.name}</span>
              <span class="shrink-0 text-xs text-muted-foreground">
                {row.company.job_count}
                {row.company.job_count === 1 ? 'job' : 'jobs'}
              </span>
            {:else}
              <Search class="size-4 shrink-0 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate text-muted-foreground"
                >Search “{row.text}” as text</span
              >
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
