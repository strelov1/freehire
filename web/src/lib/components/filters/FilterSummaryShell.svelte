<script module lang="ts">
  export interface SummaryChip {
    text: string;
    /** Excluded value — rendered in the destructive (struck-through) style. */
    exclude: boolean;
    remove: () => void;
  }
  export interface SummaryGroup {
    label: string;
    chips: SummaryChip[];
  }
</script>

<script lang="ts">
  import { SlidersHorizontal, X } from '@lucide/svelte';

  // The reusable filter-summary sidebar chrome: heading + Reset all, the All-filters
  // button (with active badge), the empty state, and the applied-filter chip groups.
  // A domain summary (FilterSummary, CompanyFilterSummary) computes `groups` from its
  // store and passes the counts/handlers; removing a chip applies immediately (each
  // chip owns its `remove`).
  let {
    groups,
    active,
    onReset,
    onOpen,
  }: {
    groups: SummaryGroup[];
    active: number;
    onReset: () => void;
    onOpen: () => void;
  } = $props();
</script>

<div class="flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-base font-semibold tracking-tight">Filters</h2>
    {#if active > 0}
      <button type="button" class="text-xs text-muted-foreground transition-colors hover:text-foreground" onclick={onReset}>Reset all</button>
    {/if}
  </div>

  <button
    type="button"
    onclick={onOpen}
    class="flex h-11 w-full items-center justify-center gap-2 rounded-xl border border-border bg-background text-sm font-medium transition-colors hover:bg-accent"
  >
    <SlidersHorizontal class="size-4" />
    <span>All filters</span>
    {#if active > 0}
      <span class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1.5 text-[11px] font-semibold text-primary-foreground">{active}</span>
    {/if}
  </button>

  {#if groups.length === 0}
    <div class="rounded-xl border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
      No filters yet.<br />Open <b>All filters</b> to refine.
    </div>
  {:else}
    {#each groups as g (g.label)}
      <div class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{g.label}</span>
        <div class="flex flex-wrap gap-1.5">
          {#each g.chips as c (c.text)}
            <span
              class={[
                'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium',
                c.exclude ? 'border-destructive/30 bg-destructive/15 text-destructive line-through' : 'border-border bg-secondary text-secondary-foreground',
              ]}
            >
              {c.text}
              <button type="button" aria-label="Remove {c.text}" onclick={c.remove} class="text-muted-foreground transition-colors hover:text-foreground">
                <X class="size-3" />
              </button>
            </span>
          {/each}
        </div>
      </div>
    {/each}
  {/if}
</div>
