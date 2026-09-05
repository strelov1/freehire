<script lang="ts">
  // The Level control: which seniorities the candidate will take, as a plain two-state
  // multi-select. Shared by the onboarding wizard's Confirm step and the profile's Role
  // card — they asked the same question with two hand-written copies of this markup, and
  // two copies of a control is how the wizard and the page it feeds start to disagree.
  //
  // Deliberately NOT facets/PillGroup: that is a three-state FILTER pill (include /
  // exclude / off, option groups, result counts, unknown-value passthrough) and none of
  // it applies to stating your own level. It also carries a different look, which is why
  // this keeps the bordered one both callers already showed rather than quietly
  // restyling two live surfaces.
  import { SENIORITY_OPTIONS } from '$lib/facets';

  let {
    selected,
    onToggle,
    busy = false,
  }: {
    selected: string[];
    onToggle: (value: string) => void;
    /** Dims and blocks the group while a write is in flight. The wizard stages its answer
     *  locally and never sets it; the profile card autosaves and does. */
    busy?: boolean;
  } = $props();
</script>

<div class={['flex flex-wrap gap-2', busy && 'pointer-events-none opacity-60']}>
  {#each SENIORITY_OPTIONS as o (o.value)}
    {@const isSelected = selected.includes(o.value)}
    <button
      type="button"
      onclick={() => onToggle(o.value)}
      aria-pressed={isSelected}
      class={[
        'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors',
        isSelected ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
      ]}
    >
      {o.label}
    </button>
  {/each}
</div>
