<script module lang="ts">
  export interface SummaryChip {
    /** Identity for the keyed `{#each}`, unique by construction — `param:value` for a
     *  facet value, a literal for the one-off chips (Search, Salary, Visa…). NOT the
     *  label: two different values can share one, and since the skills facet started
     *  labelling from the dictionary, `nextjs` and `next.js` both read "Next.js" — a
     *  duplicate key, which Svelte answers by tearing down the whole block. */
    key: string;
    text: string;
    /** Excluded value — rendered in the destructive (struck-through) style. */
    exclude: boolean;
    remove: () => void;
    /** Skill dictionary slug, when this chip is a `skills` facet value — shows its
     *  brand logo (via SkillIcon) beside the label, same as everywhere else a skill
     *  chip renders. Unset for every other facet's chips. */
    slug?: string;
  }
  export interface SummaryGroup {
    label: string;
    chips: SummaryChip[];
  }
</script>

<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { SlidersHorizontal, X } from '@lucide/svelte';
  import SkillIcon from '../SkillIcon.svelte';

  // The reusable filter-summary sidebar chrome: heading + Clear all, the All-filters
  // button (with active badge), the empty state, and the applied-filter chip groups.
  // A domain summary (FilterSummary, CompanyFilterSummary) computes `groups` from its
  // store and passes the counts/handlers; removing a chip applies immediately (each
  // chip owns its `remove`).
  let {
    groups,
    active,
    onReset,
    onOpen,
    description,
    afterButton,
  }: {
    groups: SummaryGroup[];
    active: number;
    onReset: () => void;
    onOpen: () => void;
    /** Optional caption under the "Filters" heading, explaining what refining does in
     *  this context — the Market coverage tab uses it; other summaries omit it. */
    description?: string;
    /** Optional content under the All-filters button — the job summary uses it for the
     *  "Save filter" affordance; the company summary omits it. */
    afterButton?: Snippet;
  } = $props();

  let root = $state<HTMLDivElement>();
  let heading = $state<HTMLHeadingElement>();

  // Every control here that removes something removes ITSELF: the chip's ×, a group's
  // clear, Clear all. Left alone, focus falls back to <body> and a keyboard user is sent
  // to the top of the page after each chip. So focus moves to whatever now stands where
  // the removed control stood — the next chip slides into its place, else the last one
  // left — and to the heading once no chip remains.
  //
  // `from` is the removed control's position among the chip buttons, read BEFORE the
  // removal; a group clear passes its first chip's, which the next group's first chip
  // then takes over.
  async function removeThenRefocus(remove: () => void, from: number | null) {
    remove();
    await tick();
    const left = chipButtons();
    const next = from === null ? undefined : left[Math.min(from, left.length - 1)];
    if (next) next.focus();
    else heading?.focus();
  }

  function chipButtons(): HTMLButtonElement[] {
    return root ? [...root.querySelectorAll<HTMLButtonElement>('[data-chip-remove]')] : [];
  }

  function chipIndex(el: EventTarget | null): number | null {
    if (!(el instanceof HTMLButtonElement)) return null;
    const i = chipButtons().indexOf(el);
    return i === -1 ? null : i;
  }

  // The chip's × and a group's clear share one 24px hit area around a 12px glyph.
  const removeButtonClass =
    'relative inline-flex text-muted-foreground transition-colors after:absolute after:-inset-1.5 hover:text-foreground';
</script>

<div class="flex flex-col gap-4" bind:this={root}>
  <div class="flex flex-col gap-1">
    <div class="flex items-center justify-between">
      <!-- Focusable from script only: where focus lands once the control holding it has
           removed itself and no chip is left (see `removeThenRefocus`). -->
      <h2 bind:this={heading} tabindex="-1" class="text-base font-semibold tracking-tight outline-none">Filters</h2>
      {#if active > 0}
        <button type="button" class="text-xs text-muted-foreground transition-colors hover:text-foreground" onclick={() => removeThenRefocus(onReset, null)}>Clear all</button>
      {/if}
    </div>
    {#if description}
      <p class="text-xs text-muted-foreground">{description}</p>
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
      <span class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-brand px-1.5 text-[11px] font-semibold text-brand-foreground">{active}</span>
    {/if}
  </button>

  {@render afterButton?.()}

  {#if groups.length === 0}
    <div class="rounded-xl border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
      No filters yet.<br />Open <b>All filters</b> to refine.
    </div>
  {:else}
    {#each groups as g (g.label)}
      <div class="flex flex-col gap-2">
        <div class="flex items-center gap-1">
          <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{g.label}</span>
          <button
            type="button"
            aria-label="Clear {g.label}"
            onclick={(e) => {
              const first = e.currentTarget.parentElement?.nextElementSibling?.querySelector('[data-chip-remove]') ?? null;
              removeThenRefocus(() => g.chips.forEach((c) => c.remove()), chipIndex(first));
            }}
            class={removeButtonClass}
          >
            <X class="size-3" aria-hidden="true" />
          </button>
        </div>
        <div class="flex flex-wrap gap-1.5">
          {#each g.chips as c (c.key)}
            <span
              class={[
                'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium',
                c.exclude ? 'border-destructive/30 bg-destructive/15 text-destructive-strong line-through' : 'border-transparent bg-brand-muted text-brand-strong',
              ]}
            >
              {#if c.slug}<SkillIcon slug={c.slug} />{/if}
              <!-- The strike-through is the only sign of an excluded value and a screen
                   reader does not voice it, so the state is said in words as well — in
                   the chip and in its remove button, which otherwise read the same as an
                   included value's. -->
              {#if c.exclude}<span class="sr-only">Excluded:</span>{/if}
              {c.text}
              <button
                type="button"
                data-chip-remove
                aria-label={c.exclude ? `Remove excluded ${c.text}` : `Remove ${c.text}`}
                onclick={(e) => removeThenRefocus(c.remove, chipIndex(e.currentTarget))}
                class={removeButtonClass}
              >
                <X class="size-3" aria-hidden="true" />
              </button>
            </span>
          {/each}
        </div>
      </div>
    {/each}
  {/if}
</div>
