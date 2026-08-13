<script lang="ts">
  // Typography for the CV editor: typeface, base type size, and line height. Every control
  // offers "Template default", which clears the value — an unset value is what tells the
  // renderer to use the active template's own, and it is the reason switching templates still
  // moves whatever the candidate has not overridden.
  //
  // The font list comes from the API rather than a constant here: the server's registry is what
  // the PDF obeys, and a second copy in TypeScript would drift from it. Clamping and stepping
  // live in the unit-tested helpers in $lib/tailor/geometry.
  import { RotateCcw } from '@lucide/svelte';
  import type { Style } from '$lib/generated/contracts';
  import type { CvFont } from '$lib/cv';
  import { stepFontSize, FONT_SIZE_STEP, TEMPLATE_FONT_SIZE_PT } from '$lib/tailor/geometry';
  import { SettingRow } from '$lib/ui';
  import Stepper from './Stepper.svelte';

  let { style = $bindable(), fonts = [] }: { style: Style; fonts?: CvFont[] } = $props();

  // Named presets, not a number. The stored value is the Typst leading in em, which means
  // nothing to a candidate, and a ratio would be false precision about something they are
  // choosing by eye against a live preview. 0.5 is what four of the six templates already use.
  const LINE_HEIGHTS: { value: number; label: string }[] = [
    { value: 0, label: 'Template default' },
    { value: 0.4, label: 'Compact' },
    { value: 0.5, label: 'Standard' },
    { value: 0.65, label: 'Relaxed' },
    { value: 0.8, label: 'Loose' },
  ];

  const selectClass =
    'w-full min-w-[9rem] rounded-lg border border-input bg-background px-2 py-1.5 text-sm text-foreground';

  const sizeShown = $derived(
    (style.font_size ?? 0) > 0 ? (style.font_size ?? 0).toFixed(1) : TEMPLATE_FONT_SIZE_PT.toFixed(1),
  );
  const sizeIsDefault = $derived(!((style.font_size ?? 0) > 0));

  // Sanitize accepts any leading in [0.3, 0.9], so a CLI or API client can store one that
  // matches no preset — and a <select> whose value matches no option renders blank while the
  // value quietly persists. Surfacing it as its own option keeps the control honest and lets
  // the candidate move off it. Same reason the margin axes show an em dash when sides differ.
  const lineHeightOptions = $derived.by(() => {
    const v = style.line_height ?? 0;
    if (v === 0 || LINE_HEIGHTS.some((lh) => lh.value === v)) return LINE_HEIGHTS;
    return [...LINE_HEIGHTS, { value: v, label: `Custom (${v.toFixed(2)})` }];
  });

  const isPristine = $derived(
    !style.font_family && !((style.font_size ?? 0) > 0) && !((style.line_height ?? 0) > 0),
  );

  function reset() {
    style.font_family = '';
    style.font_size = 0;
    style.line_height = 0;
  }
</script>

<div class="space-y-1">
  <SettingRow label="Font" grow>
    {#snippet control()}
      <select bind:value={style.font_family} class={selectClass} aria-label="Font">
        <option value="">Template default</option>
        {#each fonts as f (f.id)}
          <option value={f.id}>{f.label}{f.note ? ` — ${f.note}` : ''}</option>
        {/each}
      </select>
    {/snippet}
  </SettingRow>

  <SettingRow label="Font size" hint={sizeIsDefault ? 'From the template' : 'points'}>
    {#snippet control()}
      <Stepper
        display={sizeShown}
        muted={sizeIsDefault}
        label="Font size"
        onstep={(d) => (style.font_size = stepFontSize(style.font_size ?? 0, d * FONT_SIZE_STEP))}
      />
    {/snippet}
  </SettingRow>

  <SettingRow label="Line height" grow>
    {#snippet control()}
      <select bind:value={style.line_height} class={selectClass} aria-label="Line height">
        {#each lineHeightOptions as lh (lh.value)}
          <option value={lh.value}>{lh.label}</option>
        {/each}
      </select>
    {/snippet}
  </SettingRow>

  <button
    type="button"
    onclick={reset}
    disabled={isPristine}
    class="flex items-center gap-1 pt-1 text-xs text-muted-foreground transition-colors hover:text-foreground disabled:opacity-40 disabled:hover:text-muted-foreground"
  >
    <RotateCcw class="size-3.5" />
    Reset to template default
  </button>
</div>
