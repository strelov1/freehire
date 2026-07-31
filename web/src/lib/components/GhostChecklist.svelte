<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check, ChevronDown } from '@lucide/svelte';
  import { ghostBadge, ghostChecklist, ghostGauge, ghostUnobserved } from '$lib/ghost';
  import type { GhostGaugeTone } from '$lib/ghost';
  import type { Ghost } from '$lib/generated/contracts';
  import { cn } from '$lib/ui';

  // One row for the whole signal: a gauge, the hedged wording, the fired/total scale,
  // and the justification behind a disclosure.
  //
  // The justification is the point of the feature rather than decoration — a posting
  // flagged on two structural criteria with no outcome data is a very different thing
  // from one several people reported, and that difference is what separates informing
  // somebody from accusing an employer at them. But it ran fourteen lines, half of them
  // reading "no data", and it was the loudest thing on the page after the title.
  // Collapsed, the ceiling on the claim still shows: the wording says the posting MAY
  // be inactive, and the scale says how much of the evidence is missing.
  let { ghost }: { ghost?: Ghost | null } = $props();

  const badge = $derived(ghostBadge(ghost));
  const gauge = $derived(ghostGauge(ghost));
  const rows = $derived(ghost && badge ? ghostChecklist(ghost) : []);
  const fired = $derived(rows.filter((r) => r.fired));
  const unobserved = $derived(ghost && badge ? ghostUnobserved(ghost) : '');

  let open = $state(false);

  // Per-instance, not a constant. One call site renders one row today, and a
  // hardcoded id is a bug waiting for the second: two rows on a page would both claim
  // it, and each button's aria-controls would resolve to the first one's criteria.
  const criteriaId = $props.id();

  // The escalation ladder. An unfilled segment is drawn in the neutral border tone and
  // never a pale green: the payload says a criterion did not fire, never that it was
  // checked and found clear, and a reassuring colour would claim that difference.
  const filledTone: Record<GhostGaugeTone, string> = {
    low: 'bg-yellow-400 dark:bg-yellow-300',
    elevated: 'bg-warning',
    high: 'bg-orange-600 dark:bg-orange-500',
    severe: 'bg-red-600 dark:bg-red-500',
  };

  const textTone: Record<'warn' | 'muted', string> = {
    warn: 'text-warning-strong',
    muted: 'text-muted-foreground',
  };
</script>

{#if badge && rows.length}
  <section class="flex flex-col gap-2">
    <button
      type="button"
      onclick={() => (open = !open)}
      aria-expanded={open}
      aria-controls={criteriaId}
      class="group flex w-full items-center gap-2.5 rounded-md text-left text-xs font-medium"
    >
      {#if gauge}
        <!-- Decorative: the wording and the scale beside it carry the same information
             as text, so a screen reader gains nothing from four unlabelled bars. -->
        <span class="flex shrink-0 items-center gap-0.5" aria-hidden="true">
          {#each { length: gauge.segments }, i (i)}
            <span
              class={cn(
                'h-3 w-1.5 rounded-sm',
                i < gauge.filled ? filledTone[gauge.tone] : 'bg-muted-foreground/30',
              )}
            ></span>
          {/each}
        </span>
      {/if}

      <span class={textTone[badge.tone]}>{badge.label}</span>
      <span class="tabular-nums text-muted-foreground">{badge.scale}</span>

      <!-- Beside the scale, not pushed to the far edge: the button spans the row so the
           whole line is a hit area, but a control a thousand pixels from the thing it
           expands stops reading as belonging to it. -->
      <span
        class="inline-flex shrink-0 items-center gap-1 text-muted-foreground group-hover:text-foreground"
      >
        Details
        <ChevronDown
          class={cn('size-3.5 transition-transform', open && 'rotate-180')}
          aria-hidden="true"
        />
      </span>
    </button>

    <!-- Always mounted, toggled between `flex` and `hidden`: `aria-controls` above
         names this element, and unmounting it would leave that IDREF pointing at
         nothing in the collapsed state — which is every first paint. `hidden` takes it
         out of the accessibility tree and the tab order just as `{#if}` did.
         Tailwind's `hidden` utility rather than the attribute, because the `flex`
         class would otherwise win over the attribute's `display: none`.

         A rule rather than a box: the disclosure is already grouped by the row above
         it, and a second border inside the page's own bordered column was the frame
         that made this read as a warning panel. -->
    <div
      id={criteriaId}
      class={cn(
        'flex-col gap-1.5 border-l border-border pl-3 text-sm',
        open ? 'flex' : 'hidden',
      )}
    >
      <!-- A list, so a screen reader still announces how many criteria fired — a count
           the gauge conveys visually and `aria-hidden` withholds. -->
      <ul class="flex flex-col gap-1.5">
        {#each fired as row (row.code)}
          <li class="flex items-baseline gap-2">
            <Check
              class="size-3.5 shrink-0 translate-y-0.5 text-warning-strong"
              aria-hidden="true"
            />
            <!-- The separator is glued to the fact with a non-breaking space: Svelte
                 trims leading whitespace inside an element, and a bare "·" would also
                 be free to wrap onto a line of its own. -->
            <span
              >{row.label}{#if row.detail}<span class="text-muted-foreground"
                  >&nbsp;· {row.detail}</span
                >{/if}</span
            >
          </li>
        {/each}
      </ul>

      <!-- The unfired criteria collapse into one line. Naming them is what tells the
           reader why the level is not higher, but four rows each reading "No data" said
           that four times over — and said it wrongly, which is why the wording and its
           reasoning now live in ghostUnobserved. -->
      {#if unobserved}
        <p class="text-muted-foreground">{unobserved}</p>
      {/if}

      <a
        class="mt-1 w-fit text-xs text-muted-foreground underline decoration-border hover:text-foreground"
        href={resolve('/features/ghost-jobs')}
      >
        How this works
      </a>
    </div>
  </section>
{/if}
