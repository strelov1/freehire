<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import ConversationPane from './ConversationPane.svelte';
  import ConversationRow from './ConversationRow.svelte';
  import States from './States.svelte';

  // The Chats tab: the full-size conversation list beside one conversation's pane. The
  // list is the shared one in `messagingDock`, so the messaging dock on every other page shows
  // the same unread counts; the pane is the same component a dock window holds.
  const LIST_POLL_MS = 30000;
  let { initialId = null, oninvite }: { initialId?: number | null; oninvite?: (handle: string) => void } = $props();

  let selectedId = $state<number | null>(untrack(() => initialId));
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  const selected = $derived(selectedId === null ? null : messagingDock.conversation(selectedId) ?? null);
  let listTimer: ReturnType<typeof setInterval> | undefined;

  async function refreshList() {
    try { await messagingDock.refresh(); status = 'ready'; }
    // A failed poll keeps the list already on screen.
    catch { if (status === 'loading') status = 'error'; }
  }
  function onVisibility() {
    if (document.visibilityState === 'hidden') {
      if (listTimer) clearInterval(listTimer);
      listTimer = undefined;
      return;
    }
    void refreshList();
    if (!listTimer) listTimer = setInterval(() => void refreshList(), LIST_POLL_MS);
  }
  onMount(() => {
    void refreshList();
    if (document.visibilityState === 'visible') listTimer = setInterval(() => void refreshList(), LIST_POLL_MS);
    document.addEventListener('visibilitychange', onVisibility);
  });
  onDestroy(() => {
    if (listTimer) clearInterval(listTimer);
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
  });
</script>

{#if status === 'loading'}<States state="loading" />{:else if status === 'error'}<States state="error" message="Could not load your conversations." onRetry={() => { status = 'loading'; void refreshList(); }} />{:else}
  <div class="grid gap-4 md:grid-cols-3">
    <section aria-label="Conversations" class="divide-y divide-border rounded-lg border border-border">
      {#if messagingDock.conversations.length === 0}<p class="p-4 text-sm text-muted-foreground">No conversations yet.</p>{/if}
      {#each messagingDock.conversations as c (c.id)}<ConversationRow conversation={c} selected={selected?.id === c.id} onselect={(next) => (selectedId = next.id)} />{/each}
    </section>
    <section class="flex min-h-80 flex-col rounded-lg border border-border p-4 md:col-span-2" aria-label="Chat">
      {#if !selected}<p class="m-auto text-sm text-muted-foreground">Choose a conversation to read.</p>
      {:else}
        {#key selected.id}
          <ConversationPane conversation={selected} {oninvite} onchange={(change) => { if (change !== 'accepted') selectedId = null; }} />
        {/key}
      {/if}
    </section>
  </div>
{/if}
