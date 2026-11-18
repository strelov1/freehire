<script lang="ts">
  import { MessagesSquare, Rocket } from '@lucide/svelte';
  import type { OpeningAction } from '#lib/assistant/presets.js';

  // The actions an empty conversation opens with, offered by AssistantChat's host.
  let {
    actions,
    disabled,
    onRun,
  }: { actions: readonly OpeningAction[]; disabled: boolean; onRun: (action: OpeningAction) => void } = $props();
</script>

<!-- A plain list of left-aligned rows rather than filled pills in a boxed card: the
   brand fill is kept for the unattended run, the one action that does the work;
   a question to ask is a quiet row, so three of them no longer read as three
   competing calls to action. -->
<div>
  <div class="flex flex-col gap-2">
    <!-- The icon comes from the action's kind, not from the action itself: the
       surfaces that offer these are plain modules, and a Svelte component cannot
       travel through one. The two kinds are the two rhythms — the unattended run
       and the conversation — so the mapping is complete by construction. -->
    {#each actions as action (action.label)}
      {@const Icon = action.kind === 'autopilot' ? Rocket : MessagesSquare}
      <button
        type="button"
        class={[
          'flex w-full items-center gap-2.5 rounded-lg px-3 py-2.5 text-left text-sm font-medium transition-colors disabled:opacity-50',
          action.kind === 'autopilot'
            ? 'bg-brand text-brand-foreground hover:opacity-90'
            : 'border border-border bg-card text-foreground hover:bg-accent',
        ]}
        {disabled}
        onclick={() => onRun(action)}
      >
        <Icon class={['size-4 shrink-0', action.kind !== 'autopilot' && 'text-muted-foreground']} />
        {action.label}
      </button>
    {/each}
  </div>
  {#each actions.filter((a) => a.hint) as action (action.label)}
    <p class="mt-2 text-xs text-muted-foreground">{action.hint}</p>
  {/each}
</div>
