<script lang="ts">
  import { onMount } from 'svelte';
  import { openApplyChat } from '#lib/assistant/api.js';
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Button } from '#lib/ui/index.js';
  import { errorMessage } from '#lib/utils.js';
  import { messages } from './JobDrawer.messages';

  let {
    slug,
    onTurnComplete,
  }: {
    slug: string | null;
    onTurnComplete: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  let session = $state<string | null>(null);
  let error = $state<string | null>(null);
  let loading = $state(false);
  let disposed = false;

  async function openChat() {
    if (!slug || loading) return;
    loading = true;
    error = null;
    try {
      const chat = await openApplyChat(slug);
      if (!disposed) session = chat.id;
    } catch (e) {
      if (!disposed) error = errorMessage(e, s.applyChat.openFailed);
    } finally {
      if (!disposed) loading = false;
    }
  }

  onMount(() => {
    void openChat();
    return () => {
      disposed = true;
    };
  });
</script>

<section class="flex h-[min(65dvh,44rem)] min-h-80 flex-col" aria-label={s.applyChat.title}>
  {#if session}
    <AssistantChat {session} showSessionRail={false} syncApplication {onTurnComplete} />
  {:else if error}
    <div class="flex flex-col items-start gap-3" role="alert">
      <p class="text-sm text-destructive">{error}</p>
      <Button variant="outline" size="sm" onclick={openChat}>{s.tryAgain}</Button>
    </div>
  {:else}
    <p class="text-sm text-muted-foreground" role="status">{s.loading}</p>
  {/if}
</section>
