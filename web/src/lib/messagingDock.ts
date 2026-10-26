// The messaging dock's window arithmetic, kept free of Svelte runes so it unit-tests in
// plain node. The reactive store is `messagingDock.svelte.ts`; the markup is
// `components/MessagingDock.svelte`.

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

/** On a narrow screen one chat at a time is a full-width sheet: the newest window that is
 *  not minimized. Other open windows sit under it, minimized ones stack as bars above the
 *  list. `null` when every window is minimized. */
export function mobileSheetId(windows: readonly DockWindow[]): number | null {
  for (let i = windows.length - 1; i >= 0; i--) {
    const w = windows[i];
    if (w && !w.minimized) return w.id;
  }
  return null;
}

/** Bars stacked in the corner on a narrow screen: the list bar plus one per minimized window. */
function stackedBars(windows: readonly DockWindow[]): number {
  return 1 + windows.filter((w) => w.minimized).length;
}

/** The bottom padding the page reserves so the dock's bars never cover its last line: one
 *  row on a wide screen (the bars sit side by side), and on a narrow one the list bar plus
 *  a row per minimized window stacked above it. Empty when the dock is not shown;
 *  `wideOnly` is a page whose own mobile sticky bar owns the corner, where the dock only
 *  appears from `lg`. Literal class names, so Tailwind's scan finds every one. */
export function dockReserveClass(shown: boolean, wideOnly: boolean, windows: readonly DockWindow[]): string {
  if (!shown) return '';
  if (wideOnly) return 'lg:pb-12';
  const rows = stackedBars(windows);
  if (rows >= 3) return 'pb-44 sm:pb-12';
  if (rows === 2) return 'pb-28 sm:pb-12';
  return 'pb-12';
}

/** How high the support toast rises to clear the dock's bars, by the same row count. */
export function dockToastOffsetClass(windows: readonly DockWindow[]): string {
  const rows = stackedBars(windows);
  if (rows >= 3) return 'bottom-48 sm:bottom-16';
  if (rows === 2) return 'bottom-32 sm:bottom-16';
  return 'bottom-16';
}

/** Unsent composer text per conversation, for this page load only: held in memory and
 *  never written to storage, since a draft is message text. It lives in the store rather
 *  than the pane, so minimizing a window (which unmounts its pane), Escape, or moving
 *  between conversations and surfaces does not throw it away. */
export class DraftBook {
  #drafts = new Map<number, string>();

  get(id: number): string { return this.#drafts.get(id) ?? ''; }

  /** A draft emptied (sent, or deleted by hand) is forgotten. */
  set(id: number, text: string): void {
    if (text) this.#drafts.set(id, text);
    else this.#drafts.delete(id);
  }

  clear(): void { this.#drafts.clear(); }
}
