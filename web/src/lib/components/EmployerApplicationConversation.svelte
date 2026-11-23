<script module lang="ts">
  export type ConversationDraft = { text: string; key?: string };
</script>

<script lang="ts">
  import { MessageSquare } from '@lucide/svelte';
  import { onDestroy, tick } from 'svelte';
  import { api } from '#lib/api.js';
  import type { components } from '#lib/generated/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Button } from '#lib/ui/index.js';
  import { formatDateTime } from '#lib/utils.js';
  import { messages } from './EmployerApplicationConversation.messages';

  type Message = components['schemas']['EmployerApplicationMessage'];

  const {
    id,
    drafts,
    onread,
  }: { id: number; drafts: Map<number, ConversationDraft>; onread: (unread: boolean) => void } = $props();
  const s = $derived(t(messages, locale()));

  let open = $state(false);
  let destroyed = false;
  onDestroy(() => {
    destroyed = true;
  });
  let loaded = $state(false);
  let loading = $state(false);
  let busy = $state(false);
  let canSend = $state(false);
  let hasMore = $state(false);
  let thread = $state<Message[]>([]);
  const draft = $derived(drafts.get(id)?.text ?? '');
  let error = $state<'loadError' | 'sendError' | 'readError' | null>(null);

  async function load(before?: number) {
    if (loading || busy) return;
    loading = true;
    error = null;
    try {
      const result = await api.getEmployerApplicationConversation(id, before);
      thread = before ? [...thread, ...result.messages] : result.messages;
      loaded = true;
      canSend = result.can_send;
      hasMore = result.has_more;
      const newestReply = result.messages.find((message) => message.status === 'received');
      if (newestReply) {
        await tick();
        if (!open || destroyed) return;
        try {
          const state = await api.markEmployerApplicationRepliesRead(id, newestReply.id);
          if (!destroyed) onread(state.has_unread_reply);
        } catch {
          error = 'readError';
        }
      }
    } catch {
      error = 'loadError';
    } finally {
      loading = false;
    }
  }

  function toggle() {
    open = !open;
    if (open) void load();
  }

  async function send(attempt: { body: string; key: string }) {
    if (busy || loading || !canSend) return;
    const current = drafts.get(id);
    const submitted = current?.key === attempt.key && current.text.trim() === attempt.body ? current : undefined;
    busy = true;
    error = null;
    try {
      const result = await api.sendEmployerApplicationMessage(id, attempt.body, attempt.key);
      if (submitted && drafts.get(id) === submitted) drafts.delete(id);
      if (destroyed) return;
      const previous = thread.findIndex((message) => message.id === result.id);
      thread =
        previous < 0 ? [result, ...thread] : thread.map((message) => (message.id === result.id ? result : message));
    } catch {
      error = 'sendError';
    } finally {
      busy = false;
    }
  }

  function changeDraft(text: string) {
    const previous = drafts.get(id);
    const key = previous?.text.trim() === text.trim() ? previous.key : undefined;
    if (text) drafts.set(id, { text, key });
    else drafts.delete(id);
  }

  function submitDraft(event: SubmitEvent) {
    event.preventDefault();
    const current = drafts.get(id);
    const body = current?.text.trim();
    if (!current || !body || busy || loading || !canSend) return;
    const key = current.key ?? crypto.randomUUID();
    drafts.set(id, { text: current.text, key });
    void send({ body, key });
  }
</script>

<section class="flex max-w-xl flex-col items-start gap-3" aria-label={s.conversation}>
  <Button variant="outline" size="sm" aria-expanded={open} class="gap-1.5" onclick={toggle}>
    <MessageSquare class="size-4" />
    {s.conversation}
  </Button>

  {#if open}
    <div class="flex w-full flex-col gap-3 rounded-lg border border-border p-3">
      {#if loading}<p role="status" class="text-sm text-muted-foreground">{s.loading}</p>{/if}
      {#if error}<p role="alert" class="text-sm text-destructive">{s[error]}</p>{/if}
      {#if loaded}
        {#if hasMore && thread.length}
          <Button
            variant="ghost"
            size="sm"
            class="w-fit"
            disabled={loading || busy}
            onclick={() => load(thread.at(-1)?.id)}
          >
            {s.earlier}
          </Button>
        {/if}
        {#if thread.length}
          <ol class="flex flex-col gap-2">
            {#each [...thread].reverse() as message (message.id)}
              <li class="rounded-lg bg-muted/50 p-3">
                <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                  <span class="font-medium text-foreground">{message.status === 'received' ? s.candidate : s.team}</span
                  >
                  <time datetime={message.created_at}>{formatDateTime(message.created_at, locale())}</time>
                  {#if message.status !== 'received'}
                    <span class:text-destructive={message.status === 'failed'}>{s[message.status]}</span>
                  {/if}
                </div>
                <p class="mt-1 text-sm whitespace-pre-wrap break-words">{message.body}</p>
                {#if message.status === 'failed' && canSend && message.request_key}
                  <Button
                    variant="outline"
                    size="sm"
                    class="mt-2"
                    disabled={busy || loading}
                    onclick={() => message.request_key && send({ body: message.body, key: message.request_key })}
                    >{s.retry}</Button
                  >
                {/if}
              </li>
            {/each}
          </ol>
        {:else}
          <p class="text-sm text-muted-foreground">{s.empty}</p>
        {/if}
        {#if canSend}
          <form class="flex flex-col gap-2" onsubmit={submitDraft}>
            <label class="flex flex-col gap-1">
              <span class="text-sm font-medium">{s.draft}</span>
              <textarea
                value={draft}
                oninput={(event) => changeDraft(event.currentTarget.value)}
                rows="3"
                maxlength="2000"
                disabled={busy}
                class="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
              ></textarea>
            </label>
            <p class="text-xs text-muted-foreground">{s.replyHint}</p>
            <Button type="submit" size="sm" class="w-fit" disabled={busy || loading || !draft.trim()}>
              {busy ? s.sending : s.send}
            </Button>
          </form>
        {:else}
          <p class="text-xs text-muted-foreground">{s.unavailable}</p>
        {/if}
      {/if}
      <Button variant="ghost" size="sm" class="w-fit" disabled={loading || busy} onclick={() => load()}>
        {s.refresh}
      </Button>
    </div>
  {/if}
</section>
