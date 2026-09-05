<script lang="ts">
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { LayoutGrid, Link2, Search, SlidersHorizontal, Tag, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { browseQuery, planForSuggestion } from '$lib/browseTarget';
  import { dropdownRows, namedCompanies, type DropdownRow } from '$lib/dropdownRows';
  import { companyLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import type { Job, CompanyListItem, ApiSuggestionPart, FacetCounts } from '$lib/types';
  import { listSearchTarget, type ListSearchTarget } from '$lib/listSearch.svelte';
  import { headerFilterTrigger } from '$lib/headerFilterTrigger';
  import { openedOverlay, closedOverlay } from '$lib/headerOverlay';
  import { fromApi, applyParams, type ApplyPlan } from '$lib/apiSuggestions';
  import { pastedJobLink, type PastedJobLink } from '$lib/jobLink';
  import { runLinkIntake, type LinkIntakeStep } from '$lib/linkIntake';
  import { commit, edit, emptyDraft, reconcile, type SearchDraft } from '$lib/searchDraft';
  import { starterSuggestions, type Suggestion } from '$lib/suggestions';
  import { cn } from '$lib/ui';
  import HeaderLocationFilter from './HeaderLocationFilter.svelte';
  import IntakeOutcome from './IntakeOutcome.svelte';

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
    size = 'header',
    autofocus = false,
    counts = null,
    onOpenFilters,
  }: {
    placeholder: string;
    size?: 'header' | 'hero';
    /** Focus the box on mount — desktop only. A page whose whole content is this box
     *  should put the caret in it; on a phone the same call raises the keyboard over
     *  the page before the visitor has decided to search, so the width check below is
     *  the feature rather than a fallback. */
    autofocus?: boolean;
    /** A filter modal the HOST owns, for a page with no list of its own — the
     *  homepage, where picking filters composes a search rather than narrowing a list.
     *  Where a list IS registered its own modal wins, so this is never a second way to
     *  open the same thing. */
    onOpenFilters?: () => void;
    /** The category distribution behind the empty box, when the page has already
     *  measured it. Off a list page this is otherwise fetched here on first focus;
     *  a caller that server-rendered the same numbers passes them instead of making
     *  the browser ask for them again. */
    counts?: FacetCounts | null;
  } = $props();

  const hero = $derived(size === 'hero');

  // How long the draft must sit still before the suggestions are refetched.
  //
  // A settled draft costs THREE requests (completions, postings, companies), so the
  // window is not sized against one cheap pass — it is sized against the gap between
  // keystrokes. At 120ms it sat inside that gap: a fast typist's 18 characters fired
  // six rounds, eighteen requests, for one query they had not finished writing. 300ms
  // is past the gap and is the window the filter store already debounces its reload by
  // (see urlSynced.svelte.ts) — one answer to "how long is a pause" across the app.
  const SUGGEST_DEBOUNCE_MS = 300;

  // Below this, the box asks nothing. A single character matches a large fraction of
  // the catalogue, so the round trip buys a list nobody can act on — and it is the one
  // round that fires with certainty, since every query passes through its first letter.
  const SUGGEST_MIN_CHARS = 2;

  // Section caps. The dropdown is a shortcut, not a results page: past a handful each
  // section stops being scannable and the whole thing stops fitting on a phone.
  const jobsLimit = 5;
  const completionsLimit = 5;
  const companiesLimit = 3;
  // Asked for, before the relevance filter takes its cut.
  const companiesFetch = 12;

  /** The Location popover's own state, held here so the two panels can take turns:
   *  the effect below puts this box's dropdown away when the popover opens, and every
   *  path that focuses the input puts the popover away. */
  let locationOpen = $state(false);

  $effect(() => {
    if (locationOpen) close();
  });

  let inputEl = $state<HTMLInputElement | null>(null);
  let wrapEl = $state<HTMLDivElement | null>(null);

  $effect(() => {
    if (!autofocus) return;
    if (!window.matchMedia('(min-width: 640px)').matches) return;
    inputEl?.focus();
    // Focusing normally opens the dropdown, and on a landing page that would drop a
    // ten-row panel over the page on every load, covering the very shortcuts printed
    // underneath it. A caret the visitor did not place is not the question "what can I
    // put here", so close it back: their first click or keystroke opens it as usual.
    dismissed = true;
  });
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
  const filterTrigger = $derived(headerFilterTrigger(registered, onOpenFilters));

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
  let completions = $state.raw<Suggestion[]>([]);
  const suggestions = $derived(
    settledQuery.trim().length < SUGGEST_MIN_CHARS ? starters : completions,
  );
  // The parts each completion applies, by row key — kept beside the rows rather than
  // inside them because a Suggestion is what the dropdown RENDERS, and these are what
  // choosing it DOES.
  let completionParts = $state.raw(new Map<string, ApiSuggestionPart[]>());
  // Postings and companies for the typed text, fetched exactly the way the launcher
  // dropdown (HeaderSearch) fetches them — same endpoints, same abandonment rule, same
  // row rendering below. A second implementation of "show me matching jobs" is how the
  // two would drift.
  //
  // These matter MORE now than before, not less: the list below no longer narrows as
  // you type, so these rows are the only live evidence the query finds anything.
  let jobs = $state.raw<Job[]>([]);
  let companies = $state.raw<CompanyListItem[]>([]);

  $effect(() => {
    const q = settledQuery.trim();
    if (q.length < SUGGEST_MIN_CHARS || !suggest) {
      completions = [];
      completionParts = new Map();
      jobs = [];
      companies = [];
      return;
    }
    // One controller per settled query, aborted by this effect's own cleanup. It
    // replaced a stale-response counter, which dropped the ANSWER after the request had
    // already gone out and the server had already worked for it. The signal doubles as
    // the staleness check below: abandoned and stale are the same condition here, so a
    // counter beside it would be a second answer to one question.
    const ac = new AbortController();
    void (async () => {
      // allSettled, not all: the three sections are independent, so one endpoint
      // failing still shows the sections that succeeded instead of blanking all of
      // them. The completions in particular sit behind a dictionary that is rebuilt on
      // a schedule — a cold or missing one must cost the box its completions, not its
      // postings.
      const [s, j, c] = await Promise.allSettled([
        api.suggest(q, completionsLimit, ac.signal),
        api.searchJobs(new URLSearchParams({ q }), jobsLimit, 0, ac.signal),
        // Over-fetch: most of what the fuzzy endpoint returns is discarded below, and
        // asking for exactly three would leave the section empty whenever the fourth
        // was the only real match.
        api.listCompanies(q, companiesFetch, 0, undefined, ac.signal),
      ]);
      // An abort rejects all three, and `allSettled` would otherwise report that as
      // three empty sections — blanking the panel a fresher query is about to fill.
      if (ac.signal.aborted) return;
      const rows = s.status === 'fulfilled' ? s.value : [];
      completions = fromApi(rows);
      completionParts = new Map(completions.map((row, i) => [row.slug, rows[i]?.parts ?? []]));
      jobs = j.status === 'fulfilled' ? j.value.items : [];
      companies =
        c.status === 'fulfilled' ? namedCompanies(c.value.items, q, companiesLimit) : [];
    })();
    return () => ac.abort();
  });

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

  let linkBusy = $state(false);
  // Where the last run stopped, when it stopped anywhere worth showing. Null while
  // nothing has been asked yet — the row then offers to ask.
  let linkStep = $state.raw<LinkIntakeStep | null>(null);

  /** Where a signed-out visitor goes to hand this link in. The link rides along in the
   *  return path so it survives the round trip — otherwise signing in costs them the
   *  paste, and the whole point was that they had it in hand. */
  const signinHref = $derived.by(() => {
    const back = `/my/contributions?url=${encodeURIComponent(link?.url ?? '')}`;
    return `${resolve('/signin')}?returnTo=${encodeURIComponent(back)}`;
  });

  /** Look this link up, and hand it in if we don't have it. See $lib/linkIntake for the
   *  order and why it is that way round. */
  async function activateLink(pasted: PastedJobLink) {
    // One of our own posting pages: the slug is in the path, so asking the API whether
    // we carry a posting we are serving right now would be asking ourselves.
    if (pasted.ownSlug) {
      openPosting(pasted.ownSlug);
      return;
    }
    if (linkBusy) return;
    linkBusy = true;
    linkStep = null;
    const step = await runLinkIntake(pasted.url, {
      find: (u) => api.findJobByUrl(u),
      submit: (u) => api.resolveJobLink(u),
      signedIn: isAuthenticated,
    });
    linkBusy = false;
    // The box may have moved on while the intake was out: a paste over the old text, or
    // the text cleared entirely. The answer is about the URL we asked with, so applying
    // it now would navigate to a posting nobody is asking about any more — the same
    // stale-answer hazard the suggestion fetches guard with their AbortController, held
    // off here by comparing the question instead, because this runs from a click rather
    // than from an effect with a cleanup to hang the abort on.
    if (link?.url !== pasted.url) return;
    if (step.kind === 'open') {
      openPosting(step.slug);
      return;
    }
    linkStep = step;
  }

  /** Go to a posting and leave the box empty behind us: the URL that got us there is
   *  not a query, and leaving it sitting in the header over the vacancy it opened reads
   *  as a search that is still running. */
  function openPosting(slug: string) {
    draft = commit(edit(draft, ''));
    close();
    void goto(resolve('/jobs/[slug]', { slug }));
  }

  const rows = $derived(
    dropdownRows({ suggestions, jobs, companies, text: draft.text, link }),
  );
  const suggestOpen = $derived(rows.length > 0 && !dismissed);
  const rowCount = $derived(suggestOpen ? rows.length : 0);

  /** How much of the screen's bottom the on-screen keyboard is covering, in pixels.
   *
   *  Neither mobile browser shrinks the PAGE for the keyboard on its own — it is drawn
   *  over the bottom of a viewport that stays full height — so the panel below, pinned
   *  from the header to `bottom: 0`, ran under the keys with its last rows unreachable.
   *  Which is the worst place to lose: the visitor has just typed, and the rows they are
   *  reading are the ones the typing produced.
   *
   *  `visualViewport` is the part of the page actually left visible, so the difference
   *  between it and the window is the keyboard. `app.html` also asks Chrome to shrink the
   *  page itself (`interactive-widget=resizes-content`), and the two do not double up:
   *  where the browser honours that, the window shrinks with it and this measures 0.
   *
   *  Held here rather than in a shared store because this panel is the only thing that
   *  reaches the bottom edge today; a second one (a composer, a sheet) is when it earns
   *  a module of its own. */
  let keyboardInset = $state(0);

  $effect(() => {
    // Nothing to lift while the panel is shut, and no listener to keep either.
    if (!suggestOpen) return;
    const viewport = window.visualViewport;
    if (!viewport) return;
    const measure = () => {
      // Only the phone-width panel has a bottom edge to lift: at `sm` and up it hangs off
      // the box and is sized by `max-height`. Read the breakpoint the same way the
      // autofocus check above reads it, so there is one answer to "is this a phone".
      const wide = window.matchMedia('(min-width: 640px)').matches;
      keyboardInset = wide
        ? 0
        : Math.max(0, window.innerHeight - viewport.height - viewport.offsetTop);
    };
    measure();
    // `scroll` as well as `resize`: iOS scrolls the visual viewport inside the layout one
    // to keep the focused field visible, which moves the keyboard's top edge without
    // changing its height.
    viewport.addEventListener('resize', measure);
    viewport.addEventListener('scroll', measure);
    return () => {
      viewport.removeEventListener('resize', measure);
      viewport.removeEventListener('scroll', measure);
      keyboardInset = 0;
    };
  });

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
      void activateLink(link);
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
      void activateLink(row.link);
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

  /** The label above a section's first row, or null for a section that needs none.
   *
   *  An EMPTY box gets none. Its rows are the only thing in the panel — there is no
   *  second section to tell them apart from — so the heading was a line of small caps
   *  introducing a list nobody could confuse with anything. A typed box does have
   *  neighbours (postings, companies), and there "Filter by" says what picking one of
   *  these rows does rather than what it is. */
  function sectionHeading(kind: DropdownRow['kind']): string | null {
    if (kind === 'text' || kind === 'link') return null;
    if (kind === 'job') return 'Jobs';
    if (kind === 'company') return 'Companies';
    return draft.text.trim() === '' ? null : 'Filter by';
  }

  /** The panel a recognised link drops.
   *
   *  It does NOT take the phone's screen the way the suggestions list does. That panel
   *  goes full-bleed because it is a dozen rows that truncate at the box's width; this
   *  one is a sentence, and a sentence stretched from the header to the bottom of the
   *  screen reads as a page that has gone wrong. */
  const linkPanelClass = $derived(
    cn(
      'absolute inset-x-0 top-full z-50 mt-2 border border-border bg-background px-3 py-2.5 text-sm shadow-lg',
      hero ? 'rounded-2xl' : 'rounded-md',
    ),
  );

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
    class={cn(
      'flex items-center border border-border bg-background focus-within:ring-2 focus-within:ring-ring',
      hero
        ? // Taller, rounder and lifted off the page: at the centre of an otherwise
          // empty screen this is the only interactive thing, so it carries the weight
          // the header version borrows from the bar around it.
          'h-14 gap-3 rounded-2xl px-4 text-base shadow-lg shadow-foreground/5 sm:h-16 sm:px-5'
        : // 48px inside the bar's own 56px. The header height is `h-14` in eight other
          // places (`top-14`, `top-20`, `PINNED_HEADER_TOP`, the full-bleed
          // `calc(100dvh-3.5rem)`), so it is the field that grows into the bar rather
          // than the bar that grows.
          'h-12 gap-2 rounded-md px-3 text-sm',
    )}
    onpointerdown={(e) => {
      // The whole box takes the caret, not just the field inside it. Probed at 390px: the
      // field is 175px of a 278px box, and the other 103 — the rule, the magnifier, the
      // `/` hint, the padding — landed on nothing at all. That reads as a search box that
      // ignored the first tap and answered the second, better-aimed one.
      //
      // Only what is NOT itself a control: the scope prefix, the clear button and the
      // All-filters trigger keep their own taps. `preventDefault` because the default for
      // a press on a non-focusable box is to take focus AWAY from the field.
      if ((e.target as HTMLElement | null)?.closest('button, input, a')) return;
      e.preventDefault();
      inputEl?.focus();
    }}
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
      }}
      oninput={(e) => {
        dismissed = false;
        activeIndex = -1;
        // Editing the text asks a different question, so the last link's answer goes
        // with it — otherwise a half-corrected URL sits under a verdict on the old one.
        linkStep = null;
        draft = edit(draft, e.currentTarget.value);
      }}
      onfocus={() => {
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
         modal — or, on a page with no list, the one its host handed us. The badge shows
         the active-filter count, and only a list can have one. Hidden where neither
         exists. -->
    {#if filterTrigger.open}
      <div class="h-5 w-px shrink-0 bg-border"></div>
      <button
        type="button"
        onclick={filterTrigger.open}
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
  <!-- A recognised link takes the panel over. It is not a listbox: there is one thing to
       do and no list to walk, and the states it passes through carry a link of their own
       (sign in, view the company), which cannot live inside an option's button. -->
  {#if link && !dismissed}
    <div class={linkPanelClass} role="status" aria-live="polite">
      {#if linkBusy}
        <span class="flex items-center gap-2 text-muted-foreground">
          <Link2 class="size-4 shrink-0" />
          Checking whether we have this one…
        </span>
      {:else if linkStep?.kind === 'signin'}
        <span class="flex flex-col gap-1">
          <span>We don't have this one yet.</span>
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- query string appended to a resolved path -->
          <a href={signinHref} class="font-medium underline underline-offset-4">
            Sign in to send it to us →
          </a>
        </span>
      {:else if linkStep?.kind === 'outcome'}
        <IntakeOutcome resolved={linkStep.resolved} />
      {:else if linkStep?.kind === 'error'}
        <span class="flex flex-col gap-1">
          <span>That didn't go through.</span>
          <button
            type="button"
            onclick={() => link && activateLink(link)}
            class="self-start font-medium underline underline-offset-4"
          >
            Try again
          </button>
        </span>
      {:else}
        <button
          type="button"
          onclick={() => link && activateLink(link)}
          class="flex w-full items-center gap-2 text-left"
        >
          <Link2 class="size-4 shrink-0 text-muted-foreground" />
          <span class="min-w-0 flex-1 truncate">Open this job link</span>
          <kbd class="hidden shrink-0 rounded border border-border px-1.5 text-xs text-muted-foreground sm:inline">
            ↵
          </kbd>
        </button>
      {/if}
    </div>
  {:else if suggestOpen}
    <ul
      id="role-suggestions"
      role="listbox"
      style:bottom={keyboardInset > 0 ? `${keyboardInset}px` : null}
      aria-label="Search suggestions"
      class={cn(
        'inset-x-0 z-50 max-h-[70vh] overflow-y-auto border border-border bg-background py-1 shadow-lg',
        hero
          ? 'absolute top-full mt-2 rounded-2xl'
          : // On a phone the panel leaves the box and takes the screen: full width, from
            // under the sticky header (`top-14` is that header's own `h-14`) down to
            // `bottom-0`. The box is 278px of a 390px screen, so at its width every row —
            // a logo, a title, a company line — truncates; and a panel that stopped short
            // of the bottom left the feed showing through beneath it, where a scroll moved
            // whichever of the two the finger happened to land on. The side borders and
            // the radius go with the edges they drew.
            //
            // `fixed` rather than an outward margin, because the box does not start at the
            // viewport edge — the brand sits to its left and the menu to its right, so no
            // margin reaches past them. It works only because the header draws no
            // `backdrop-filter` (see TopBar): one would make the header the containing
            // block and pin this panel to it instead of to the window.
            'max-sm:fixed max-sm:bottom-0 max-sm:top-14 max-sm:max-h-none max-sm:border-x-0 max-sm:border-b-0 sm:absolute sm:top-full sm:mt-2 sm:rounded-md',
      )}
    >
      {#each rows as row, i (row.key)}
        {@const heading = row.first ? sectionHeading(row.kind) : null}
        {#if heading}
          <li
            class="px-3 pb-1 pt-2 text-xs font-medium uppercase tracking-wide text-muted-foreground"
          >
            {heading}
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
            {:else if row.kind === 'text'}
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
