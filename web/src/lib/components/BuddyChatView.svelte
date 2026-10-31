<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import { api } from '#lib/api.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import type { BuddyCompany } from '#lib/types.js';
  import BuddyChatRail from './BuddyChatRail.svelte';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';
  import ConversationPane from './ConversationPane.svelte';
  import ConversationRow from './ConversationRow.svelte';
  import States from './States.svelte';

  // The Chats tab: the full-size conversation list beside one conversation's pane. The
  // list is the shared one in `messagingDock`, so the messaging dock on every other page shows
  // the same unread counts; the pane is the same component a dock window holds.
  const LIST_POLL_MS = 30000;
  let { initialId = null, oninvite }: { initialId?: number | null; oninvite?: (handle: string) => void } = $props();

  let selectedId = $state<number | null>(untrack(() => initialId));

  // The companies the viewer shares with each similar member, read once: the open chat's
  // "you both applied to" and the rail draw from it. A member outside the similar list
  // simply has none shown.
  let shared = $state<Map<string, BuddyCompany[]>>(new Map());
  onMount(() => {
    api.listBuddySimilar(50).then(
      (list) => { shared = new Map(list.map((m) => [m.handle, m.shared_companies])); },
      () => {},
    );
  });
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  const selected = $derived(selectedId === null ? null : messagingDock.conversation(selectedId) ?? null);
  const handle = $derived(selected?.counterpart.reachable ? selected.counterpart.detail.handle : null);
  const common = $derived(handle ? (shared.get(handle) ?? []) : []);
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

{#snippet together()}
  <p class="flex items-center gap-2 rounded-full bg-muted py-1 pr-3 pl-1.5 text-xs text-muted-foreground">
    <span class="flex" aria-hidden="true">
      {#each common.slice(0, 3) as c, i (c.slug)}
        <BuddyCompanyLogo name={c.name} size="sm" class="size-5 ring-2 ring-muted {i > 0 ? '-ml-1.5' : ''}" />
      {/each}
    </span>
    <span>You both applied to <span class="font-medium text-foreground">{common.map((c) => c.name).join(', ')}</span></span>
  </p>
{/snippet}

{#if status === 'loading'}<States state="loading" />{:else if status === 'error'}<States state="error" message="Could not load your conversations." onRetry={() => { status = 'loading'; void refreshList(); }} />{:else}
  <div class="flex flex-col gap-4 lg:h-160 lg:flex-row">
    <section aria-label="Conversations" class="flex max-h-64 shrink-0 flex-col overflow-y-auto rounded-xl border border-border bg-card p-1.5 lg:max-h-none lg:w-64">
      {#if messagingDock.conversations.length === 0}<p class="p-4 text-sm text-muted-foreground">No conversations yet. Message someone from the feed or your network.</p>{/if}
      {#each messagingDock.conversations as c (c.id)}<ConversationRow conversation={c} selected={selected?.id === c.id} onselect={(next) => (selectedId = next.id)} />{/each}
    </section>
    <section class="flex min-h-96 min-w-0 flex-1 flex-col rounded-xl border border-border bg-card p-4" aria-label="Chat">
      {#if !selected}<p class="m-auto text-sm text-muted-foreground">Choose a conversation to read.</p>
      {:else}
        {#key selected.id}
          <ConversationPane conversation={selected} {oninvite} intro={common.length ? together : undefined} onchange={(change) => { if (change !== 'accepted') selectedId = null; }} />
        {/key}
      {/if}
    </section>
    {#if handle}
      <BuddyChatRail {handle} summary={selected?.counterpart.reachable ? selected.counterpart.detail.card_summary : null} companies={common} />
    {/if}
  </div>
{/if}
