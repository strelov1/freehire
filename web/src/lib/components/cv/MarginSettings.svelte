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
  import { messages } from './MarginSettings.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  let { margins = $bindable() }: { margins: Margins } = $props();

  let perSide = $state(false);

  const s = $derived(t(messages, locale()));

  const axes = $derived<{ key: MarginAxis; label: string }[]>([
    { key: 'sides', label: s.axes.sides },
    { key: 'ends', label: s.axes.ends },
  ]);
  const sides = $derived<{ key: keyof Margins; row: string; stepper: string }[]>([
    { key: 'left', row: s.sides.left.row, stepper: s.sides.left.stepper },
    { key: 'right', row: s.sides.right.row, stepper: s.sides.right.stepper },
    { key: 'top', row: s.sides.top.row, stepper: s.sides.top.stepper },
    { key: 'bottom', row: s.sides.bottom.row, stepper: s.sides.bottom.stepper },
  ]);

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
    {#each sides as side (side.key)}
      <SettingRow label={side.row}>
        {#snippet control()}
          <Stepper
            display={margins[side.key].toFixed(2)}
            label={side.stepper}
            onstep={(d) => (margins[side.key] = stepMargin(margins[side.key], d * MARGIN_STEP))}
          />
        {/snippet}
      </SettingRow>
    {/each}
  {:else}
    {#each axes as { key, label } (key)}
      <SettingRow {label} hint={axisValue(margins, key) === null ? s.sidesDiffer : undefined}>
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
    {perSide ? s.linkMargins : s.setEachSide}
  </button>
</div>
