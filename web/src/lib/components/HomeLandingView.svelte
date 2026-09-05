<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ArrowRight } from '@lucide/svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import HeaderSearch from './HeaderSearch.svelte';
  import { rolePlaceholders } from '$lib/placeholderRoles';
  import { api } from '$lib/api';
  import { browseQuery, planForSuggestion } from '$lib/browseTarget';
  import type { ApplyPlan } from '$lib/apiSuggestions';
  import { countryLabel } from '$lib/facets';
  import { emptyFilters, filtersToParams } from '$lib/facetModel';
  import type { StagedFilters } from '$lib/stagedFilters.svelte';
  import { CLI_INSTALL, CLI_REPO } from '$lib/cliLinks';
  import { EXTENSION_CLAIMS, EXTENSION_STORE_URL } from '$lib/extensionLinks';
  import { starterSuggestions } from '$lib/suggestions';
  import { Button, SectionLabel } from '$lib/ui';
  import type { CatalogScale, FacetCounts } from '$lib/types';

  // The homepage. One search box on an otherwise empty screen, and under it — for the
  // visitor who came to look rather than to search — how big the catalogue is and the
  // two places freehire runs that are not this page.
  //
  // The box is the header's own search component at hero size. On a page with no list
  // registered, that component already drives a target that turns every pick into a
  // link to the feed (see browseTarget) — so the dropdown, its three sections, the
  // keyboard walk, the composed picks and the hotkeys all arrive here unchanged, and
  // the landing owns none of that behaviour. The header renders no box on this route
  // (see TopBar): one screen, one field.
  //
  // Everything below the fold is a POINTER, not a second copy of a page that exists:
  // /open owns the numbers, /cli owns the CLI, /features/extension owns the extension.
  // Each section here says the one sentence that makes someone click, and the copy is
  // lifted verbatim from the page it leads to rather than written again.
  let {
    counts,
    scale,
  }: {
    /** The unfiltered category and country distributions, or null when the call
     *  failed. Feeds both rows of chips below and — handed to the box — the empty
     *  dropdown's starting points, so the two cannot disagree about what the catalogue
     *  holds. */
    counts: FacetCounts | null;
    /** How big the catalogue is, or null when the call failed. */
    scale: CatalogScale | null;
  } = $props();

  /** How many shortcuts each row draws. The dropdown offers ten crafts; six and three
   *  are what fit on ONE line each at 1280px, measured rather than guessed — every chip
   *  carries a six-digit count, so eight of them wrapped to three lines and pushed the
   *  catalogue's own figures off the first screen. Those figures are the argument this
   *  page makes; the shortcuts are a convenience, so the shortcuts give way. */
  const CHIP_LIMIT = 6;
  const COUNTRY_CHIP_LIMIT = 3;

  type Chip = { href: string; label: string; count?: number };

  /** Where a chip goes, serialized by the same `browseQuery` a pick in the dropdown
   *  navigates through — so the link and the control that offers it cannot come to
   *  filter differently. A chip always names exactly one facet, so the query is never
   *  empty and needs no fallback to the bare feed.
   *
   *  Rendered as a real `<a href>` rather than a click handler: these are the
   *  homepage's only outgoing links into the catalogue, so they are what a crawler
   *  follows to reach the feed at all. */
  function planHref(plan: ApplyPlan): string {
    return `${resolve('/jobs')}?${browseQuery(plan)}`;
  }

  function feedHref(param: string, value: string): string {
    return planHref({ facets: [[param, value]] });
  }

  /** What, rather than where: the starting points in the curated group order the filter
   *  modal uses — Engineering first, the consumer industries last. Built by the same
   *  function the dropdown's empty state uses, from the same distribution, and turned
   *  into a link by the same `planForSuggestion` the dropdown applies — so which facet
   *  a craft means is answered in one place, not two. */
  const crafts = $derived<Chip[]>(
    starterSuggestions(counts, CHIP_LIMIT).map((s) => ({
      href: planHref(planForSuggestion(s)),
      label: s.label,
      count: s.count,
    })),
  );

  /** Where, rather than what. Remote leads because it is the answer that is not a place;
   *  behind it the busiest countries the catalogue actually carries — measured, not
   *  listed, so this row cannot offer a country with nothing in it.
   *
   *  Unlike the crafts there is no curated order to honour: a country's only claim on
   *  the row is how much work is in it. */
  const places = $derived<Chip[]>([
    { href: feedHref('work_mode', 'remote'), label: 'Remote' },
    ...Object.entries(counts?.facets?.countries ?? {})
      .sort(([, a], [, b]) => b - a)
      .slice(0, COUNTRY_CHIP_LIMIT)
      .map(([code, count]) => ({
        href: feedHref('countries', code),
        label: countryLabel(code),
        count,
      })),
  ]);

  type Figure = { value: string; label: string };

  /** One catalogue figure, or nothing at all. A cold stats snapshot answers with an
   *  estimated open-job count and zeroes the figures that exist only in the database,
   *  so a zero here is an ABSENT measurement — and a homepage that says "0 companies"
   *  is worse than one that says nothing. Same rule /open applies to the same
   *  snapshot. */
  function figure(value: number | undefined, label: string): Figure | null {
    if (!value) return null;
    return { value: value.toLocaleString('en-US'), label };
  }

  const figures = $derived(
    [
      figure(scale?.open_jobs, 'open jobs'),
      figure(scale?.companies, 'companies'),
      figure(scale?.sources, 'sources'),
      figure(scale?.ats_platforms, 'ATS platforms'),
    ].filter((f) => f !== null),
  );

  /** The All-filters modal, hosted here rather than by a list — there is no list.
   *
   *  Everywhere else this modal NARROWS what is already on screen, so it writes into
   *  that page's URL-synced store as you pick. Here it COMPOSES a search: it is seeded
   *  empty, edits nothing, and its footer hands back the staged selection, which
   *  becomes the query the feed opens with. `seed` and `onApply` are the modal's own
   *  provisions for exactly this — the same pair the profile editor uses. */
  let filtersOpen = $state(false);

  /** The distribution behind the modal's controls, recomputed as the selection moves.
   *
   *  Not optional decoration: a DYNAMIC facet — Skills, City, Job language, Source —
   *  has no compiled option list at all. FacetSection builds its entire list from this
   *  map (see its `def.dynamic` branch), so without it those four panes are empty
   *  selects with nothing to pick. The page's own `counts` cannot serve: the SSR call
   *  asks for `category` and `countries` alone, which is all the chips need.
   *
   *  `disjunctive` so each facet's numbers ignore its own selection — the same call
   *  every other host of this modal makes. */
  const stagedCounts = (params: URLSearchParams) =>
    api.facetCounts(params, { disjunctive: true });

  /** How many postings the staged selection would open, for the footer's Apply — read
   *  off the same count call rather than a one-row search, the way /analytics does. */
  const previewCount = (params: URLSearchParams) =>
    api.facetCounts(params).then((c) => c.total);

  function searchWithFilters(staged: StagedFilters) {
    filtersOpen = false;
    const query = filtersToParams(staged.value).toString();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query appended to a resolved path
    void goto(query ? `${resolve('/jobs')}?${query}` : resolve('/jobs'));
  }

  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  async function copyInstall() {
    try {
      await navigator.clipboard.writeText(CLI_INSTALL);
      copied = true;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 1600);
    } catch {
      // Clipboard can be blocked (no permission, insecure context) — the command is
      // plainly visible to select by hand, so a failed copy needs no fallback.
    }
  }
</script>

<!-- One row of shortcuts into the catalogue. Two rows use it — the crafts and the
     places — and they differ only in what they list, so they are one shape. -->
{#snippet chipRow(label: string, chips: Chip[])}
  <nav aria-label={label} class="flex flex-wrap justify-center gap-2">
    {#each chips as chip (chip.href)}
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- feedHref resolves /jobs and appends the serialized filter -->
      <a href={chip.href}
        class="rounded-full border border-border px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:border-brand-strong hover:text-foreground"
      >
        {chip.label}
        {#if chip.count !== undefined}
          <span class="ml-1 text-xs tabular-nums opacity-60"
            >{chip.count.toLocaleString('en-US')}</span
          >
        {/if}
      </a>
    {/each}
  </nav>
{/snippet}

<!-- The one link shape this page uses twice: an onward link with an arrow that leans
     when you point at it. Written once so the two cannot drift into being two
     different-looking ways of saying "there is more of this over there". -->
{#snippet onward(href: string, text: string)}
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- every caller passes a resolve() result; the rule cannot see through a snippet parameter -->
  <a {href}
    class="group inline-flex items-center gap-1.5 text-sm font-medium text-brand-strong transition-colors hover:text-foreground"
  >
    {text}
    <ArrowRight class="size-4 transition-transform group-hover:translate-x-0.5" />
  </a>
{/snippet}

<!-- Hero. Deliberately shorter than the viewport: `-19rem` is the 3.5rem header plus
     the room the figures below need, so how big the catalogue is lands on the FIRST
     screen rather than under it. The number is the argument — a stranger deciding
     whether to type anything into the box is answering "does this place even have my
     job", and making them scroll to find out asks them to trust it first.

     `svh` rather than `vh`: on mobile Safari `100vh` counts browser chrome that is not
     there, so the box it centres would sit noticeably below the middle of the screen.

     `dot-grid` is the shared backdrop from app.css that every landing hero here wears.
     `dot-grid-centred` (also in app.css) is its one variant: the same grid, the same
     fade from the top, moved to the horizontal centre because this hero is not a left
     column beside a visual. -->
<section
  class="dot-grid dot-grid-centred -mx-4 flex min-h-[calc(100svh-19rem)] flex-col items-center justify-center px-4 pb-10 pt-10"
>
  <div class="flex w-full max-w-2xl flex-col items-center gap-6">
    <!-- No mark or wordmark here: the header carries both, three centimetres above and
         on the same screen. Repeating them made the page introduce itself twice before
         it said anything. -->
    <div class="flex flex-col items-center gap-5 text-center">
      <h1 class="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
        Every tech job, straight from the source.
      </h1>
      <p class="text-balance text-base leading-relaxed text-muted-foreground">
        Indexed from company career boards, deduplicated, and tagged by stack, seniority
        and location. Free, open source, no walls.
      </p>
    </div>

    <!-- The box's root is `min-w-0 flex-1`, so this wrapper is what sets its width. -->
    <div class="flex w-full">
      <!-- The visible examples are roles; the accessible name is the broad one, because
           this box does search companies and skills too. An example is a starting point
           rather than a boundary, and a role is what people actually type first. -->
      <HeaderSearch
        placeholder={rolePlaceholders()}
        label="Search jobs, companies and skills"
        size="hero"
        autofocus
        {counts}
        onOpenFilters={() => (filtersOpen = true)}
      />
    </div>

    {#if crafts.length > 0}
      {@render chipRow('Browse by specialization', crafts)}
    {/if}
    {#if places.length > 1}
      <!-- Only with real countries behind it: Remote alone is a row that looks like a
           measurement and is a constant. -->
      {@render chipRow('Browse by location', places)}
    {/if}

    {@render onward(resolve('/jobs'), 'Browse the whole catalogue')}
  </div>
</section>

{#if figures.length > 0}
  <!-- How big it is. The figures come from the one catalogue-scale snapshot /open
       reads, so the two pages cannot quote numbers measured at different moments —
       and a figure the snapshot could not measure is dropped rather than shown as a
       zero. `tabular-nums` keeps the row from twitching as the counts move. -->
  <section class="border-t border-border py-10">
    <SectionLabel text="the catalogue, today" />
    <dl class="mt-6 grid grid-cols-2 gap-x-6 gap-y-6 sm:grid-cols-4">
      {#each figures as f (f.label)}
        <!-- `flex-col-reverse` so the number reads first and the label sits under it,
             while the DOM keeps the term before its description. The label is written
             once and is its own <dt>: an sr-only copy plus an aria-label plus a visible
             span was three spellings of one word, and each could go stale alone. -->
        <div class="flex flex-col-reverse gap-1">
          <dt class="text-sm text-muted-foreground">{f.label}</dt>
          <dd class="text-3xl font-semibold tabular-nums tracking-tight sm:text-4xl">
            {f.value}
          </dd>
        </div>
      {/each}
    </dl>
    <div class="mt-6">
      {@render onward(resolve('/open'), 'Every number, live, with the endpoint behind it')}
    </div>
  </section>
{/if}

<!-- The two places freehire runs that are not a web page. Side by side because they
     are the same offer twice — the catalogue where you already work — and stacking
     them would make the second read as an afterthought. Each card carries its own
     evidence: the CLI its install line, the extension what the panel actually does. -->
<section class="border-t border-border py-12 sm:py-16">
  <SectionLabel text="take it with you" />
  <!-- `grid-cols-1` is load-bearing, not the default written out: with no column template
       the single track below `lg` is `auto`, which sizes to its widest content — the
       install line in the `pre`, which does not wrap. That took the track to 416px on a
       390px phone and gave the whole page a horizontal scrollbar. Tailwind's `grid-cols-*`
       are `repeat(N, minmax(0, 1fr))` for exactly this, and the `pre` scrolls itself. -->
  <div class="mt-8 grid grid-cols-1 gap-6 lg:grid-cols-2">
    <div class="flex flex-col gap-5 rounded-xl border border-border p-6 sm:p-8">
      <div class="flex flex-col gap-3">
        <h2 class="text-2xl font-semibold tracking-tight">
          Search and track from the terminal.
        </h2>
        <p class="text-sm leading-relaxed text-muted-foreground">
          One binary, no Go needed. Ships agent skills too, so Claude Code and any MCP
          host can run the search, read the facets and move an application along.
        </p>
      </div>

      <figure class="overflow-hidden rounded-lg border border-border bg-secondary/40 font-mono text-xs">
        <!-- The figure carries `font-mono text-xs`, so nothing inside restates it. -->
        <figcaption
          class="flex items-center gap-2 border-b border-border px-3 py-1.5 text-muted-foreground"
        >
          install
          <button
            type="button"
            onclick={copyInstall}
            class="ml-auto rounded-md border border-border px-2 py-0.5 font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            {copied ? 'copied ✓' : 'copy'}
          </button>
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed">{CLI_INSTALL}</pre>
      </figure>

      <div class="mt-auto flex flex-wrap items-center gap-3">
        <Button href={resolve('/cli')} variant="outline" size="md">What it does</Button>
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- absolute GitHub URL from $lib/cliLinks (shared with the CLI page's JSON-LD codeRepository), not a SvelteKit route -->
        <a href={CLI_REPO}
          target="_blank"
          rel="noopener noreferrer"
          class="text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          Source
        </a>
      </div>
    </div>

    <div class="flex flex-col gap-5 rounded-xl border border-border p-6 sm:p-8">
      <div class="flex flex-col gap-3">
        <h2 class="text-2xl font-semibold tracking-tight">Apply where you already are.</h2>
        <p class="text-sm leading-relaxed text-muted-foreground">
          A job-application agent in Chrome's side panel, next to the posting you are
          reading — on any site, not just the ones freehire tracks.
        </p>
      </div>

      <ul class="list-disc space-y-2 pl-5 text-sm text-muted-foreground">
        {#each EXTENSION_CLAIMS as claim (claim)}
          <li>{claim}</li>
        {/each}
      </ul>

      <div class="mt-auto flex flex-wrap items-center gap-3">
        <Button href={EXTENSION_STORE_URL} target="_blank" variant="primary" size="md">
          Add to Chrome
        </Button>
        <a
          href={resolve('/features/extension')}
          class="text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          How it works
        </a>
      </div>
    </div>
  </div>
</section>

<!-- Seeded empty and applied by navigating: see `searchWithFilters`. `savedSearches`
     is off — that tab manages the alerts a signed-in visitor saved from a list, and
     this modal has no list behind it. `counts` seeds the controls before the first
     staged fetch answers; `stagedCounts` is what keeps them right, and what gives the
     dynamic facets any options at all. -->
<FilterModal
  seed={emptyFilters()}
  {counts}
  {stagedCounts}
  {previewCount}
  open={filtersOpen}
  onClose={() => (filtersOpen = false)}
  applyLabel="Search jobs"
  onApply={searchWithFilters}
/>
