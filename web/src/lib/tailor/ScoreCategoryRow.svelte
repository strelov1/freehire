<script lang="ts">
  // One scoring category as a disclosure: label, its value, a note beside it (how much the
  // row can move the score, or why it could not be scored), and — expanded — the individual
  // checks behind it. Shared by the Job Match tab and the Score tab, which carry different
  // numbers but read the same way; the caller supplies the value's wording and tone, since
  // only it knows whether "up" means a rise or a high share.
  //
  // A row with no items is not a button: a disclosure that opens onto nothing trains the
  // candidate that the chevrons are decorative.
  import { ChevronDown, Check, TriangleAlert, X } from '@lucide/svelte';
  import type { LineItem } from '$lib/generated/contracts';

  let {
    label,
    value = '',
    valueClass = 'text-muted-foreground',
    note = '',
    items = [],
    id,
  }: {
    label: string;
    value?: string;
    valueClass?: string;
    note?: string;
    items?: LineItem[];
    /** Stable id for the aria-controls IDREF. */
    id: string;
  } = $props();

  let open = $state(false);
  const expandable = $derived(items.length > 0);

  const itemTone: Record<string, string> = {
    pass: 'text-emerald-600 dark:text-emerald-400',
    warn: 'text-amber-600 dark:text-amber-400',
    fail: 'text-destructive',
  };
</script>

<div class="border-b border-border/60 last:border-b-0">
  <svelte:element
    this={expandable ? 'button' : 'div'}
    type={expandable ? 'button' : undefined}
    onclick={expandable ? () => (open = !open) : undefined}
    aria-expanded={expandable ? open : undefined}
    aria-controls={expandable ? id : undefined}
    class={[
      'group flex w-full items-start gap-2 py-2 text-left',
      expandable && 'cursor-pointer',
    ]}
  >
    {#if expandable}
      <ChevronDown
        class={['mt-0.5 size-3.5 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180']}
        aria-hidden="true"
      />
    {:else}
      <span class="mt-0.5 size-3.5 shrink-0" aria-hidden="true"></span>
    {/if}

    <span class="min-w-0 flex-1">
      <span class="block text-sm font-medium text-foreground">{label}</span>
      {#if note}
        <span class="block text-xs text-muted-foreground">{note}</span>
      {/if}
    </span>

    {#if value}
      <span class="shrink-0 text-sm font-semibold tabular-nums {valueClass}">{value}</span>
    {/if}
  </svelte:element>

  <!-- Always mounted, toggled between `block` and `hidden`: aria-controls above names this
       element, and unmounting it would leave that IDREF pointing at nothing in the collapsed
       state — which is every first paint. -->
  <ul {id} class={['space-y-1.5 pb-2 pl-5.5 text-xs', open ? 'block' : 'hidden']}>
    {#each items as item, i (i)}
      <li class="flex items-start gap-1.5">
        {#if item.status === 'pass'}
          <Check class="mt-0.5 size-3.5 shrink-0 {itemTone.pass}" aria-hidden="true" />
        {:else if item.status === 'fail'}
          <X class="mt-0.5 size-3.5 shrink-0 {itemTone.fail}" aria-hidden="true" />
        {:else}
          <TriangleAlert class="mt-0.5 size-3.5 shrink-0 {itemTone.warn}" aria-hidden="true" />
        {/if}
        <span class="min-w-0 flex-1 text-muted-foreground">{item.text}</span>
        <span class="shrink-0 tabular-nums text-muted-foreground/70">{item.points}</span>
      </li>
    {/each}
  </ul>
</div>
