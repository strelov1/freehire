// The centralized "save this search, then get its new jobs in Telegram" flow, as a
// pure module: no Svelte runes and no store singletons imported at load (vitest runs
// plain Node), so the save half takes its store port as a dep — the component wires
// the real singleton, tests pass a fake. Saving is the primary, standalone action
// (it works without any subscription); the Telegram alert is a follow-on offered only
// once the search is saved. The localStorage handoff carries the save intent across a
// full-page OAuth redirect (mirrors filterStorage.ts's guards).

import { ApiError } from './api';
import { canonicalQuery, filtersFromParams } from './facetModel';
import { FACETS, dynamicLabel } from './facets';
import type { SavedSearch, Subscription } from './types';

// ---- auto-name ----

// The facets whose value most identifies a search, in name order. A new saved search
// is named from the first few; dedupe-by-query means the exact string is not critical.
const NAME_FACETS = ['category', 'seniority', 'work_mode', 'regions', 'skills'];

function labelFor(param: string, value: string): string {
  const def = FACETS.find((f) => f.param === param);
  const o = def?.options?.find((op) => op.value === value);
  return o ? o.label : dynamicLabel(param, value);
}

/** A short, human name for the saved search a query implies — the text query and a
 *  few identifying facet values, capped at the 100-char server limit, with a stable
 *  fallback for an empty ("all jobs") query. */
export function alertName(query: string): string {
  const f = filtersFromParams(new URLSearchParams(query));
  const parts: string[] = [];
  if (f.q.trim()) parts.push(f.q.trim());
  for (const param of NAME_FACETS) {
    if (parts.length >= 3) break;
    const st = f.facets[param];
    if (st?.include.length) parts.push(st.include.map((v) => labelFor(param, v)).join('/'));
  }
  const name = parts.slice(0, 3).join(' · ').slice(0, 100);
  return name || 'Job alert';
}

// ---- dedupe ----

/** The saved search whose canonical query matches this query, if any — so the flow
 *  reuses an existing set instead of creating a duplicate. */
export function matchedSavedSearch(query: string, items: SavedSearch[]): SavedSearch | undefined {
  const target = canonicalQuery(query);
  return items.find((s) => canonicalQuery(s.query) === target);
}

// ---- the save ----

export interface SavedSearchesPort {
  ensureLoaded(): Promise<void>;
  readonly items: SavedSearch[];
  create(name: string, query: string): Promise<SavedSearch>;
}

/** Reuse the saved search matching this query, or create one (auto-named), tolerating
 *  a 409 (concurrent save → reuse; name collision → retry with a numbered suffix).
 *  The standalone save action — no Telegram involved. */
export async function ensureSaved(query: string, ss: SavedSearchesPort): Promise<SavedSearch> {
  await ss.ensureLoaded();
  const existing = matchedSavedSearch(query, ss.items);
  if (existing) return existing;
  const base = alertName(query);
  try {
    return await ss.create(base, query);
  } catch (e) {
    if (!(e instanceof ApiError) || e.status !== 409) throw e;
    const raced = matchedSavedSearch(query, ss.items);
    if (raced) return raced;
    return ss.create(`${base.slice(0, 90)} (${ss.items.length + 1})`, query);
  }
}

// ---- the subscribe ----

export interface SubscriptionsPort {
  ensureLoaded(): Promise<void>;
  /** The held subscription for this saved search and channel, if any. */
  find(savedSearchId: number, channel: string): Subscription | undefined;
  /** Create the subscription and keep the row the server returns. */
  add(savedSearchId: number, channel: string): Promise<void>;
  /** Re-read every subscription and replace what is held. */
  refresh(): Promise<void>;
}

/** Subscribe a saved search to a channel, tolerating a 409 the way the spec asks:
 *  "an 'already subscribed' conflict is treated as success". The save half's rule
 *  (ensureSaved, above) written for the other half of the same flow.
 *
 *  A conflict is repaired by re-reading rather than ignored. Ignoring it leaves the
 *  caller holding a list without the row the server has, so whatever renders from that
 *  list still draws the channel "off" and the next tap conflicts again — which is how
 *  this reached a user as a raw "already subscribed to this saved search on this
 *  channel". The load is awaited first for the same reason: a write racing an in-flight
 *  load is discarded by the snapshot that lands after it. */
export async function ensureSubscribed(
  savedSearchId: number,
  channel: string,
  subs: SubscriptionsPort,
): Promise<void> {
  await subs.ensureLoaded();
  if (subs.find(savedSearchId, channel)) return;
  try {
    await subs.add(savedSearchId, channel);
  } catch (e) {
    if (!(e instanceof ApiError) || e.status !== 409) throw e;
    await subs.refresh();
  }
}

// ---- OAuth-redirect handoff (carries the save intent) ----

export const PENDING_ALERT_KEY = 'hire.pendingAlert';

/** Record the query the user wanted to save, to survive a full-page OAuth redirect.
 *  Best-effort (private mode / SSR never throw). */
export function setPendingAlert(query: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(PENDING_ALERT_KEY, query);
  } catch {
    // best-effort
  }
}

/** Read the pending query and clear it (consume exactly once), or null if none. An
 *  empty string is a valid ("all jobs") save and is returned as ''. */
export function consumePendingAlert(): string | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const v = localStorage.getItem(PENDING_ALERT_KEY);
    if (v !== null) localStorage.removeItem(PENDING_ALERT_KEY);
    return v;
  } catch {
    return null;
  }
}
