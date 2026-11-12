<script lang="ts">
  /**
   * The support agent, opened beside the account pages in the shared docked panel. It runs
   * on its own routes and is free for every account, so unlike the other assistant surfaces
   * it never meets an upgrade prompt.
   */
  import { Plus, ThumbsDown, ThumbsUp } from '@lucide/svelte';
  import { rateSupportSession } from '#lib/assistant/api.js';
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

  // The only quality signal the support agent has: after a reply, the user may say whether
  // the conversation helped. One verdict per conversation, a second click replaces it.
  let sessionId = $state<string | null>(null);
  let answered = $state(false);
  let rated = $state<boolean | null>(null);

  function newConversation() {
    chatKey++;
    sessionId = null;
    answered = false;
    rated = null;
  }

  async function rate(helpful: boolean) {
    if (!sessionId) return;
    const previous = rated;
    rated = helpful;
    try {
      await rateSupportSession(sessionId, helpful);
    } catch {
      rated = previous;
    }
  }
</script>

<AssistantPanel {open} title={s.title} {onClose}>
  <!-- In the header row rather than a band of its own under it. -->
  {#snippet headerActions()}
    <button
      type="button"
      onclick={newConversation}
      disabled={replying}
      class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
    >
      <Plus class="size-3.5" />{s.newChat}
    </button>
  {/snippet}
  <div class="flex min-h-0 flex-1 flex-col">
    {#key chatKey}
      <AssistantChat
        preset="support"
        showSessionRail={false}
        openingActions={starters}
        onTurnStateChange={(active) => (replying = active)}
        onSessionChange={(id) => (sessionId = id)}
        onTurnComplete={() => (answered = true)}
      />
    {/key}
    {#if answered && sessionId && !replying}
      <div
        class="flex items-center justify-center gap-2 border-t border-border px-3 py-1.5 text-xs text-muted-foreground"
      >
        {#if rated === null}
          <span>{s.helpedQuestion}</span>
        {:else}
          <span role="status">{s.helpedThanks}</span>
        {/if}
        {#each [true, false] as helpful (helpful)}
          {@const Icon = helpful ? ThumbsUp : ThumbsDown}
          <button
            type="button"
            onclick={() => rate(helpful)}
            aria-label={helpful ? s.helpedYes : s.helpedNo}
            aria-pressed={rated === helpful}
            class="rounded-md p-1 transition-colors hover:bg-accent hover:text-foreground aria-pressed:text-foreground"
          >
            <Icon class="size-3.5" />
          </button>
        {/each}
      </div>
    {/if}
  </div>
</AssistantPanel>
