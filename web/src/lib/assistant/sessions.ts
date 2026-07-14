// Pure session-list logic for the assistant sidebar: label derivation, ordering,
// and the add/remove/select reducers. Kept out of the Svelte component so it is
// unit-testable (vitest) without a DOM — mirroring how `chat.ts` isolates
// `reduceTurnEvent`. Everything here is pure; localStorage/label caching and the
// backend fetches live in the component and `api.ts`.

/** One row of the owner-scoped `GET /sessions` response. `project_id`,
 *  `agent_name`, and `tags` also come over the wire but the sidebar ignores
 *  them, so they are omitted from this shape. */
export interface SessionSummary {
  session_id: string;
  display_label: string | null;
  created_at: number;
  live: boolean;
}

/** A session as rendered in the sidebar. */
export interface SessionItem {
  id: string;
  label: string;
  createdAt: number;
  live: boolean;
}

const MAX_LABEL = 60;

/** Turn a first user message into a compact one-line sidebar label
 *  (whitespace collapsed, trimmed, truncated with an ellipsis). */
export function labelFromMessage(text: string): string {
  const oneLine = text.replace(/\s+/g, ' ').trim();
  if (oneLine.length <= MAX_LABEL) return oneLine;
  return oneLine.slice(0, MAX_LABEL - 1).trimEnd() + '…';
}

/** Resolve a session's display label by priority: a derived/cached label (e.g.
 *  from the first user message) > the backend `display_label` > the supplied
 *  timezone-formatted `fallback`. Blank strings are ignored so an empty label is
 *  never shown. */
export function resolveLabel(
  s: SessionSummary,
  cached: string | undefined,
  fallback: string,
): string {
  if (cached && cached.trim()) return cached;
  if (s.display_label && s.display_label.trim()) return s.display_label;
  return fallback;
}

/** Map a wire row to a `SessionItem`, resolving its label. */
export function fromSummary(
  s: SessionSummary,
  cached: string | undefined,
  fallback: string,
): SessionItem {
  return {
    id: s.session_id,
    label: resolveLabel(s, cached, fallback),
    createdAt: s.created_at,
    live: s.live,
  };
}

/** Newest-first by `createdAt`, without mutating the input. */
export function newestFirst(items: SessionItem[]): SessionItem[] {
  return [...items].sort((a, b) => b.createdAt - a.createdAt);
}

/** Insert or replace a session by id (no duplicates), keeping newest-first order. */
export function upsertSession(items: SessionItem[], item: SessionItem): SessionItem[] {
  return newestFirst([item, ...items.filter((i) => i.id !== item.id)]);
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
 *  remaining (or `null` if none are left, so the caller can spawn a fresh one);
 *  otherwise keep `currentActive`. */
export function activeAfterDelete(
  remaining: SessionItem[],
  deletedWasActive: boolean,
  currentActive: string | null,
): string | null {
  if (!deletedWasActive) return currentActive;
  return newestFirst(remaining)[0]?.id ?? null;
}
