<script lang="ts">
  import { CLOSED_OUTCOMES, type ClosedOutcome } from '#lib/board.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { groupedStages, humanizeStage } from '#lib/stages.js';
  import { Button } from '#lib/ui/index.js';
  import NoteEditor from './NoteEditor.svelte';
  import { messages } from './JobDrawer.messages';

  // The drawer's Application tab: how a closed application ended, its stage and its notes.
  let {
    stage,
    notes,
    pendingOutcome,
    onchooseoutcome,
    onsetstage,
    onsavenotes,
  }: {
    stage: string;
    notes: string;
    pendingOutcome: boolean;
    onchooseoutcome: (o: ClosedOutcome) => void;
    onsetstage: (stage: string) => void;
    onsavenotes: (notes: string) => Promise<void>;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

<div class="flex flex-col gap-4">
  {#if pendingOutcome}
    <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
      <p class="text-sm font-medium">{s.howDidItClose}</p>
      <div class="flex flex-wrap gap-2">
        {#each CLOSED_OUTCOMES as o (o)}
          <Button variant="outline" onclick={() => onchooseoutcome(o)}>{humanizeStage(o, locale())}</Button>
        {/each}
      </div>
    </div>
  {/if}

  <label class="flex flex-col gap-1 text-sm">
    <span class="font-medium">{s.stage}</span>
    <select
      value={stage}
      onchange={(e) => onsetstage(e.currentTarget.value)}
      class="min-h-11 rounded-md border border-input bg-transparent px-2 py-1.5 text-base sm:min-h-0 sm:text-sm"
    >
      <option value="">{s.noStage}</option>
      <!-- Grouped so `Closed` reads as a heading over its three outcomes rather than
           as a fifth state competing with them — the same four groups the board's
           columns use, from the same generated table. -->
      {#each groupedStages(locale()) as g (g.id)}
        <optgroup label={g.label}>
          {#each g.options as s (s.value)}
            <option value={s.value}>{s.label}</option>
          {/each}
        </optgroup>
      {/each}
    </select>
  </label>

  <div class="flex flex-col gap-1 text-sm">
    <span class="font-medium">{s.notes}</span>
    <NoteEditor value={notes} onsave={onsavenotes} label={s.notes} showSaveState />
  </div>
</div>
