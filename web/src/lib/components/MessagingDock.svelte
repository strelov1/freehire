<script lang="ts">
  import { ChevronDown, ChevronUp, MessageSquare, Minus, X } from '@lucide/svelte';
  import { onDestroy, onMount, tick } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { counterpartName } from '#lib/messaging.js';
  import { type DockWindow, mobileSheetId, totalUnread } from '#lib/messagingDock.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import { ownsMobileStickyCta } from '#lib/supportToast.js';
  import BuddyInviteDialog from './BuddyInviteDialog.svelte';
  import ConversationCounterpart from './ConversationCounterpart.svelte';
  import ConversationPane from './ConversationPane.svelte';
  import ConversationRow from './ConversationRow.svelte';
  import UnreadBadge from './UnreadBadge.svelte';

  // The messaging dock: a bar in the bottom-right corner that unfolds into the
  // conversation list (every kind), with up to two chat windows beside it. Mounted once by
  // the root layout and only for an account with a conversation, so it and its windows
  // survive client-side navigation. It renders nothing on /my/buddies, whose Chats tab is
  // the full-size version of the same list and pane. It also hosts the buddy first-message
  // dialog, so "Message" works from any page.
  //
  // A narrow screen shows one chat at a time as a full-width sheet under the header, and
  // that sheet is modal: everything outside it is inert while it is open, and folding it
  // returns focus to whatever opened it. Minimized chats stack as bars above the list.
  // On a wide screen the windows sit beside the list and nothing is modal.
  //
  // The list polls every 30 s while the page is visible and the dock is on screen; each
  // open window's pane polls its own messages. Minimizing a window unmounts its pane, so a
  // minimized chat costs nothing; its unsent draft is kept in `messagingDock`.
  const LIST_POLL_MS = 30000;

  const shown = $derived(messagingDock.shownOn(page.url.pathname));
  const unread = $derived(totalUnread(messagingDock.conversations));
  // Tailwind's `sm` breakpoint, which is where the layout below switches.
  const wide = new MediaQuery('min-width: 40rem', true);
  // The window drawn as the narrow screen's sheet, and the one holding the page modal.
  const sheetId = $derived(mobileSheetId(messagingDock.windows));
  const modalId = $derived(!wide.current && messagingDock.inviteHandle === null ? sheetId : null);
  let visible = $state(false);
  let headerButtons: Record<number, HTMLButtonElement | undefined> = $state({});
  let windowEls: Record<number, HTMLElement | undefined> = $state({});
  let listToggle = $state<HTMLButtonElement>();
  let listEl = $state<HTMLElement>();
  // Windows already on screen, so only a newly opened one takes focus — not the ones
  // restored from sessionStorage on load.
  let knownWindows = new Set(messagingDock.windows.map((w) => w.id));
  let expandedWindows = new Set(messagingDock.windows.filter((w) => !w.minimized).map((w) => w.id));
  // The control that unfolded each window, so the narrow screen's sheet can hand focus back.
  const openers: Record<number, HTMLElement | null> = {};

  $effect(() => {
    if (!shown || !visible) return;
    void messagingDock.refresh().catch(() => { /* the next poll catches up */ });
    const timer = setInterval(() => void messagingDock.refresh().catch(() => {}), LIST_POLL_MS);
    return () => clearInterval(timer);
  });

  // A window opened from a "Message" control anywhere on the page takes focus, so a
  // keyboard user lands in it rather than at the end of the document.
  $effect(() => {
    const ids = messagingDock.windows.map((w) => w.id);
    const opened = ids.find((id) => !knownWindows.has(id));
    knownWindows = new Set(ids);
    const expanded = messagingDock.windows.filter((w) => !w.minimized).map((w) => w.id);
    const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    for (const id of expanded) if (!expandedWindows.has(id)) openers[id] = active;
    expandedWindows = new Set(expanded);
    if (opened !== undefined) void tick().then(() => headerButtons[opened]?.focus());
  });

  // The narrow screen's sheet covers the page, so it behaves as a modal: the rest of the
  // document (the dock's own list and bars included) is inert, which keeps Tab inside it,
  // and focus moves in if it is not there already. Lifted while the first-message dialog
  // is open, since that dialog lives outside the sheet.
  $effect(() => {
    if (modalId === null) return;
    const sheet = windowEls[modalId];
    if (!sheet) return;
    const restore = inertOutside(sheet);
    if (!sheet.contains(document.activeElement)) headerButtons[modalId]?.focus();
    return restore;
  });

  function inertOutside(el: HTMLElement): () => void {
    const made: HTMLElement[] = [];
    for (let node: HTMLElement | null = el; node && node !== document.body; node = node.parentElement) {
      for (const sibling of node.parentElement?.children ?? []) {
        if (sibling !== node && sibling instanceof HTMLElement && !sibling.inert) {
          sibling.inert = true;
          made.push(sibling);
        }
      }
    }
    return () => { for (const n of made) n.inert = false; };
  }

  // Where focus goes once a window folds or closes: on a narrow screen back to the control
  // that opened the sheet when it is still on screen; otherwise the window's own bar, or
  // the list toggle once the window is gone.
  function returnFocus(id: number, closed: boolean) {
    const opener = openers[id];
    openers[id] = null;
    void tick().then(() => {
      if (!wide.current && opener?.isConnected && !windowEls[id]?.contains(opener) && opener.checkVisibility()) opener.focus();
      else if (!closed && headerButtons[id]) headerButtons[id]?.focus();
      else listToggle?.focus();
    });
  }

  function minimize(id: number) {
    messagingDock.setMinimized(id, true);
    returnFocus(id, false);
  }

  function close(id: number) {
    messagingDock.close(id);
    returnFocus(id, true);
  }

  function toggle(w: DockWindow) {
    if (w.minimized) messagingDock.setMinimized(w.id, false);
    else minimize(w.id);
  }

  function onVisibility() { visible = document.visibilityState === 'visible'; }
  onMount(() => {
    visible = document.visibilityState === 'visible';
    document.addEventListener('visibilitychange', onVisibility);
  });
  onDestroy(() => {
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
  });

  // A narrow screen shows the sheet window under the header and each minimized one as a
  // bar stacked above the list; a wide one shows each beside the list, a minimized one as
  // its header bar alone.
  function windowLayout(minimized: boolean, sheet: boolean): string {
    if (minimized) return 'flex w-72 rounded-lg border sm:w-64 sm:rounded-b-none sm:border-b-0';
    if (sheet) return 'fixed inset-x-0 bottom-0 top-14 z-10 flex rounded-t-lg border border-b-0 sm:static sm:z-auto sm:h-112 sm:w-80';
    return 'hidden rounded-t-lg border border-b-0 sm:flex sm:h-112 sm:w-80';
  }

  // Escape folds the window holding focus (or the list), never anything a dialog inside it
  // is handling. The draft survives, since it is kept by conversation in `messagingDock`.
  function onKeydown(e: KeyboardEvent) {
    const target = e.target;
    if (e.key !== 'Escape' || !(target instanceof Node)) return;
    if (target instanceof Element && target.closest('dialog')) return;
    const open = messagingDock.windows.find((w) => !w.minimized && windowEls[w.id]?.contains(target));
    if (open) {
      e.preventDefault();
      minimize(open.id);
    } else if (messagingDock.expanded && listEl?.contains(target)) {
      e.preventDefault();
      messagingDock.expanded = false;
      listToggle?.focus();
    }
  }
</script>

<svelte:window onkeydown={shown ? onKeydown : undefined} />

{#if shown}
  <aside
    aria-label="Messaging"
    class="pointer-events-none fixed bottom-0 right-2 z-30 flex-col items-end gap-2 sm:right-4 sm:flex-row {ownsMobileStickyCta(page.url.pathname) ? 'hidden lg:flex' : 'flex'}"
  >
    {#each messagingDock.windows as w (w.id)}
      {@const c = messagingDock.conversation(w.id)}
      {#if c}
        {@const name = counterpartName(c.counterpart)}
        {@const modal = modalId === w.id}
        <section
          aria-label="Conversation with {name}"
          role={modal ? 'dialog' : undefined}
          aria-modal={modal ? 'true' : undefined}
          bind:this={windowEls[w.id]}
          class="pointer-events-auto flex-col border-border bg-background shadow-lg {windowLayout(w.minimized, sheetId === w.id)}"
        >
          <header class="flex items-center gap-1 border-b border-border px-1 py-0.5 sm:px-2 sm:py-1.5 {w.minimized ? 'max-sm:border-b-0' : ''}">
            <button
              bind:this={headerButtons[w.id]}
              type="button"
              class="flex min-h-11 min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1 text-left text-sm font-semibold hover:bg-accent sm:min-h-0 sm:px-1"
              aria-expanded={!w.minimized}
              onclick={() => toggle(w)}
            >
              <ConversationCounterpart counterpart={c.counterpart} nameClass="truncate" />
              {#if w.minimized}<UnreadBadge count={c.unread} />{/if}
            </button>
            <button
              type="button"
              class="inline-flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground sm:size-7"
              aria-label={w.minimized ? `Expand conversation with ${name}` : `Minimize conversation with ${name}`}
              onclick={() => toggle(w)}
            >
              {#if w.minimized}<ChevronUp class="size-4" aria-hidden="true" />{:else}<Minus class="size-4" aria-hidden="true" />{/if}
            </button>
            <button
              type="button"
              class="inline-flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground sm:size-7"
              aria-label="Close conversation with {name}"
              onclick={() => close(w.id)}
            >
              <X class="size-4" aria-hidden="true" />
            </button>
          </header>
          {#if !w.minimized}
            {#key c.id}
              <ConversationPane
                conversation={c}
                compact
                onchange={(change) => { if (change !== 'accepted') messagingDock.close(c.id); }}
                oninvite={(handle) => messagingDock.messageBuddy(handle)}
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
        class="flex min-h-11 w-full items-center justify-between gap-2 rounded-t-lg px-3 py-2.5 text-sm font-semibold hover:bg-accent"
        aria-expanded={messagingDock.expanded}
        aria-controls="messaging-dock-list"
        onclick={() => (messagingDock.expanded = !messagingDock.expanded)}
      >
        <span class="flex items-center gap-2">
          <MessageSquare class="size-4" aria-hidden="true" />
          Messaging
          <UnreadBadge count={unread} />
        </span>
        {#if messagingDock.expanded}<ChevronDown class="size-4" aria-hidden="true" />{:else}<ChevronUp class="size-4" aria-hidden="true" />{/if}
      </button>
      {#if messagingDock.expanded}
        <div id="messaging-dock-list" class="max-h-96 overflow-y-auto border-t border-border">
          {#if messagingDock.conversations.length === 0}<p class="p-3 text-sm text-muted-foreground">No conversations yet.</p>{/if}
          <div class="divide-y divide-border">
            {#each messagingDock.conversations as c (c.id)}
              <ConversationRow
                conversation={c}
                selected={messagingDock.windows.some((w) => w.id === c.id && !w.minimized)}
                onselect={(next) => messagingDock.openConversation(next.id)}
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
  bind:open={() => messagingDock.inviteHandle !== null, (open) => { if (!open) messagingDock.inviteHandle = null; }}
  handle={messagingDock.inviteHandle ?? ''}
  onsent={(conversation) => void messagingDock.invited(conversation)}
/>
