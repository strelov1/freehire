<script lang="ts">
  import { sortOptionsFor, type FilterStore, type JobSort } from '#lib/filters.js';
  import { freshnessOptions } from '#lib/filterControls.js';

  // JobsView's own list controls: the date bound and the sort order.
  let {
    filters,
    selectedSort,
    openWithinAvailable,
  }: { filters: FilterStore; selectedSort: JobSort; openWithinAvailable: boolean } = $props();

  // Match is offered to everyone. It used to need the same profile the match SLIDER
  // does, which meant the one ordering that answers "which of these actually suit me"
  // was invisible to every visitor who had not filled a profile in — and nothing told
  // them it existed. Choosing it without skills on file is now answered out loud
  // instead, by the notice below the toolbar.
  const sortOptions = $derived(sortOptionsFor(filters.value.q));
  const openBound = $derived(openWithinAvailable);
</script>

<!-- The list's own controls, handed to ListToolbar so they sit in the shared toolbar
     (mobile) / above the list (desktop) — same shape as the company catalog's
     sortSelect. Each is reachable without opening the filter modal, which is the whole
     point: the endpoint has always served these orderings and bounds, and every one of
     them used to require either a profile, a modal, or a hand-edited URL.

     The URL params are NOT cleared when a control disappears — the server degrades an
     ordering it cannot serve rather than refusing it, so a shared match link stays
     intact and starts working the moment its opener signs in. -->
<!-- A date bound, the same one a modal slider drags. This is the control the complaint
     that prompted it asked for: years-old postings in the feed, with the only way to
     bound them buried in the modal's third section.

     WHICH date it bounds is the flag's decision, and there is deliberately only ever
     one select here. Once `created_ts` is in the index it bounds how long the posting
     has been open, because that is the question this control's placement implies and
     the one the source-stated date was quietly failing: a board that restates its date
     every crawl passes a three-day bound while its own card reads "Open 72d". Until
     then it bounds the source's date exactly as before — never absent, so there is no
     state in which the reader loses the ability to bound the feed by date at all. The
     other bound stays reachable in the modal either way. -->
<label class="flex shrink-0 items-center gap-1.5 text-sm text-muted-foreground">
  <span class="hidden sm:inline">{openBound ? 'Open' : 'Posted'}</span>
  <select
    aria-label={openBound ? 'Open within' : 'Posted within'}
    class="rounded-lg border border-input bg-transparent py-2 pl-2 text-sm max-sm:max-w-20 text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:py-1 dark:bg-input/30"
    value={String((openBound ? filters.value.openWithinDays : filters.value.postedWithinDays) ?? '')}
    onchange={(e) => {
      const days = Number(e.currentTarget.value) || null;
      if (openBound) filters.pickOpenWithinDays(days);
      else filters.pickPostedWithinDays(days);
    }}
  >
    {#each freshnessOptions(openBound ? filters.value.openWithinDays : filters.value.postedWithinDays) as preset (preset.label)}
      <option value={String(preset.days ?? '')}>{preset.label}</option>
    {/each}
  </select>
</label>

<!-- Unconditional. This used to be guarded against a one-option select - a label
     wearing a control's clothes - which could happen when match was hidden from a
     viewer with no profile. Every caller is now offered newest, most-viewed and match,
     so the guard could never be false. -->
<label class="flex shrink-0 items-center gap-1.5 text-sm text-muted-foreground">
  <span class="hidden sm:inline">Sort</span>
  <select
    aria-label="Sort jobs"
    class="rounded-lg border border-input bg-transparent py-2 pl-2 text-sm max-sm:max-w-24 text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:py-1 dark:bg-input/30"
    value={selectedSort}
    onchange={(e) => filters.setSort(e.currentTarget.value as JobSort)}
  >
    {#each sortOptions as opt (opt.value)}
      <option value={opt.value}>{opt.label}</option>
    {/each}
  </select>
</label>
