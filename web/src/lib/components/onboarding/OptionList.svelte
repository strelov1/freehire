<script lang="ts">
  // A single-select list of full-width option buttons — the shape the stage and challenge
  // steps both ask in. Rendered as radios rather than as buttons with aria-pressed: these
  // options are mutually exclusive, and a radiogroup is what tells a screen reader that
  // picking one un-picks the rest. The pill groups elsewhere in the wizard are multi-select
  // and correctly use aria-pressed instead.
  import { Check } from '@lucide/svelte';
  import type { SurveyOption } from '#lib/surveyOptions.js';

  interface Props {
    options: SurveyOption[];
    value: string | null;
    onSelect: (value: string) => void;
    label: string;
  }

  let { options, value, onSelect, label }: Props = $props();

  // A radiogroup is ONE tab stop, the platform's own radio behaviour: Tab lands on the
  // checked option (or the first, when none is), and the arrows move between options and
  // select as they go. Selecting here only records the answer — the wizard never advances
  // on it — so selection following focus costs nothing.
  const tabbable = $derived(options.some((o) => o.value === value) ? value : (options[0]?.value ?? null));

  let group: HTMLDivElement | undefined = $state();

  function onkeydown(e: KeyboardEvent) {
    const current = options.findIndex((o) => o.value === (e.currentTarget as HTMLElement).dataset.value);
    if (current < 0) return;
    let next: number;
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowRight':
        next = (current + 1) % options.length;
        break;
      case 'ArrowUp':
      case 'ArrowLeft':
        next = (current - 1 + options.length) % options.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = options.length - 1;
        break;
      default:
        return;
    }
    const target = options[next];
    if (!target) return;
    e.preventDefault();
    onSelect(target.value);
    group?.querySelectorAll<HTMLElement>('[role="radio"]')[next]?.focus();
  }
</script>

<div class="flex flex-col gap-2" role="radiogroup" aria-label={label} bind:this={group}>
  {#each options as o (o.value)}
    {@const selected = value === o.value}
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      tabindex={o.value === tabbable ? 0 : -1}
      data-value={o.value}
      onclick={() => onSelect(o.value)}
      {onkeydown}
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
