<script lang="ts">
  import { sectionHeading, type DropdownRow } from '#lib/dropdownRows.js';
  import type { KeyboardFit } from '#lib/keyboardFit.js';
  import { cn } from '#lib/ui/index.js';
  import HeaderSearchRow from './HeaderSearchRow.svelte';

  // HeaderSearch's suggestion listbox. The input keeps the keyboard (aria-activedescendant);
  // this list only renders the rows and takes their pointer events.
  let {
    rows,
    activeIndex = $bindable(),
    hero,
    fit,
    emptyQuery,
    onChoose,
  }: {
    rows: DropdownRow[];
    activeIndex: number;
    hero: boolean;
    /** Where the panel may sit with the on-screen keyboard up (see trackKeyboardFit). */
    fit: KeyboardFit;
    emptyQuery: boolean;
    onChoose: (index: number) => void;
  } = $props();

  const rowClass = (active: boolean) =>
    cn(
      'flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors',
      active ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50',
    );
</script>

<ul
  id="role-suggestions"
  role="listbox"
  style:bottom={fit.lift > 0 ? `${fit.lift}px` : null}
  style:max-height={fit.ceiling > 0 ? `${fit.ceiling}px` : null}
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
    {@const heading = row.first ? sectionHeading(row, emptyQuery) : null}
    {#if heading}
      <!-- A caption, not a choice: presentation keeps it out of the option count while
           its text still reads in place. -->
      <li role="presentation" class="px-3 pb-1 pt-2 text-xs font-medium text-muted-foreground">
        {heading}
      </li>
    {/if}
    <!-- The option itself takes the click — a button inside it would be a second
         interactive element nested in the first. tabindex -1 keeps rows out of the Tab
         order; the input owns the keyboard through aria-activedescendant. -->
    <li
      role="option"
      id="role-suggestion-{i}"
      aria-selected={activeIndex === i}
      tabindex="-1"
      onmouseenter={() => (activeIndex = i)}
      onclick={() => onChoose(i)}
      onkeydown={(e) => {
        // Reached only if a row ever held focus itself; the input normally handles keys.
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onChoose(i);
        }
      }}
      class={cn(rowClass(activeIndex === i), 'cursor-pointer', row.kind === 'text' && 'border-t border-border')}
    >
      <HeaderSearchRow {row} />
    </li>
  {/each}
</ul>
