<script lang="ts">
  import { CONVERGENCE, WITNESS_GATE } from '$lib/ghost';
  import { gateMatrix } from '$lib/ghostDiagrams';
  import { cn } from '$lib/ui';

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

<div class="w-full max-w-2xl">
  <div class="grid grid-cols-[auto_1fr_1fr] gap-px overflow-hidden rounded-lg border border-border bg-border text-sm">
    <div class="bg-background p-3"></div>
    <div class="bg-background p-3 text-xs text-muted-foreground">
      under {CONVERGENCE} criteria
    </div>
    <div class="bg-background p-3 text-xs text-muted-foreground">
      {CONVERGENCE}+ criteria fired
    </div>

    {#each [false, true] as witnessed (witnessed)}
      <div class="flex items-center bg-background p-3 text-xs text-muted-foreground">
        {witnessed ? `${WITNESS_GATE}+ people` : `under ${WITNESS_GATE} people`}
      </div>
      {#each [false, true] as converged (converged)}
        {@const cell = cells.find((c) => c.converged === converged && c.witnessed === witnessed)!}
        <div
          class={cn(
            'bg-background p-3',
            cell.level === 'likely' && 'text-warning-strong',
            cell.level === 'none' && 'text-muted-foreground',
          )}
        >
          {WORDING[cell.level]}
        </div>
      {/each}
    {/each}
  </div>

  <p class="mt-3 max-w-xl text-sm leading-relaxed text-muted-foreground">
    Posting shape can fill the right-hand column, and nothing more. The bottom row needs
    something only people who applied can supply, which is why no quantity of the first
    kind ever produces the strongest wording.
  </p>
</div>
