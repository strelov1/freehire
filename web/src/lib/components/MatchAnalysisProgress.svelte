<script lang="ts">
  import { Check, Loader } from '@lucide/svelte';
  import type { MatchStreamState } from '#lib/matchAnalysis.js';

  // The fit analysis while it computes: the stage stepper, the dropped-connection note and
  // the model's reasoning as it streams. MatchAnalysisFull owns the stream.
  let {
    stages,
    thinking,
    recovering,
    showThinking = $bindable(),
  }: {
    stages: MatchStreamState['stages'];
    thinking: string;
    recovering: boolean;
    showThinking: boolean;
  } = $props();

  const thinkingId = $props.id();
  // The thinking panel tails the model's reasoning: keep it pinned to the newest tokens.
  let thinkingEl = $state<HTMLElement | null>(null);
  $effect(() => {
    void thinking; // track new reasoning
    if (thinkingEl) thinkingEl.scrollTop = thinkingEl.scrollHeight;
  });
</script>

<section class="rounded-2xl border border-border bg-card p-6 sm:px-8">
  <!-- Stage stepper: evenly-spaced nodes over a single connecting rail. -->
  <div class="relative flex">
    <div class="absolute inset-x-[16.67%] top-4 h-px bg-border" aria-hidden="true"></div>
    {#each stages as st (st.n)}
      <div class="relative z-10 flex flex-1 flex-col items-center gap-2">
        <span
          class="flex size-8 shrink-0 items-center justify-center rounded-full border text-sm font-semibold transition-colors
          {st.state === 'done'
            ? 'border-brand bg-brand text-brand-foreground'
            : st.state === 'active'
              ? 'border-brand bg-card text-primary'
              : 'border-border bg-card text-muted-foreground'}"
        >
          {#if st.state === 'done'}<Check class="size-4" />
          {:else if st.state === 'active'}<Loader class="size-4 animate-spin" />
          {:else}{st.n}{/if}
        </span>
        <span
          class="max-w-[7rem] text-center text-xs font-medium leading-tight sm:text-sm {st.state === 'pending'
            ? 'text-muted-foreground'
            : 'text-foreground'}">{st.label}</span
        >
      </div>
    {/each}
  </div>

  {#if recovering}
    <p class="mt-6 flex items-center justify-center gap-2 border-t border-border pt-4 text-xs text-muted-foreground">
      <Loader class="size-3.5 animate-spin" aria-hidden="true" /> Connection dropped — waiting for your result…
    </p>
  {/if}

  {#if thinking}
    <div class="mt-6 border-t border-border pt-4">
      <button
        type="button"
        class="flex items-center gap-2 text-xs font-semibold text-muted-foreground"
        onclick={() => (showThinking = !showThinking)}
        aria-expanded={showThinking}
        aria-controls={thinkingId}
      >
        <span class="relative flex size-2" aria-hidden="true">
          <span class="absolute inline-flex size-full motion-safe:animate-ping rounded-full bg-brand/60"></span>
          <span class="relative inline-flex size-2 rounded-full bg-brand"></span>
        </span>
        Thinking <span aria-hidden="true">{showThinking ? '▾' : '▸'}</span>
      </button>
      {#if showThinking}
        <p
          id={thinkingId}
          bind:this={thinkingEl}
          class="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap font-mono text-xs leading-relaxed text-muted-foreground"
        >
          {thinking}
        </p>
      {/if}
    </div>
  {/if}
</section>
