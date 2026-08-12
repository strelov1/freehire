<script lang="ts">
  import { CONVERGENCE, WITNESS_GATE } from '$lib/ghost';
  import { gateMatrix } from '$lib/ghostDiagrams';
  import { cn } from '$lib/ui';
  import { must } from '$lib/utils';

  // Two gates, four outcomes. The cell wording is asked of the rule rather than typed
  // here, so the diagram cannot caption a level the classifier stopped producing.
  //
  // What the picture is for: the cell where posting shape converges and nobody has
  // witnessed anything sits one square away from the strongest claim and cannot reach it,
  // whatever else the employer does to the posting. That is the feature's central limit,
  // and it survives as geometry where it did not survive as a clause in a sentence.
  const cells = gateMatrix();

  const WORDING: Record<string, string> = {
    none: 'nothing shown',
    possible: 'Possibly inactive',
    likely: 'Likely inactive',
  };
</script>

<!-- Table and its payoff side by side. Stacked, they were two more blocks in a column
     that already ran four deep while the right half of the page sat empty. -->
<div class="flex flex-col gap-6 lg:flex-row lg:items-center lg:gap-10">
  <div class="w-full max-w-xl shrink-0">
  <!-- A real table, not a grid of divs: this is two axes crossed, and a screen reader
       given nine unrelated cells in source order gets none of that. `scope` is what ties
       "nothing shown" back to "under 2 criteria" and "under 2 people". -->
  <table
    class="w-full border-separate border-spacing-0 overflow-hidden rounded-lg border border-border text-sm"
  >
    <caption class="sr-only">
      The ghost level for each combination of the convergence and witness gates
    </caption>
    <thead>
      <tr>
        <td class="border-b border-border p-3"></td>
        <th
          scope="col"
          class="border-b border-l border-border p-3 text-left text-xs font-normal text-muted-foreground"
        >
          under {CONVERGENCE} criteria
        </th>
        <th
          scope="col"
          class="border-b border-l border-border p-3 text-left text-xs font-normal text-muted-foreground"
        >
          {CONVERGENCE}+ criteria fired
        </th>
      </tr>
    </thead>
    <tbody>
      {#each [false, true] as witnessed (witnessed)}
        <tr>
          <th
            scope="row"
            class={cn(
              'p-3 text-left text-xs font-normal text-muted-foreground',
              !witnessed && 'border-b border-border',
            )}
          >
            {witnessed ? `${WITNESS_GATE}+ people` : `under ${WITNESS_GATE} people`}
          </th>
          {#each [false, true] as converged (converged)}
            {@const cell = must(
              cells.find((c) => c.converged === converged && c.witnessed === witnessed),
            )}
            <td
              class={cn(
                'border-l border-border p-3',
                !witnessed && 'border-b',
                cell.level === 'likely' && 'text-warning-strong',
                cell.level === 'none' && 'text-muted-foreground',
              )}
            >
              {WORDING[cell.level]}
            </td>
          {/each}
        </tr>
      {/each}
    </tbody>
    </table>
  </div>

  <p class="max-w-sm text-sm leading-relaxed text-muted-foreground">
    The posting itself can only get you into the right-hand column. The bottom row needs
    something only people who applied can give — which is why no amount of the first kind
    ever reaches the strongest wording.
  </p>
</div>
