// The caller's conversations (every kind) and the messaging dock's windows, as one
// module-level store: the dock in the root layout, the buddies Chats tab and every
// "Message" control read the SAME list, so an unread count cannot disagree between two
// surfaces on the same screen.
//
// The dock exists for an account that has at least one conversation. The list itself is
// open to every signed-in account (a non-member simply has none), so it is the only probe:
// never GET /me/buddies, which touches a member's ranking keys and may only run on entering
// that section (internal/engage/buddy/AGENTS.md).
//
// SSR-safe (see UserResource): nothing loads on the server, and the window state is
// only ever written from browser interactions. What survives a reload is which windows
// are open, in sessionStorage — never a message body. Unsent drafts live in memory only
// and go with the session.

import { browser } from '$app/env';
import { api } from '#lib/api.js';
import { messageActionFor } from '#lib/buddies.js';
import {
  closeDockWindow,
  type DockCorner,
  dockCorner,
  dockHiddenOn,
  type DockWindow,
  DraftBook,
  openDockWindow,
  parseDockWindows,
  pruneDockWindows,
  setDockWindowMinimized,
} from '#lib/messagingDock.js';
import type { Conversation } from '#lib/types.js';
import { UserResource } from '#lib/userResource.svelte.js';

const WINDOWS_KEY = 'hire.messaging-dock-windows';

function readWindows(): DockWindow[] {
  if (!browser) return [];
  try { return parseDockWindows(sessionStorage.getItem(WINDOWS_KEY)); } catch { return []; }
}

class MessagingDockStore extends UserResource<Conversation[]> {
  #conversations = $state.raw<Conversation[]>([]);
  #windows = $state.raw<DockWindow[]>(readWindows());
  #generation = 0;
  #drafts = new DraftBook();
  /** Whether the dock's conversation list is unfolded. */
  expanded = $state(false);
  /** The buddy the first-message dialog is aimed at, or null when it is closed. */
  inviteHandle = $state<string | null>(null);

  /** Whether the dock is needed at all: there is a conversation to show, or a first
   *  message is being written (its dialog lives in the dock). */
  get active(): boolean { return this.#conversations.length > 0 || this.inviteHandle !== null; }
  get conversations(): Conversation[] { return this.#conversations; }
  get windows(): DockWindow[] { return this.#windows; }

  protected load(): Promise<Conversation[]> { return api.listConversations(); }

  protected apply(data: Conversation[]): void {
    this.#conversations = data;
    // A declined or blocked conversation leaves the list; its window goes with it. Only
    // pruned against a list that was actually read, so a restored window survives until then.
    this.#setWindows(pruneDockWindows(this.#windows, new Set(data.map((c) => c.id))));
  }

  protected clearState(): void {
    this.#generation++;
    this.#conversations = [];
    this.#setWindows([]);
    this.#drafts.clear();
    this.expanded = false;
    this.inviteHandle = null;
  }

  /** Re-read the list now. Rejects on a failure, so a caller with its own error state can
   *  show it; a poll can ignore it. */
  async refresh(): Promise<void> {
    const generation = this.#generation;
    const data = await this.load();
    if (generation !== this.#generation) return; // signed out (or reset) meanwhile
    this.apply(data);
    this.markLoaded();
  }

  /** Whether the dock renders on this page at all. */
  shownOn(pathname: string): boolean { return this.#conversations.length > 0 && !dockHiddenOn(pathname); }

  /** How much of the bottom-right corner the dock takes on this page. */
  cornerOn(pathname: string): DockCorner {
    return dockCorner(this.shownOn(pathname), this.expanded, this.#windows);
  }

  conversation(id: number): Conversation | undefined {
    return this.#conversations.find((c) => c.id === id);
  }

  /** Open a conversation's chat window (restoring it when minimized). */
  openConversation(id: number): void {
    this.#setWindows(openDockWindow(this.#windows, id));
  }

  /** "Message" for a buddy: their conversation's window when one is pending or open,
   *  otherwise the first-message dialog (a conversation closed by a leave is re-invited). */
  messageBuddy(handle: string): void {
    const action = messageActionFor(this.#conversations, handle);
    if (action.kind === 'invite') this.inviteHandle = handle;
    else this.openConversation(action.conversationId);
  }

  /** The first message went out: show the new pending conversation in a window. */
  async invited(conversation: Conversation): Promise<void> {
    this.#conversations = [conversation, ...this.#conversations.filter((c) => c.id !== conversation.id)];
    this.openConversation(conversation.id);
    await this.refresh().catch(() => { /* the poll catches up */ });
  }

  /** The composer text a conversation was left with, kept across minimize and remount. */
  draft(id: number): string { return this.#drafts.get(id); }
  setDraft(id: number, text: string): void { this.#drafts.set(id, text); }

  close(id: number): void { this.#setWindows(closeDockWindow(this.#windows, id)); }

  setMinimized(id: number, minimized: boolean): void {
    this.#setWindows(setDockWindowMinimized(this.#windows, id, minimized));
  }

  #setWindows(windows: DockWindow[]): void {
    this.#windows = windows;
    if (!browser) return;
    try {
      if (windows.length) sessionStorage.setItem(WINDOWS_KEY, JSON.stringify(windows));
      else sessionStorage.removeItem(WINDOWS_KEY);
    } catch { /* storage unavailable: the windows last for this page load */ }
  }
}

export const messagingDock = new MessagingDockStore();
