// Pure session-list logic for the assistant sidebar: label derivation and the
// add/remove/select reducers. Kept out of the Svelte component so it is
// unit-testable (vitest) without a DOM — mirroring how `chat.ts` isolates
// `reduceTurnEvent`. Everything here is pure; the fetches live in `api.ts`.
//
// Ordering is the backend's: `GET /assistant/sessions` returns the caller's
// conversations most-recently-active first, so the list needs no client-side sort
// and carries no timestamps.

import type { SessionSummary } from './wire';

/** A session as rendered in the sidebar. */
export interface SessionItem {
  id: string;
  label: string;
  preset: string;
}

const MAX_LABEL = 60;

/** Turn a first user message into a compact one-line sidebar label
 *  (whitespace collapsed, trimmed, truncated with an ellipsis). */
export function labelFromMessage(text: string): string {
  const oneLine = text.replace(/\s+/g, ' ').trim();
  if (oneLine.length <= MAX_LABEL) return oneLine;
  return oneLine.slice(0, MAX_LABEL - 1).trimEnd() + '…';
}

/** Map a wire row to a `SessionItem`. The backend names a session after its first
 *  user message; a conversation with no turns yet has none, so the caller supplies
 *  a fallback. */
export function fromSummary(s: SessionSummary, fallback: string): SessionItem {
  const label = s.label?.trim() ? labelFromMessage(s.label) : fallback;
  return { id: s.id, label, preset: s.preset };
}

/** Insert or replace a session by id (no duplicates), newest first. */
export function upsertSession(items: SessionItem[], item: SessionItem): SessionItem[] {
  return [item, ...items.filter((i) => i.id !== item.id)];
}

/** Drop the session with `id` (no-op if absent). */
export function removeSession(items: SessionItem[], id: string): SessionItem[] {
  return items.filter((i) => i.id !== id);
}

/** Set the label of the session with `id`, leaving the rest untouched. */
export function setLabel(items: SessionItem[], id: string, label: string): SessionItem[] {
  return items.map((i) => (i.id === id ? { ...i, label } : i));
}

/** Which session should be active after a deletion. `remaining` is the list
 *  AFTER removal. If the deleted session was the active one, activate the newest
 *  remaining (or `null` if none are left, so the caller can start a fresh one);
 *  otherwise keep `currentActive`. */
export function activeAfterDelete(
  remaining: SessionItem[],
  deletedWasActive: boolean,
  currentActive: string | null,
): string | null {
  if (!deletedWasActive) return currentActive;
  return remaining[0]?.id ?? null;
}
