<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Search, X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { browseQuery, planForSuggestion } from '#lib/browseTarget.js';
  import { displayQuery } from '#lib/filters.js';
  import { dropdownRows } from '#lib/dropdownRows.js';
  import type { FacetCounts } from '#lib/types.js';
  import { listSearchTarget, type ListSearchTarget } from '#lib/listSearch.svelte.js';
  import { headerFilterTrigger } from '#lib/headerFilterTrigger.js';
  import { trackKeyboardFit } from '#lib/keyboardFitTracker.svelte.js';
  import { openedOverlay, closedOverlay } from '#lib/headerOverlay.js';
  import { applyParams, type ApplyPlan } from '#lib/apiSuggestions.js';
  import { pastedJobLink } from '#lib/jobLink.js';
  import { PastedLinkIntake } from '#lib/pastedLinkIntake.svelte.js';
  import { TypedSuggestions } from '#lib/typedSuggestions.svelte.js';
  import { commit, edit, emptyDraft, reconcile, type SearchDraft } from '#lib/searchDraft.js';
  import { starterSuggestions } from '#lib/suggestions.js';
  import { OperatorPanel } from './operatorPanel.svelte';
  import HeaderStatusPanel from './HeaderStatusPanel.svelte';
  import { focusFieldFromBox, searchBoxClass } from './headerSearchBox';
  import {
    SUGGEST_DEBOUNCE_MS,
    SUGGEST_MIN_CHARS,
    companiesFetch,
    companiesLimit,
    completionsLimit,
    jobsLimit,
  } from './headerSearchLimits';
  import {
    activeOperator,
    hasOperators,
    operatorPlan,
    parseOperators,
    replaceToken,
    unresolvedToken,
    type OperatorOption,
  } from '#lib/searchOperators.js';
  import { typedPlaceholder } from '#lib/typedPlaceholder.svelte.js';
  import { cn } from '#lib/ui/index.js';
  import HeaderLocationFilter from './HeaderLocationFilter.svelte';
  import type { HeaderSearchProps } from './HeaderSearch.props';
  import HeaderFilterTrigger from './HeaderFilterTrigger.svelte';
  import HeaderSuggestionList from './HeaderSuggestionList.svelte';
  import LinkIntakeRow from './LinkIntakeRow.svelte';

  // The header's search box — the ONE of them, on every page.
  //
  // There were two: this, which filtered the list under it, and a launcher on every
  // other page, which navigated to the feed. They shared the debounce, the stale-answer
  // guard, the arrow keys, the hotkeys, the dismissal and the row rendering, in two
  // copies, and differed in exactly one thing: what a pick DOES. So
  // that one thing is a target now (see `target` below), and everything else is
  // written once.
  //
  // Typing does NOT run the search. What you type is a draft; Enter or choosing a row
  // commits it. The box used to push every keystroke into the store, so the feed
  // refetched while the visitor was still composing — and the half-typed word it
  // searched for was rarely the one they meant.
  //
  // `size` and `autofocus` are presentation. The homepage renders this same box, at
  // hero size, as the whole of its content (see HomeLandingView) — it registers no
  // list, so it gets the browse target above and every pick becomes a link to the
  // feed. A hero-sized second copy of this component is how the two would drift.
  let {
    placeholder,
    label,
    size = 'header',
    autofocus = false,
    counts = null,
    onOpenFilters,
  }: HeaderSearchProps = $props();

  const hero = $derived(size === 'hero');

  // The typed placeholder: see typedPlaceholder.svelte.ts.
  const typedHint = typedPlaceholder(() => placeholder);

  // How long a pause is, and what the dropdown asks for: see headerSearchLimits.ts.

  /** The Location popover's own state, held here so the two panels can take turns:
   *  the effect below puts this box's dropdown away when the popover opens, and every
   *  path that focuses the input puts the popover away. */
  let locationOpen = $state(false);

  $effect(() => {
    if (locationOpen) close();
  });

  let inputEl = $state<HTMLInputElement | null>(null);
  let wrapEl = $state<HTMLDivElement | null>(null);

  /** Tailwind's `sm`. Two things below ask it — whether to place the caret on mount, and
   *  how this box's panel is drawn — and they must get one answer, since the markup they
   *  are reasoning about switches on the same breakpoint. */
  const WIDE_QUERY = '(min-width: 640px)';

  // True while the caret sits where the page put it, before the visitor touched anything:
  // the focus ring is for focus somebody moved, and drawn on load it framed the homepage's
  // search in a heavy ring nobody asked for. The first key or pointer press inside clears it.
  let autoFocused = $state(false);

  $effect(() => {
    if (!autofocus) return;
    if (!window.matchMedia(WIDE_QUERY).matches) return;
    inputEl?.focus();
    autoFocused = true;
    // Focusing normally opens the dropdown, and on a landing page that would drop a
    // ten-row panel over the page on every load, covering the very shortcuts printed
    // underneath it. A caret the visitor did not place is not the question "what can I
    // put here", so close it back: their first click or keystroke opens it as usual.
    dismissed = true;
    // Same reasoning, and the same undo: the focus above fires the field's own handler,
    // which stops the placeholder animation. On the homepage — where this box IS the
    // page, and the one surface the animation exists for — that killed it before the
    // first character. A caret nobody placed has not interrupted anything.
    typedHint.resume();
  });
  // -1 means nothing is highlighted, which is the state the dropdown opens in: Enter
  // then falls through to the free-text search it has always run.
  let activeIndex = $state(-1);
  // Starts TRUE, which is what keeps the dropdown shut on a cold page. An empty box
  // now has rows to offer, so without this the starter list would hang open under the
  // header on every load of the feed, focused or not.
  let dismissed = $state(true);
  // Where the caret sits in the box. It decides whether the visitor is inside an
  // operator's value (`company:goo|`), which is what switches the panel to that kind.
  let caret = $state(0);
  // The draft and caret as of the last pause — what the fetches below answer.
  let settled = $state.raw({ text: '', caret: 0 });

  // The distribution behind the empty box on a page with no list of its own. Fetched
  // once, lazily, the first time somebody focuses the box — a page that nobody searches
  // from should not pay for it, and a page that has a list already has the numbers.
  let browseCounts = $state.raw<FacetCounts | null>(null);
  let browseCountsAsked = false;

  function loadBrowseCounts() {
    // Already measured by the page that rendered us — asking again would fetch the
    // identical unfiltered distribution a second time.
    if (counts) return;
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
    void goto(`${resolve('/jobs')}?${query}`);
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
        counts: () => counts ?? browseCounts,
        apply: (s) => browse(planForSuggestion(s)),
        applyParts: browse,
      },
    },
  );
  // Fall back to the URL's `q` before the view registers (SSR + first paint), so a
  // shared /jobs?q=… link shows its query immediately.
  const q = $derived(displayQuery(target.value.q || (page.url.searchParams.get('q') ?? '')));

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
  const filterTrigger = $derived(headerFilterTrigger(registered, onOpenFilters));

  // Roles and categories are jobs facets, so the companies list publishes no `suggest`
  // and this stays null there — the header never asks which page it is on.
  const suggest = $derived(target.suggest ?? null);

  // Suggestions follow the DRAFT, not the committed query — they are what helps the
  // visitor decide what to commit, so waiting for the commit would be circular.
  $effect(() => {
    const next = { text: draft.text, caret };
    const timer = setTimeout(() => (settled = next), SUGGEST_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  });
  // The ordinary sections search the free text only. Sent whole, `company:google java`
  // would ask every endpoint for the word "company:google".
  const settledQuery = $derived(parseOperators(settled.text).text);

  // A box with nothing to complete offers the catalogue's shape; a typed one offers
  // what matches.
  //
  // The first case is the whole point of opening on focus, and it is answered LOCALLY:
  // the curated group order lives in the filter modal's own grouping, checked there
  // against the category vocabulary at compile time, so asking a server for it would
  // be a second copy of that order. The typed case is the endpoint's — it completes a
  // phrase against the catalogue's real vocabulary, which no dictionary shipped to the
  // browser can do.
  //
  // The threshold is SUGGEST_MIN_CHARS, the same one the fetch below gates on, and
  // deliberately not a separate `=== ''`. Two thresholds opened a state nobody designed:
  // at exactly one character the box was past "empty" but short of "asked", so it showed
  // neither the starters nor completions it had not fetched — a ten-row panel collapsing
  // to the single free-text row, then refilling on the next keystroke. One predicate
  // answers "is there a query yet", so there is no gap for a state to fall into.
  const starters = $derived(suggest ? starterSuggestions(suggest.counts()) : []);
  // Completions, postings and companies for the typed text (see TypedSuggestions).
  const typed = new TypedSuggestions(
    () => settledQuery,
    () => !!suggest,
    {
      completions: completionsLimit,
      jobs: jobsLimit,
      companies: companiesLimit,
      companiesFetch,
      minChars: SUGGEST_MIN_CHARS,
    },
  );
  const suggestions = $derived(settled.text.trim().length < SUGGEST_MIN_CHARS ? starters : typed.completions);

  // ── Field operators ───────────────────────────────────────────────────────
  //
  // `title:`, `company:` and `source:` are a typing shortcut for filters the feed
  // already has (see #lib/searchOperators and OperatorPanel).
  const ops = new OperatorPanel(
    () => ({ text: draft.text, caret }),
    () => settled,
    () => !!suggest,
    { companiesFetch, minChars: SUGGEST_MIN_CHARS },
  );

  /** Put the caret at `at` once the box has rendered the text it points into. */
  async function placeCaret(at: number) {
    await tick();
    inputEl?.focus();
    inputEl?.setSelectionRange(at, at);
    // Last: focusing runs `onfocus`, which reads the caret from the box BEFORE the range
    // above moved it.
    caret = at;
  }

  /** A picked operator row: write its canonical value over the token and keep typing. */
  function chooseOperator(option: OperatorOption) {
    const op = activeOperator(draft.text, caret);
    // The row may belong to the token the caret just LEFT (its answer is a debounce
    // behind); writing a company over a `source:` token is a wrong filter, not a pick.
    if (!op || op.key !== option.key) return;
    if (option.key === 'company') ops.lookup.rememberCompany(option.value, option.id);
    const next = replaceToken(draft.text, op, option.value);
    draft = edit(draft, next.text);
    activeIndex = -1;
    void placeCaret(next.caret);
  }

  /** Enter on a draft carrying operators. Everything applies, or — when a value names
   *  nothing — nothing does, and the caret goes to that value with its options open: a
   *  feed filtered by half of what was typed is not what was typed. */
  async function runOperatorSearch(apply: (plan: ApplyPlan) => void) {
    const text = draft.text;
    const resolvers = await ops.lookup.resolversFor(text);
    // Typing went on while the lookups were out; this answer is about older text.
    if (draft.text !== text) return;
    const bad = unresolvedToken(text, resolvers);
    if (bad) {
      dismissed = false;
      activeIndex = -1;
      await placeCaret(bad.end);
      return;
    }
    apply(operatorPlan(parseOperators(text), resolvers));
    // Cleared for the same reason a picked completion clears it: the filters now carry
    // what was in the box.
    draft = commit(edit(draft, ''));
    close();
  }

  // ── A pasted link ─────────────────────────────────────────────────────────
  //
  // The box is one input serving two intents: almost everything typed into it is a
  // query, and occasionally somebody drops in the URL of a vacancy they found elsewhere.
  // Searching that URL as text finds nothing every time, so the box recognises it and
  // offers the other thing instead — look it up, and hand it in if we don't have it.
  //
  // Recognition follows the DRAFT rather than the settled query: a paste is a complete
  // thought the moment it lands, and making it wait out the suggestion debounce would
  // show a panel of nothing first.
  const link = $derived(pastedJobLink(draft.text, page.url.origin));

  // Looking the link up and handing it in (see PastedLinkIntake).
  const intake = new PastedLinkIntake(
    () => link,
    (slug) => openPosting(slug),
  );

  /** Go to a posting and leave the box empty behind us: the URL that got us there is
   *  not a query, and leaving it sitting in the header over the vacancy it opened reads
   *  as a search that is still running. */
  function openPosting(slug: string) {
    draft = commit(edit(draft, ''));
    close();
    void goto(resolve('/jobs/[slug]', { slug }));
  }

  const rows = $derived(
    dropdownRows({
      suggestions,
      jobs: typed.jobs,
      companies: typed.companies,
      text: draft.text,
      link,
      operator: ops.options,
    }),
  );
  const suggestOpen = $derived(rows.length > 0 && !dismissed);
  const rowCount = $derived(suggestOpen ? rows.length : 0);

  // Where the panel may sit with the on-screen keyboard up (see trackKeyboardFit). Pinned to
  // the bottom in exactly the case the class list below pins to `bottom-0`: the header's box,
  // on a phone. The hero hangs its panel off the box at every width, and so does the header
  // at `sm` and up.
  const keyboard = trackKeyboardFit(
    () => suggestOpen,
    () => wrapEl,
    () => !hero && !window.matchMedia(WIDE_QUERY).matches,
  );

  function close() {
    dismissed = true;
    activeIndex = -1;
  }

  // Close whatever other header overlay (the bell dropdown, the hamburger menu)
  // was open, and let them close this one back — see headerOverlay.ts.
  $effect(() => {
    if (!suggestOpen) return;
    openedOverlay(close);
    return () => closedOverlay(close);
  });

  /** Run what is in the box, and close over it. The store owns the URL write and the
   *  reload from here. Every path that searches free text goes through this — Enter,
   *  the dropdown's last row, and the clear button, which is a search for nothing.
   *
   *  Where that goes depends on the page: a list filters in place, everything else
   *  navigates to the feed carrying the query. The target is never null — off a list
   *  page it is the navigating one — so this path has no "nobody received it" case. */
  function runSearch() {
    // Enter on a pasted link runs the link, not a text search for it. This sits here
    // rather than in the keyboard handler because every path that searches free text
    // comes through this function, and a URL is never the text somebody meant to search
    // for — including from the dropdown's own last row, which is why that row is not
    // offered at all while a link is recognised.
    if (link) {
      void intake.activate(link);
      return;
    }
    // Operators need a list that takes facets; the companies list publishes none, so
    // there the text is searched as written.
    if (suggest && hasOperators(draft.text)) {
      void runOperatorSearch(suggest.applyParts);
      return;
    }
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
    if (row.kind === 'link') {
      void intake.activate(row.link);
      return;
    }
    if (row.kind === 'job') {
      close();
      void goto(resolve('/jobs/[slug]', { slug: row.job.public_slug }));
      return;
    }
    if (row.kind === 'operator') {
      chooseOperator(row.option);
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
    const parts = typed.parts[row.suggestion.slug];
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
<div
  bind:this={wrapEl}
  class="relative min-w-0 flex-1"
  onkeydowncapture={() => (autoFocused = false)}
  onpointerdowncapture={() => (autoFocused = false)}
  onfocusout={() => (autoFocused = false)}
>
  <div class={searchBoxClass(hero, autoFocused)} onpointerdown={(e) => focusFieldFromBox(e, inputEl)}>
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
      bind:open={locationOpen}
    />
    <div class={cn('w-px shrink-0 bg-border', hero ? 'h-7' : 'h-5')}></div>
    <Search class={cn('shrink-0 text-muted-foreground', hero ? 'size-5' : 'size-4')} />
    <input
      bind:this={inputEl}
      value={draft.text}
      onpointerdown={() => {
        // A click on an already-focused box fires no `focus` event, so without this an
        // autofocused hero would stay silent when its own field is clicked.
        dismissed = false;
        activeIndex = -1;
        typedHint.stop();
      }}
      oninput={(e) => {
        dismissed = false;
        activeIndex = -1;
        typedHint.stop();
        // Editing the text asks a different question, so the last link's answer goes
        // with it — otherwise a half-corrected URL sits under a verdict on the old one.
        intake.step = null;
        draft = edit(draft, e.currentTarget.value);
        caret = e.currentTarget.selectionStart ?? draft.text.length;
      }}
      onkeyup={(e) => (caret = e.currentTarget.selectionStart ?? caret)}
      onpointerup={(e) => (caret = e.currentTarget.selectionStart ?? caret)}
      onselect={(e) => (caret = e.currentTarget.selectionStart ?? caret)}
      onfocus={(e) => {
        // `/`, Cmd+K and Tab focus without a click or a key IN the box, so the caret is
        // read here too — otherwise the panel decides its mode from a stale position.
        caret = e.currentTarget.selectionStart ?? caret;
        // Off a list page nothing has measured the catalogue for us, so the empty
        // box's starting points are fetched here — on the first focus, never on load.
        if (!registered) loadBrowseCounts();
        // Reached without a click by `/` and Cmd+K, which is the case the popover's own
        // click-away handler cannot see.
        locationOpen = false;
        // Focus is the question "what can I put here", so it reopens the dropdown —
        // including after a click-away dismissed it, which would otherwise leave the
        // box permanently silent for the rest of the visit.
        dismissed = false;
        activeIndex = -1;
        typedHint.stop();
      }}
      onkeydown={onKeydown}
      type="text"
      placeholder={typedHint.shown}
      aria-label={label}
      autocomplete="off"
      spellcheck="false"
      role="combobox"
      aria-expanded={suggestOpen}
      aria-controls="role-suggestions"
      aria-activedescendant={activeIndex >= 0 ? `role-suggestion-${activeIndex}` : undefined}
      class={cn(
        'min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground',
        !hero && 'placeholder:text-sm',
      )}
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
        class="grid size-6 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:text-foreground"
      >
        <!-- 24px box around the 16px glyph: the glyph alone was the whole target. -->
        <X class="size-4" aria-hidden="true" />
      </button>
    {:else}
      <kbd class="hidden shrink-0 rounded border border-border px-1.5 text-xs text-muted-foreground sm:inline">/</kbd>
    {/if}
    <HeaderFilterTrigger open={filterTrigger.open} count={filterTrigger.count} />
  </div>

  <!-- Three sections and a free-text row, flattened by `dropdownRows` into the single
       list the keyboard walks. Rendered only where the list published the capability,
       so /companies needs no exclusion here.

       Section headings ride on the row (`first`) rather than living in a second
       structure: one list means one set of indices, and the arrow keys cross a section
       boundary without knowing there was one. -->
  <!-- A recognised link takes the panel over. It is not a listbox: there is one thing to
       do and no list to walk, and the states it passes through carry a link of their own
       (sign in, view the company), which cannot live inside an option's button. -->
  {#if link && !dismissed}
    <HeaderStatusPanel {hero}>
      <LinkIntakeRow
        url={link.url}
        busy={intake.busy}
        step={intake.step}
        onActivate={() => link && intake.activate(link)}
      />
    </HeaderStatusPanel>
  {:else if ops.miss && ops.live && !dismissed}
    <HeaderStatusPanel {hero}>
      <span class="text-muted-foreground">
        No {ops.live.key} matches “{ops.live.value}”
      </span>
    </HeaderStatusPanel>
  {:else if suggestOpen}
    <HeaderSuggestionList
      {rows}
      bind:activeIndex
      {hero}
      fit={keyboard.fit}
      emptyQuery={draft.text.trim() === ''}
      onChoose={choose}
    />
  {/if}
</div>
