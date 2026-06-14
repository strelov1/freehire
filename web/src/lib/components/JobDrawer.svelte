<script lang="ts">
  import { Button } from '$lib/ui';
  import { STAGES, humanizeStage } from '$lib/stages';
  import { CLOSED_OUTCOMES, type ClosedOutcome } from '$lib/board';
  import { timeAgo } from '$lib/utils';
  import type { MyJob } from '$lib/types';

  let {
    item,
    pendingOutcome,
    onsetstage,
    onsavenotes,
    onchooseoutcome,
    onremove,
    onclose,
  }: {
    item: MyJob;
    pendingOutcome: boolean;
    onsetstage: (stage: string) => void;
    onsavenotes: (notes: string) => void;
    onchooseoutcome: (o: ClosedOutcome) => void;
    onremove: () => void;
    onclose: () => void;
  } = $props();

  // Derive the notes string reactively so the local draft stays in sync when
  // the parent swaps to a different item. $derived keeps the reference live.
  let notes = $state('');
  const _notesSync = $derived(item.notes ?? '');
  $effect(() => {
    notes = _notesSync;
  });
</script>

<div
  class="fixed inset-0 z-40 bg-black/30"
  role="button"
  tabindex="-1"
  aria-label="Close"
  onclick={onclose}
  onkeydown={(e) => e.key === 'Escape' && onclose()}
></div>

<aside
  class="fixed z-50 flex flex-col gap-4 overflow-y-auto bg-card p-5 shadow-xl
         inset-x-0 bottom-0 max-h-[85vh] rounded-t-2xl
         sm:inset-y-0 sm:right-0 sm:left-auto sm:w-96 sm:max-h-none sm:rounded-none"
  aria-label="Job details"
>
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <p class="text-sm text-muted-foreground">{item.job.company || 'Unknown company'}</p>
      <h2 class="text-lg font-semibold tracking-tight">{item.job.title}</h2>
    </div>
    <button type="button" onclick={onclose} class="text-muted-foreground hover:text-foreground" aria-label="Close">✕</button>
  </div>

  <a href={`/jobs/${item.job.public_slug}`} class="text-sm font-medium underline underline-offset-4">Open job →</a>

  {#if pendingOutcome}
    <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
      <p class="text-sm font-medium">How did it close?</p>
      <div class="flex flex-wrap gap-2">
        {#each CLOSED_OUTCOMES as o (o)}
          <Button variant="outline" onclick={() => onchooseoutcome(o)}>{humanizeStage(o)}</Button>
        {/each}
      </div>
    </div>
  {/if}

  <label class="flex flex-col gap-1 text-sm">
    <span class="font-medium">Stage</span>
    <select
      value={item.stage ?? ''}
      onchange={(e) => onsetstage(e.currentTarget.value)}
      class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm"
    >
      <option value="">No stage</option>
      {#each STAGES as s (s.value)}
        <option value={s.value}>{s.label}</option>
      {/each}
    </select>
  </label>

  <label class="flex flex-col gap-1 text-sm">
    <span class="font-medium">Notes</span>
    <textarea
      bind:value={notes}
      onblur={() => onsavenotes(notes)}
      placeholder="Notes…"
      rows="5"
      class="resize-y rounded-md border border-input bg-transparent px-2 py-1.5 text-sm placeholder:text-muted-foreground focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    ></textarea>
  </label>

  <div class="flex flex-col gap-1 text-xs text-muted-foreground">
    {#if item.saved_at}<span>Saved {timeAgo(item.saved_at)}</span>{/if}
    {#if item.applied_at}<span>Applied {timeAgo(item.applied_at)}</span>{/if}
    <span>Viewed {timeAgo(item.viewed_at)}</span>
  </div>

  <Button variant="outline" onclick={onremove}>Remove from board</Button>
</aside>
