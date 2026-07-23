<script lang="ts">
  // Page-margin steppers for the CV editor: four per-side controls (in inches) that mutate the
  // caller-owned Document margins. Each step is clamped and rounded by stepMargin (unit-tested in
  // $lib/tailor/geometry), so this component is layout only. Editing a value flows straight back to
  // the bound margins, so the centre preview re-paginates live and autosave persists the change.
  import { Minus, Plus } from '@lucide/svelte';
  import type { Margins } from '$lib/generated/contracts';
  import { stepMargin, MARGIN_STEP } from '$lib/tailor/geometry';

  let { margins = $bindable() }: { margins: Margins } = $props();

  const sides: { key: keyof Margins; label: string }[] = [
    { key: 'left', label: 'Left' },
    { key: 'right', label: 'Right' },
    { key: 'top', label: 'Top' },
    { key: 'bottom', label: 'Bottom' },
  ];

  const bump = (key: keyof Margins, delta: number) => () => {
    margins[key] = stepMargin(margins[key], delta);
  };
</script>

<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
  {#each sides as { key, label } (key)}
    <div class="space-y-1">
      <p class="text-sm font-medium">{label}</p>
      <div class="flex items-center rounded-lg border border-input">
        <button
          type="button"
          aria-label="Decrease {label} margin"
          onclick={bump(key, -MARGIN_STEP)}
          class="grid h-9 w-9 shrink-0 place-items-center rounded-l-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <Minus class="h-4 w-4" />
        </button>
        <span class="flex-1 text-center text-sm tabular-nums" aria-label="{label} margin">{margins[key].toFixed(2)}</span>
        <button
          type="button"
          aria-label="Increase {label} margin"
          onclick={bump(key, MARGIN_STEP)}
          class="grid h-9 w-9 shrink-0 place-items-center rounded-r-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <Plus class="h-4 w-4" />
        </button>
      </div>
    </div>
  {/each}
</div>
