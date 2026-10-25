<script lang="ts">
  import { ChevronDown, ChevronUp, MessageSquare, Minus, X } from '@lucide/svelte';
  import { onDestroy, onMount, tick } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { buddyConversationName } from '#lib/buddies.js';
  import { totalUnread } from '#lib/buddyDock.js';
  import { buddyDock } from '#lib/buddyDock.svelte.js';
  import { ownsMobileStickyCta } from '#lib/supportToast.js';
  import BuddyConversationPane from './BuddyConversationPane.svelte';
  import BuddyConversationRow from './BuddyConversationRow.svelte';
  import BuddyIdentity from './BuddyIdentity.svelte';
  import BuddyInviteDialog from './BuddyInviteDialog.svelte';

  // The interview-buddies messaging dock: a bar in the bottom-right corner that unfolds
  // into the conversation list, with up to two chat windows beside it (one full-width
  // sheet on a narrow screen). Mounted once by the root layout and only for a member, so it
  // and its windows survive client-side navigation. It renders nothing on /my/buddies,
  // whose Chats tab is the full-size version of the same list and pane.
  //
  // The list polls every 30 s while the page is visible and the dock is on screen; each
  // open window's pane polls its own messages. Minimizing a window unmounts its pane, so a
  // minimized chat costs nothing.
  const LIST_POLL_MS = 30000;

  const shown = $derived(buddyDock.shownOn(page.url.pathname));
  const unread = $derived(totalUnread(buddyDock.conversations));
  let visible = $state(false);
  let headerButtons: Record<number, HTMLButtonElement | undefined> = $state({});
  let windowEls: Record<number, HTMLElement | undefined> = $state({});
  let listToggle = $state<HTMLButtonElement>();
  let listEl = $state<HTMLElement>();
  // Windows already on screen, so only a newly opened one takes focus — not the ones
  // restored from sessionStorage on load.
  let knownWindows = new Set(buddyDock.windows.map((w) => w.id));

  $effect(() => {
    if (!shown || !visible) return;
    void buddyDock.refresh().catch(() => { /* the next poll catches up */ });
    const timer = setInterval(() => void buddyDock.refresh().catch(() => {}), LIST_POLL_MS);
    return () => clearInterval(timer);
  });

  // A window opened from a "Message" control anywhere on the page takes focus, so a
  // keyboard user lands in it rather than at the end of the document.
  $effect(() => {
    const ids = buddyDock.windows.map((w) => w.id);
    const opened = ids.find((id) => !knownWindows.has(id));
    knownWindows = new Set(ids);
    if (opened !== undefined) void tick().then(() => headerButtons[opened]?.focus());
  });

  function onVisibility() { visible = document.visibilityState === 'visible'; }
  onMount(() => {
    visible = document.visibilityState === 'visible';
    document.addEventListener('visibilitychange', onVisibility);
  });
  onDestroy(() => {
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
  });

  // Escape folds the window holding focus (or the list), never anything a dialog inside it
  // is handling, and leaves focus on the header so it is not lost to the document.
  // A narrow screen shows only the newest open window, as a sheet under the header; a
  // wide one shows each beside the list, a minimized one as its header bar alone.
  function windowLayout(minimized: boolean, newest: boolean): string {
    if (minimized) return 'hidden sm:flex sm:w-64';
    if (newest) return 'fixed inset-x-0 bottom-0 top-14 z-10 flex sm:static sm:z-auto sm:h-112 sm:w-80';
    return 'hidden sm:flex sm:h-112 sm:w-80';
  }

  function onKeydown(e: KeyboardEvent) {
    const target = e.target;
    if (e.key !== 'Escape' || !(target instanceof Node)) return;
    if (target instanceof Element && target.closest('dialog')) return;
    const open = buddyDock.windows.find((w) => !w.minimized && windowEls[w.id]?.contains(target));
    if (open) {
      e.preventDefault();
      buddyDock.setMinimized(open.id, true);
      headerButtons[open.id]?.focus();
    } else if (buddyDock.expanded && listEl?.contains(target)) {
      e.preventDefault();
      buddyDock.expanded = false;
      listToggle?.focus();
    }
  }
</script>

<svelte:window onkeydown={shown ? onKeydown : undefined} />

{#if shown}
  <aside
    aria-label="Messaging"
    class="pointer-events-none fixed bottom-0 right-2 z-30 items-end gap-2 sm:right-4 {ownsMobileStickyCta(page.url.pathname) ? 'hidden lg:flex' : 'flex'}"
  >
    {#each buddyDock.windows as w, i (w.id)}
      {@const c = buddyDock.conversation(w.id)}
      {#if c}
        {@const name = buddyConversationName(c.former_member, c.other.handle)}
        {@const newest = i === buddyDock.windows.length - 1}
        <section
          aria-label="Conversation with {name}"
          bind:this={windowEls[w.id]}
          class="pointer-events-auto flex-col rounded-t-lg border border-b-0 border-border bg-background shadow-lg {windowLayout(w.minimized, newest)}"
        >
          <header class="flex items-center gap-1 border-b border-border px-2 py-1.5">
            <button
              bind:this={headerButtons[w.id]}
              type="button"
              class="flex min-w-0 flex-1 items-center gap-2 rounded-md px-1 py-1 text-left text-sm font-semibold hover:bg-accent"
              aria-expanded={!w.minimized}
              onclick={() => buddyDock.setMinimized(w.id, !w.minimized)}
            >
              {#if c.former_member}<span class="truncate">{name}</span>{:else}<BuddyIdentity handle={c.other.handle} summary={c.other.card_summary} size="sm" link={false} />{/if}
              {#if w.minimized && c.unread}<span class="shrink-0 rounded-full bg-brand px-2 text-xs text-brand-foreground">{c.unread}<span class="sr-only"> unread</span></span>{/if}
            </button>
            <button
              type="button"
              class="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              aria-label={w.minimized ? `Expand conversation with ${name}` : `Minimize conversation with ${name}`}
              onclick={() => buddyDock.setMinimized(w.id, !w.minimized)}
            >
              {#if w.minimized}<ChevronUp class="size-4" aria-hidden="true" />{:else}<Minus class="size-4" aria-hidden="true" />{/if}
            </button>
            <button
              type="button"
              class="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              aria-label="Close conversation with {name}"
              onclick={() => buddyDock.close(w.id)}
            >
              <X class="size-4" aria-hidden="true" />
            </button>
          </header>
          {#if !w.minimized}
            {#key c.id}
              <BuddyConversationPane
                conversation={c}
                compact
                onchange={(change) => { if (change !== 'accepted') buddyDock.close(c.id); }}
              />
            {/key}
          {/if}
        </section>
      {/if}
    {/each}

    <section bind:this={listEl} aria-label="Conversations" class="pointer-events-auto w-72 rounded-t-lg border border-b-0 border-border bg-background shadow-lg">
      <button
        bind:this={listToggle}
        type="button"
        class="flex w-full items-center justify-between gap-2 rounded-t-lg px-3 py-2.5 text-sm font-semibold hover:bg-accent"
        aria-expanded={buddyDock.expanded}
        aria-controls="buddy-dock-list"
        onclick={() => (buddyDock.expanded = !buddyDock.expanded)}
      >
        <span class="flex items-center gap-2">
          <MessageSquare class="size-4" aria-hidden="true" />
          Messaging
          {#if unread}<span class="rounded-full bg-brand px-2 text-xs text-brand-foreground">{unread}<span class="sr-only"> unread</span></span>{/if}
        </span>
        {#if buddyDock.expanded}<ChevronDown class="size-4" aria-hidden="true" />{:else}<ChevronUp class="size-4" aria-hidden="true" />{/if}
      </button>
      {#if buddyDock.expanded}
        <div id="buddy-dock-list" class="max-h-96 overflow-y-auto border-t border-border">
          {#if buddyDock.conversations.length === 0}<p class="p-3 text-sm text-muted-foreground">No conversations yet.</p>{/if}
          <div class="divide-y divide-border">
            {#each buddyDock.conversations as c (c.id)}
              <BuddyConversationRow
                conversation={c}
                selected={buddyDock.windows.some((w) => w.id === c.id && !w.minimized)}
                onselect={(next) => buddyDock.openConversation(next.id)}
              />
            {/each}
          </div>
          <a href={resolve('/my/buddies')} class="block border-t border-border p-3 text-sm font-medium text-brand hover:underline">Open interview buddies</a>
        </div>
      {/if}
    </section>
  </aside>
{/if}

<BuddyInviteDialog
  bind:open={() => buddyDock.inviteHandle !== null, (open) => { if (!open) buddyDock.inviteHandle = null; }}
  handle={buddyDock.inviteHandle ?? ''}
  onsent={(conversation) => void buddyDock.invited(conversation)}
/>
