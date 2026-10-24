<script lang="ts">
  import { Layers } from '@lucide/svelte';
  import type { Snippet } from 'svelte';
  import { formatCount } from '#lib/utils.js';
  import { resultCountMessage } from './listToolbar.js';

  // The mobile controls for a list page (jobs, companies, …): an inline toolbar at the
  // top of the list — the results total on the left, and (on the jobs list) a Swipe entry
  // on the right. Filters are opened from the header search box's All-filters trigger (see
  // HeaderListSearch), not from here. Mobile-only; the desktop sidebar aside carries filters
  // there, so this shows only the total (right-aligned) at md+. Render it at the top of the
  // list column, outside the view's status branches, so the controls stay reachable while
  // the list is loading/empty/errored.
  //
  // `total` is null until the list is ready (then the count appears); `unit` is the
  // already-pluralised noun ("jobs" / "companies"). `onSwipe` is optional — pass it only
  // where a swipe deck exists (the standalone jobs list). `controls` is an optional slot
  // for the list's own controls — however many the view passes (the jobs feed's sort and
  // freshness selects; the company catalog's sort select) — rendered in the mobile toolbar
  // and beside the desktop total. It shows even when `total` is null so the controls stay
  // reachable while the list is empty or standing in a prompt.
  //
  // `settled` says whether the list has finished loading successfully. Only a caller that
  // passes it can be told apart "no results" from "still loading" — `total` is null for
  // both — so only such a caller gets "No matching …" announced.
  //
  // `announceKey` names the result set the count belongs to — the query (filters,
  // search, sort), never the page number. A caller that passes it gets the count spoken
  // only when the set or its size actually changed: turning a page reloads the list
  // too, and without the key every page link re-announced the same "1,234 jobs".
  let {
    total,
    unit,
    settled,
    announceKey,
    onSwipe,
    controls,
    leading,
  }: {
    total: number | null;
    unit: string;
    settled?: boolean;
    announceKey?: string;
    onSwipe?: () => void;
    controls?: Snippet;
    /** A control pinned to the LEFT edge, apart from the right-aligned list controls
     *  (the jobs list's Map button). Leads the mobile row and the desktop one alike. */
    leading?: Snippet;
  } = $props();

  // What the result count says to a screen reader. The visible counts below come and go
  // with `total` (they are removed while the list reloads), and a live region inserted
  // together with its text is not announced — so the announcement lives in ONE region
  // that is always mounted, and only its text changes.
  const message = $derived(resultCountMessage(total, unit, settled));

  // Empty at mount: the count the page was rendered with is not news, and announcing it
  // on every page load would talk over the page title. Every later change is spoken. The
  // region is cleared while the list reloads, so the same count after a filter change is
  // still a change and is announced again — unless `announceKey` says the reload was the
  // same result set (a page turn), in which case the region keeps its text and only a
  // count that moved is spoken.
  let announcement = $state('');
  let primed = false;
  // The result set and message last spoken (or, at mount, the ones rendered silently).
  let spokenKey: string | undefined;
  let spokenMessage = '';
  $effect(() => {
    const next = message;
    const key = announceKey;
    if (!primed) {
      if (next !== '' || settled !== undefined) {
        primed = true;
        spokenKey = key;
        spokenMessage = next;
      }
      return;
    }
    const sameSet = key !== undefined && key === spokenKey;
    if (next === '') {
      if (!sameSet) announcement = '';
      return;
    }
    if (sameSet && next === spokenMessage) return;
    announcement = next;
    spokenKey = key;
    spokenMessage = next;
  });
</script>

<!-- Mobile inline toolbar: total on the left, controls on the right. The Swipe entry is
     icon-only here (labelled for a11y) so the row stays on one line with the count and the
     list controls; the word would crowd it out on a narrow phone.

     `flex-wrap` is the safety net under that: the controls are sized by their content
     (a long count, a translated label, a fourth control) and a row that runs out of width
     must break onto a second line rather than clip its rightmost control off-screen. -->
<span class="sr-only" role="status" aria-live="polite" aria-atomic="true">{announcement}</span>

<div class="mb-3 flex flex-wrap items-center gap-2 md:hidden">
  {@render leading?.()}
  {#if total !== null}
    <!-- Rounded on this row, spelled out on the desktop one below. The row is 358px on a
         390px phone and its contents came to 376: the count at full precision was 103px of
         that and "2.1M" is 60, which is what puts the controls back on one line. The exact
         figure stays in the title. -->
    <span
      class="shrink-0 whitespace-nowrap text-sm text-muted-foreground"
      title="{total.toLocaleString()} {unit}"
    >
      <span class="font-semibold tabular-nums text-foreground">{formatCount(total)}</span>
      {unit}
    </span>
  {/if}
  <!-- Wraps too, not just the outer row. Without this the controls are one indivisible
       block: the row can drop the whole block to a second line but the block itself
       still overflows, so a longer control (a translated select value, a fourth entry)
       clips exactly as before. `justify-end` keeps a wrapped line right-aligned under
       the `ml-auto`. -->
  <div class="ml-auto flex flex-wrap items-center justify-end gap-2">
    {@render controls?.()}
    {#if onSwipe}
      <button
        type="button"
        onclick={onSwipe}
        aria-label="Swipe mode"
        title="Swipe mode"
        class="inline-flex items-center rounded-lg border border-border bg-card px-2.5 py-2 text-sm font-medium transition-colors hover:bg-accent"
      >
        <Layers class="size-4 shrink-0" />
      </button>
    {/if}
  </div>
</div>

<!-- Desktop: the total (and any list controls) sit top-right above the list (filters
     live in the sidebar). The two are gated INDEPENDENTLY — a list standing in a prompt
     has no total but must keep its controls reachable. -->
{#if total !== null || controls || leading}
  <div class="mb-3 hidden items-center justify-end gap-3 md:flex">
    {#if leading}<div class="mr-auto">{@render leading()}</div>{/if}
    {#if total !== null}
      <span class="text-sm text-muted-foreground">
        <span class="font-semibold tabular-nums text-foreground">{total.toLocaleString()}</span>
        {unit}
      </span>
    {/if}
    {@render controls?.()}
  </div>
{/if}
