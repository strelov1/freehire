<script lang="ts">
  import { Check, Loader, Minus } from '@lucide/svelte';
  import { Card } from 'freehire-design-system';
  import type { ApplyPlan, PlanItem } from '../../lib/applyPlan';

  /**
   * The panel's account of the application form in front of the user: what it
   * asks, what is answered, and how far the required questions are along.
   *
   * It reaches the page through nobody — `onReveal` is the one thing it asks for,
   * and App.svelte decides what that means. The plan itself is computed in
   * `lib/applyPlan.ts`, so this file has no arithmetic to get wrong.
   */
  let {
    plan,
    /** The question being filled right now, by label — the walk's cursor. */
    filling = null,
    /** True while a walk is running: the card's own heading takes over, since
     *  that is where the user is looking while the page scrolls past. */
    walking = false,
    onReveal,
    onCancel,
  }: {
    plan: ApplyPlan;
    filling?: string | null;
    walking?: boolean;
    onReveal: (item: PlanItem) => void;
    onCancel: () => void;
  } = $props();

  /** What the icon says, for a reader that cannot see it — the icon itself is
   *  aria-hidden, so without this the state never reaches a screen reader. */
  function itemState(item: PlanItem): string {
    if (filling === item.label) return 'filling now';
    return item.answered ? 'answered' : 'not answered';
  }

  let required = $derived(plan.items.filter((i) => i.required));
  let optional = $derived(plan.items.filter((i) => !i.required));
</script>

{#snippet group(heading: string, items: PlanItem[])}
  {#if items.length > 0}
    <p class="group">{heading}</p>
    <ul>
      {#each items as item (item.key)}
        <li>
          <button
            type="button"
            class="item"
            class:answered={item.answered}
            aria-label="{item.label}: {itemState(item)}"
            onclick={() => onReveal(item)}
          >
            <span class="mark" aria-hidden="true">
              {#if filling === item.label}
                <Loader class="size-3 spin" />
              {:else if item.answered}
                <Check class="size-3" />
              {:else}
                <Minus class="size-3" />
              {/if}
            </span>
            <span class="label">{item.label}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
{/snippet}

<Card class="apply-plan">
  <div class="head">
    <span class="title">{walking ? 'Autofilling…' : 'Application form'}</span>
    {#if plan.required}
      <span class="count">{plan.required.answered}/{plan.required.total} · {plan.required.percent}%</span>
    {/if}
    {#if walking}
      <!-- Cancel belongs beside the progress it cancels, not pinned at the far
           end of the panel: while a walk runs, this card is what the user is
           watching. -->
      <button type="button" class="cancel" onclick={onCancel}>Cancel</button>
    {/if}
  </div>

  {#if plan.required}
    <!-- The bar states the same figure as the count beside it, so it carries no
         label of its own; the count is what a screen reader reads out. -->
    <div class="bar" role="presentation">
      <div class="fill" style="width: {plan.required.percent}%"></div>
    </div>
  {/if}


  {@render group('Required', required)}
  {@render group('Optional', optional)}
</Card>

<style>
  /* Same inset as MatchCard, for the same reason: the panel's own padding is the
   * card's outer margin, so a margin here would double it. */
  :global(.apply-plan) {
    padding: 14px;
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }

  .title {
    font-size: 13px;
    font-weight: 600;
  }

  .cancel {
    border: 0;
    background: none;
    padding: 0;
    font: inherit;
    font-size: 12px;
    color: var(--muted-foreground);
    text-decoration: underline;
    cursor: pointer;
  }

  .cancel:hover {
    color: var(--foreground);
  }

  .count {
    font-size: 12px;
    color: var(--muted-foreground);
    font-variant-numeric: tabular-nums;
  }

  .bar {
    margin-top: 8px;
    height: 6px;
    border-radius: 999px;
    background: var(--muted);
    overflow: hidden;
  }

  .fill {
    height: 100%;
    background: var(--brand);
    transition: width 200ms ease-out;
  }

  .group {
    margin: 12px 0 4px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted-foreground);
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  /* Every item is actionable, answered or not: going back to a question you have
   * already answered is how you check what went into it. */
  .item {
    width: 100%;
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 4px 6px;
    border: 0;
    border-radius: 6px;
    background: none;
    text-align: left;
    font: inherit;
    font-size: 13px;
    color: var(--muted-foreground);
    cursor: pointer;
  }

  .item:hover {
    background: var(--accent);
  }

  .item.answered {
    color: var(--foreground);
  }

  .mark {
    flex: none;
    margin-top: 1px;
    display: flex;
    width: 16px;
    height: 16px;
    align-items: center;
    justify-content: center;
    border-radius: 999px;
    border: 1px solid var(--border);
    color: var(--muted-foreground);
  }

  .item.answered .mark {
    background: var(--brand);
    border-color: var(--brand);
    color: var(--brand-foreground);
  }

  .label {
    min-width: 0;
  }

  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
