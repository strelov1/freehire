// The interview-buddies conversation list and the messaging dock's windows, as one
// module-level store: the dock in the root layout, the Chats tab and every "Message"
// control read the SAME list, so an unread count cannot disagree between two surfaces
// on the same screen.
//
// Membership is probed through the conversation list itself, which answers 403 to a
// non-member — never through GET /me/buddies, which touches the member's ranking keys
// and may only run on entering the section (internal/engage/buddy/AGENTS.md).
//
// SSR-safe (see UserResource): nothing loads on the server, and the window state is
// only ever written from browser interactions. What survives a reload is which windows
// are open, in sessionStorage — never a message body. Unsent drafts live in memory only
// and go with the session.

import { browser } from '$app/env';
import { ApiError, api } from '#lib/api.js';
import { messageActionFor } from '#lib/buddies.js';
import {
  closeDockWindow,
  type DockCorner,
  dockCorner,
  dockHiddenOn,
  dockIntentFor,
  type DockWindow,
  DraftBook,
  openDockWindow,
  parseDockWindows,
  pruneDockWindows,
  setDockWindowMinimized,
} from '#lib/buddyDock.js';
import type { BuddyConversation } from '#lib/types.js';
import { UserResource } from '#lib/userResource.svelte.js';

const WINDOWS_KEY = 'hire.buddy-dock-windows';

/** `null` is "not a member": the list answered 403. */
type Loaded = BuddyConversation[] | null;

function readWindows(): DockWindow[] {
  if (!browser) return [];
  try { return parseDockWindows(sessionStorage.getItem(WINDOWS_KEY)); } catch { return []; }
}

class BuddyDockStore extends UserResource<Loaded> {
  #member = $state(false);
  #conversations = $state.raw<BuddyConversation[]>([]);
  #windows = $state.raw<DockWindow[]>(readWindows());
  #generation = 0;
  #drafts = new DraftBook();
  /** Whether the dock's conversation list is unfolded. */
  expanded = $state(false);
  /** The member the first-message dialog is aimed at, or null when it is closed. */
  inviteHandle = $state<string | null>(null);

  get member(): boolean { return this.#member; }
  get conversations(): BuddyConversation[] { return this.#conversations; }
  get windows(): DockWindow[] { return this.#windows; }

  protected async load(): Promise<Loaded> {
    try { return await api.listBuddyConversations(); }
    catch (err) {
      if (err instanceof ApiError && err.status === 403) return null;
      throw err;
    }
  }

  protected apply(data: Loaded): void {
    this.#member = data !== null;
    this.#conversations = data ?? [];
    if (data === null) this.#setWindows([]);
    // A declined or blocked conversation leaves the list; its window goes with it. Only
    // pruned against a list that was actually read, so a restored window survives until then.
    else this.#setWindows(pruneDockWindows(this.#windows, new Set(data.map((c) => c.id))));
  }

  protected clearState(): void {
    this.#generation++;
    this.#member = false;
    this.#conversations = [];
    this.#setWindows([]);
    this.#drafts.clear();
    this.expanded = false;
    this.inviteHandle = null;
  }

  /** Re-read the list now. Rejects on a failure other than "not a member", so a caller
   *  with its own error state can show it; a poll can ignore it. */
  async refresh(): Promise<void> {
    const generation = this.#generation;
    const data = await this.load();
    if (generation !== this.#generation) return; // signed out (or rejoined) meanwhile
    this.apply(data);
    this.markLoaded();
  }

  /** Whether the dock renders on this page at all. */
  shownOn(pathname: string): boolean { return this.#member && !dockHiddenOn(pathname); }

  /** How much of the bottom-right corner the dock takes on this page. */
  cornerOn(pathname: string): DockCorner {
    return dockCorner(this.shownOn(pathname), this.expanded, this.#windows);
  }

  conversation(id: number): BuddyConversation | undefined {
    return this.#conversations.find((c) => c.id === id);
  }

  /** Open a conversation's chat window (restoring it when minimized). */
  openConversation(id: number): void {
    this.#setWindows(openDockWindow(this.#windows, id));
  }

  /** "Message" for a member: their conversation's window when one exists, otherwise the
   *  first-message dialog. A conversation closed by a leave is
   *  re-invited; a former member's offers nothing. */
  openNew(handle: string): void {
    const intent = dockIntentFor(messageActionFor(this.#conversations, handle));
    if (intent?.kind === 'window') this.openConversation(intent.conversationId);
    else if (intent?.kind === 'invite') this.inviteHandle = handle;
  }

  /** The first message went out: show the new pending conversation in a window. */
  async invited(conversation: BuddyConversation): Promise<void> {
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

export const buddyDock = new BuddyDockStore();
