<script lang="ts">
  import { CircleAlert, Search, X as XIcon } from '@lucide/svelte';
  import { cn } from '#lib/ui/index.js';
  import { BOARD_COLUMNS, parseBoardSort, type BoardColumnId, type BoardSort } from '#lib/board.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import type { UrlSyncedState } from '#lib/urlSynced.svelte.js';
  import { messages } from './JobBoard.messages';

  // The tracking board's toolbar: the search over both views, "needs attention", the sort,
  // and the stage (the visible column on the board, a filter on the list).
  let {
    searchInput = $bindable(),
    filters,
    searching,
    onClearSearch,
    view,
    visibleColumn,
    onGoToColumn,
    columnLabel,
    counts,
    filtering,
    matched,
    total,
  }: {
    searchInput: HTMLInputElement | undefined;
    filters: UrlSyncedState<{ q: string; attention: boolean; stage: string; sort: BoardSort }>;
    searching: boolean;
    onClearSearch: () => void;
    view: 'board' | 'list';
    visibleColumn: BoardColumnId;
    onGoToColumn: (id: string) => void;
    columnLabel: (col: { id: BoardColumnId; label: string }) => string;
    counts: Record<string, number>;
    filtering: boolean;
    matched: number;
    total: number;
  } = $props();

  const s = $derived(t(messages, locale()));
  const query = $derived(filters.value.q);
  const attentionOnly = $derived(filters.value.attention);
  const sort = $derived(filters.value.sort);
  const stageFilter = $derived(view === 'list' ? filters.value.stage : '');
</script>

<!-- One field over both views. It narrows what is already loaded: the listing is
     bounded, and a request per keystroke would buy nothing. -->
<div class="mb-3 flex flex-wrap items-center gap-2">
  <div class="relative w-full sm:max-w-xs sm:flex-1">
    <Search
      class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
      aria-hidden="true"
    />
    <input
      bind:this={searchInput}
      type="search"
      value={query}
      oninput={(e) => filters.setNow({ ...filters.value, q: e.currentTarget.value })}
      placeholder={s.searchPlaceholder}
      aria-label={s.searchAria}
      class="h-10 w-full rounded-md border border-input bg-transparent pl-8 pr-10 text-base focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:h-9 sm:text-sm [&::-webkit-search-cancel-button]:appearance-none"
    />
    {#if searching}
      <button
        type="button"
        onclick={onClearSearch}
        aria-label={s.clearSearchAria}
        class="absolute right-0 top-0 flex size-10 items-center justify-center rounded text-muted-foreground hover:text-foreground sm:size-9"
      >
        <XIcon class="size-3.5" />
      </button>
    {/if}
  </div>
  <div class="flex w-full items-center gap-2 sm:w-auto">
    <button
      type="button"
      aria-label={s.needsAttention}
      title={s.needsAttention}
      aria-pressed={attentionOnly}
      onclick={() => filters.setNow({ ...filters.value, attention: !attentionOnly })}
      class={cn(
        'inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors',
        attentionOnly
          ? 'bg-warning-muted text-warning-strong'
          : 'text-muted-foreground hover:bg-accent hover:text-foreground',
      )}
    >
      <CircleAlert class="size-4 shrink-0" aria-hidden="true" />
      <span class="sr-only sm:not-sr-only">{s.needsAttention}</span>
    </button>
    <span class="relative flex min-w-0 flex-1 items-center sm:w-40 sm:flex-none">
      <select
        value={sort}
        onchange={(e) => filters.setNow({ ...filters.value, sort: parseBoardSort(e.currentTarget.value) })}
        aria-label={s.sortAria}
        class="h-8 w-full min-w-0 rounded-lg border border-input bg-background pl-2.5 text-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:text-sm"
      >
        <option value="default">{s.sortDefault}</option>
        <option value="company">{s.sortCompany}</option>
        <option value="added">{s.sortAdded}</option>
        <option value="deadline">{s.sortDeadline}</option>
      </select>
    </span>
    {#if view === 'board'}
      <span class="relative flex min-w-0 flex-1 items-center sm:w-40 sm:flex-none">
        <select
          value={visibleColumn}
          onchange={(e) => onGoToColumn(e.currentTarget.value)}
          aria-label={s.boardStage}
          class="h-8 w-full min-w-0 rounded-lg border border-input bg-background pl-2.5 text-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:text-sm"
        >
          {#each BOARD_COLUMNS as col (col.id)}
            <option value={col.id}>{columnLabel(col)} ({counts[col.id]})</option>
          {/each}
        </select>
      </span>
    {:else}
      <span class="relative flex min-w-0 flex-1 items-center sm:w-40 sm:flex-none">
        <select
          value={stageFilter}
          onchange={(e) => filters.setNow({ ...filters.value, stage: e.currentTarget.value })}
          aria-label={s.stageFilterAria}
          class="h-8 w-full min-w-0 rounded-lg border border-input bg-background pl-2.5 text-xs focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:text-sm"
        >
          <option value="">{s.allStages}</option>
          {#each BOARD_COLUMNS as col (col.id)}<option value={col.id}>{columnLabel(col)}</option>{/each}
        </select>
      </span>
    {/if}
  </div>
  {#if filtering}
    <span role="status" class="text-xs tabular-nums text-muted-foreground"
      >{format(s.matchedOfTotal, { matched: String(matched), total: String(total) })}</span
    >
  {/if}
</div>
