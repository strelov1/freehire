<script lang="ts">
  // Page margins for the CV editor, in inches. The default view links each axis — side margins,
  // top-and-bottom — because uniform margins are the common case and four steppers abreast do not
  // fit the workspace panel at its narrow end; the independent per-side steppers stay one
  // disclosure away. Clamping and rounding live in the unit-tested helpers in $lib/tailor/geometry,
  // so this file is layout only. Editing flows straight back to the bound margins, so the centre
  // preview re-paginates live and autosave persists the change.
  import { ChevronRight } from '@lucide/svelte';
  import type { Margins } from '$lib/generated/contracts';
  import { stepMargin, stepAxis, axisValue, MARGIN_STEP, type MarginAxis } from '$lib/tailor/geometry';
  import { SettingRow } from '$lib/ui';
  import Stepper from './Stepper.svelte';

  let { margins = $bindable() }: { margins: Margins } = $props();

  let perSide = $state(false);

  const axes: { key: MarginAxis; label: string }[] = [
    { key: 'sides', label: 'Side margins' },
    { key: 'ends', label: 'Top & bottom' },
  ];
  const sides: { key: keyof Margins; label: string }[] = [
    { key: 'left', label: 'Left' },
    { key: 'right', label: 'Right' },
    { key: 'top', label: 'Top' },
    { key: 'bottom', label: 'Bottom' },
  ];

  // An axis whose two sides differ has no single value to show. Saying so — rather than
  // displaying one side — is what keeps the linked stepper honest about the asymmetry it is
  // about to shift rather than level.
  const shown = (key: MarginAxis) => {
    const v = axisValue(margins, key);
    return v === null ? '—' : v.toFixed(2);
  };
</script>

<div class="space-y-1">
  {#if perSide}
    {#each sides as { key, label } (key)}
      <SettingRow {label}>
        {#snippet control()}
          <Stepper
            display={margins[key].toFixed(2)}
            label="{label} margin"
            onstep={(d) => (margins[key] = stepMargin(margins[key], d * MARGIN_STEP))}
          />
        {/snippet}
      </SettingRow>
    {/each}
  {:else}
    {#each axes as { key, label } (key)}
      <SettingRow {label} hint={axisValue(margins, key) === null ? 'Sides differ' : undefined}>
        {#snippet control()}
          <Stepper
            display={shown(key)}
            muted={axisValue(margins, key) === null}
            {label}
            onstep={(d) => (margins = stepAxis(margins, key, d * MARGIN_STEP))}
          />
        {/snippet}
      </SettingRow>
    {/each}
  {/if}

  <button
    type="button"
    onclick={() => (perSide = !perSide)}
    aria-expanded={perSide}
    class="flex items-center gap-1 pt-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
  >
    <ChevronRight class={['size-3.5 transition-transform', perSide && 'rotate-90']} />
    {perSide ? 'Link the margins' : 'Set each side separately'}
  </button>
</div>
