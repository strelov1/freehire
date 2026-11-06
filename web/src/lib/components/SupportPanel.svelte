<script lang="ts">
  /**
   * The support agent, opened beside the account pages in the shared docked panel. It runs
   * on its own routes and is free for every account, so unlike the other assistant surfaces
   * it never meets an upgrade prompt.
   */
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
</script>

<AssistantPanel {open} title={s.title} {onClose}>
  <AssistantChat preset="support" showSessionRail={false} openingActions={starters} />
</AssistantPanel>
