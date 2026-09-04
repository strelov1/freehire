<script lang="ts">
  // A single-select list of full-width option buttons — the shape the stage and challenge
  // steps both ask in. Rendered as radios rather than as buttons with aria-pressed: these
  // options are mutually exclusive, and a radiogroup is what tells a screen reader that
  // picking one un-picks the rest. The pill groups elsewhere in the wizard are multi-select
  // and correctly use aria-pressed instead.
  import { Check } from '@lucide/svelte';
  import type { SurveyOption } from '$lib/surveyOptions';

  interface Props {
    options: SurveyOption[];
    value: string | null;
    onSelect: (value: string) => void;
    label: string;
  }

  let { options, value, onSelect, label }: Props = $props();
</script>

<div class="flex flex-col gap-2" role="radiogroup" aria-label={label}>
  {#each options as o (o.value)}
    {@const selected = value === o.value}
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onclick={() => onSelect(o.value)}
      class={[
        'flex w-full items-center justify-between gap-3 rounded-xl border px-4 py-3.5 text-left text-sm transition-colors',
        selected
          ? 'border-brand bg-brand/5 font-medium text-foreground'
          : 'border-border bg-card hover:bg-accent',
      ]}
    >
      <span>{o.label}</span>
      {#if selected}
        <Check class="size-4 shrink-0 text-brand-strong" aria-hidden="true" />
      {/if}
    </button>
  {/each}
</div>
