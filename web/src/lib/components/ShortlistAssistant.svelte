<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { MessageSquareText } from '@lucide/svelte';
  import { createShortlistSession, getSession } from '#lib/assistant/api.js';
  import type { SessionSummary } from '#lib/assistant/wire.js';
  import { ShortlistSession, shortlistQuery } from '#lib/shortlist.js';
  import { signinUrl } from '#lib/signin.js';
  import { errorMessage } from '#lib/utils.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Button } from '#lib/ui/index.js';
  import AssistantPanel from './AssistantPanel.svelte';
  import { messages } from './ShortlistAssistant.messages';

  let { params, userId }: { params: URLSearchParams; userId: number | null } = $props();
  const s = $derived(t(messages, locale()));
  const currentQuery = $derived(shortlistQuery(params));
  let open = $state(false);
  let opening = $state(false);
  let turnActive = $state(false);
  let failure = $state<string | null>(null);
  let conversation = $state.raw<SessionSummary | null>(null);
  let Chat = $state<typeof import('#lib/assistant/AssistantChat.svelte').default | null>(null);
  let capturedQuery = $state<string | null>(null);
  let controller: ShortlistSession | null = null;
  let controllerOwner: number | null = null;
  const changed = $derived(capturedQuery !== null && currentQuery !== shortlistQuery(capturedQuery));
  const searchHref = $derived(`${resolve('/jobs')}${capturedQuery ? `?${capturedQuery}` : ''}`);

  async function activate(fresh = false) {
    if (opening || (fresh && turnActive)) return;
    if (userId === null) {
      const returnTo = `${resolve('/jobs')}${currentQuery ? `?${currentQuery}` : ''}`;
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl resolves the route.
      void goto(signinUrl({ returnTo, mode: 'login' }));
      return;
    }
    if (controllerOwner !== userId) {
      controller = new ShortlistSession(userId);
      controllerOwner = userId;
      conversation = null;
    }
    const owner = controller;
    if (!owner) return;
    if (fresh) {
      owner.startNew();
      conversation = null;
    }
    capturedQuery = owner.query ?? currentQuery;
    open = true;
    failure = null;
    opening = true;
    try {
      const result = await owner.open(currentQuery, createShortlistSession, getSession);
      const module = await import('#lib/assistant/AssistantChat.svelte');
      if (controller !== owner) return;
      Chat = module.default;
      conversation = result;
      capturedQuery = result.search_context?.query ?? owner.query ?? currentQuery;
    } catch (err) {
      failure = errorMessage(err, s.failed);
    } finally {
      opening = false;
    }
  }
</script>

<Button variant="outline" size="sm" onclick={() => void activate()} disabled={opening}>
  <MessageSquareText class="size-4" aria-hidden="true" />
  {s.open}
</Button>

<AssistantPanel {open} title={s.title} onClose={() => (open = false)}>
  {#snippet children(setSessionId)}
    <div class="flex min-h-0 flex-1 flex-col">
      <div class="shrink-0 space-y-2 border-b border-border p-3 text-sm">
        <p class="font-medium">{s.captured}</p>
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- searchHref includes the resolved jobs route. -->
        <a href={searchHref} target="_blank" rel="noopener" class="text-brand underline underline-offset-2">{s.viewSearch}</a>
        {#if changed}
          <p class="text-xs text-muted-foreground">{s.changed}</p>
        {/if}
        <Button variant="secondary" size="sm" disabled={opening || turnActive} onclick={() => void activate(true)}>{s.startCurrent}</Button>
      </div>
      {#if opening}
        <p class="p-4 text-sm text-muted-foreground" role="status">{s.opening}</p>
      {:else if failure}
        <div class="space-y-3 p-4">
          <p class="text-sm text-destructive" role="alert">{failure}</p>
          <Button variant="secondary" size="sm" onclick={() => void activate()}>{s.retry}</Button>
        </div>
      {/if}
      {#if conversation && Chat}
        {#key conversation.id}
          <Chat
            session={conversation.id}
            kickoff={s.kickoff}
            sessionLabel={s.title}
            showSessionRail={false}
            onSessionChange={setSessionId}
            onTurnStateChange={(active) => (turnActive = active)}
          />
        {/key}
      {/if}
    </div>
  {/snippet}
</AssistantPanel>
