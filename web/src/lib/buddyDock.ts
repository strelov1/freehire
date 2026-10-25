// The buddy messaging dock's window arithmetic, kept free of Svelte runes so it unit-tests
// in plain node. The reactive store is `buddyDock.svelte.ts`; the markup is
// `components/BuddyMessagingDock.svelte`.

import type { BuddyMessageAction } from './buddies.js';

/** One chat window docked beside the conversation list. */
export interface DockWindow {
  id: number;
  minimized: boolean;
}

/** How many chat windows sit side by side on a wide screen. A narrow one shows only the
 *  newest, as a full-width sheet. */
const MAX_DOCK_WINDOWS = 2;

/** Open (or restore) a conversation's window. A window already open keeps its place and
 *  is un-minimized; a new one goes to the end, and the oldest closes past `max`. */
export function openDockWindow(windows: readonly DockWindow[], id: number, max = MAX_DOCK_WINDOWS): DockWindow[] {
  if (windows.some((w) => w.id === id)) {
    return windows.map((w) => (w.id === id ? { ...w, minimized: false } : w));
  }
  return [...windows, { id, minimized: false }].slice(-max);
}

export function closeDockWindow(windows: readonly DockWindow[], id: number): DockWindow[] {
  return windows.filter((w) => w.id !== id);
}

export function setDockWindowMinimized(windows: readonly DockWindow[], id: number, minimized: boolean): DockWindow[] {
  return windows.map((w) => (w.id === id ? { ...w, minimized } : w));
}

/** Drop the windows whose conversation the list no longer carries (declined, blocked). */
export function pruneDockWindows(windows: readonly DockWindow[], ids: ReadonlySet<number>): DockWindow[] {
  return windows.filter((w) => ids.has(w.id));
}

/** What the dock does for a "Message" press, from the one rule `messageActionFor`
 *  decides: an existing conversation (open or pending either way) opens its window, no
 *  conversation opens the first-message dialog, a closed one does nothing. */
type DockIntent = { kind: 'window'; conversationId: number } | { kind: 'invite' } | null;

export function dockIntentFor(action: BuddyMessageAction): DockIntent {
  switch (action.kind) {
    case 'invite': return { kind: 'invite' };
    case 'open':
    case 'sent':
    case 'reply': return { kind: 'window', conversationId: action.conversationId };
    case 'none': return null;
  }
}

export function totalUnread(conversations: readonly { unread: number }[]): number {
  return conversations.reduce((sum, c) => sum + c.unread, 0);
}

/** Which windows are open is all the dock keeps across a reload, in sessionStorage —
 *  never a message body. A malformed value reads as no windows. */
export function parseDockWindows(raw: string | null, max = MAX_DOCK_WINDOWS): DockWindow[] {
  if (!raw) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    const windows: DockWindow[] = [];
    for (const item of parsed) {
      if (typeof item !== 'object' || item === null) continue;
      const { id, minimized } = item as Record<string, unknown>;
      if (typeof id !== 'number' || !Number.isInteger(id) || id <= 0) continue;
      if (windows.some((w) => w.id === id)) continue;
      windows.push({ id, minimized: minimized === true });
    }
    return windows.slice(-max);
  } catch {
    return [];
  }
}

/** The dock stays off the buddies section's own page, whose Chats tab is the full-size
 *  list and pane — two copies of one conversation on one screen would each poll and
 *  mark it read. Every other page, the member pages included, gets the dock. */
export function dockHiddenOn(pathname: string): boolean {
  return pathname === '/my/buddies';
}

/** How much of the bottom-right corner the dock is taking, so the support toast that
 *  shares it can step aside: above the bars, or out of the way entirely while a list or
 *  chat is unfolded. */
export type DockCorner = 'none' | 'bars' | 'open';

export function dockCorner(shown: boolean, expanded: boolean, windows: readonly DockWindow[]): DockCorner {
  if (!shown) return 'none';
  return expanded || windows.some((w) => !w.minimized) ? 'open' : 'bars';
}
