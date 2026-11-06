<script lang="ts">
  /**
   * The support agent, opened beside the account pages in the shared docked panel. It runs
   * on its own routes and is free for every account, so unlike the other assistant surfaces
   * it never meets an upgrade prompt.
   */
  import { Plus } from '@lucide/svelte';
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import type { OpeningAction } from '#lib/assistant/presets.js';
  import AssistantPanel from '#lib/components/AssistantPanel.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './SupportPanel.messages';

  let { open = false, onClose }: { open?: boolean; onClose: () => void } = $props();

  const s = $derived(t(messages, locale()));
  const starters = $derived<OpeningAction[]>(
    [s.askPlan, s.askLimits, s.askProblem].map((text) => ({ label: text, kind: 'message', text })),
  );

  // The chat has no session rail here, so a conversation that reached its turn limit had no
  // way out but a reload. Remounting starts a fresh support session; closing the panel does
  // not remount, so a turn still streaming is never cut off — and neither does this button,
  // which waits for the reply to finish.
  let chatKey = $state(0);
  let replying = $state(false);
</script>

<AssistantPanel {open} title={s.title} {onClose}>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="flex shrink-0 justify-end border-b border-border px-3 py-1.5">
      <button
        type="button"
        onclick={() => chatKey++}
        disabled={replying}
        class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
      >
        <Plus class="size-3.5" />{s.newChat}
      </button>
    </div>
    {#key chatKey}
      <AssistantChat
        preset="support"
        showSessionRail={false}
        openingActions={starters}
        onTurnStateChange={(active) => (replying = active)}
      />
    {/key}
  </div>
</AssistantPanel>
