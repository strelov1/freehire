// What a candidate was doing when the tailoring pre-flight sent them to buy Pro, kept across
// the payment provider's round trip so they can pick the same task up afterwards.
//
// sessionStorage, same tab only: the checkout redirects this very tab and comes back to it.
// Only the vacancy slug and the chosen base CV id are kept — never a title, CV text or an
// address — scoped to the account and bounded in time. It is a convenience: nothing here
// proves a purchase or an entitlement, and a missing or invalid record just means there is
// nothing to resume, never a blocked checkout.
import { track } from './analytics';
import { api } from './api';
import type { PublicPrice } from './types';

type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

export interface TailorContext {
  slug: string;
  baseCvId?: string;
}

const KEY = 'fh:tailor-context';
export const TAILOR_CONTEXT_TTL_MS = 2 * 60 * 60 * 1000;
// Both become part of a local URL, so only characters that cannot change its shape pass.
const SLUG = /^[a-z0-9][a-z0-9-]{0,199}$/;
const BASE_ID = /^[A-Za-z0-9_-]{1,64}$/;

/** The tab's own storage, or null where it is blocked (some privacy modes throw on access). */
export function browserStore(): Store | null {
  try {
    return typeof sessionStorage === 'undefined' ? null : sessionStorage;
  } catch {
    return null;
  }
}

export function saveTailorContext(userId: number, ctx: TailorContext, store: Store, now = Date.now()): void {
  try {
    store.setItem(KEY, JSON.stringify({ v: 1, userId, slug: ctx.slug, baseCvId: ctx.baseCvId, savedAt: now }));
  } catch {
    // Storage may be full or disabled; the purchase must go ahead without it.
  }
}

export function clearTailorContext(store: Store): void {
  try {
    store.removeItem(KEY);
  } catch {
    // Nothing to clean if storage is unavailable.
  }
}

/** The saved task for THIS account, or null. An invalid, foreign or expired record is
 *  removed so it cannot resurface for the next person on the tab. */
export function readTailorContext(userId: number | undefined, store: Store, now = Date.now()): TailorContext | null {
  try {
    const raw = store.getItem(KEY);
    if (raw === null) return null;
    const v: unknown = JSON.parse(raw);
    const ok =
      userId !== undefined &&
      !!v &&
      typeof v === 'object' &&
      'v' in v &&
      v.v === 1 &&
      'userId' in v &&
      v.userId === userId &&
      'savedAt' in v &&
      typeof v.savedAt === 'number' &&
      v.savedAt <= now &&
      now - v.savedAt <= TAILOR_CONTEXT_TTL_MS &&
      'slug' in v &&
      typeof v.slug === 'string' &&
      SLUG.test(v.slug) &&
      (!('baseCvId' in v) || v.baseCvId == null || (typeof v.baseCvId === 'string' && BASE_ID.test(v.baseCvId)));
    if (!ok) {
      clearTailorContext(store);
      return null;
    }
    const base = (v as { baseCvId?: string | null }).baseCvId;
    return base ? { slug: (v as { slug: string }).slug, baseCvId: base } : { slug: (v as { slug: string }).slug };
  } catch {
    clearTailorContext(store);
    return null;
  }
}

/** Remembers the task, then asks the provider for a checkout session at `price` and returns
 *  its URL. The record is written first and survives a failure, so the candidate can retry or
 *  come back to the task; a storage failure is swallowed. The event carries no identifiers. */
export async function startTailorCheckout(
  userId: number | undefined,
  ctx: TailorContext,
  price: PublicPrice,
  store: Store | null = browserStore(),
): Promise<string> {
  if (store && userId !== undefined) saveTailorContext(userId, ctx, store);
  track('contextual_pro_checkout_start', { surface: 'confirm_tailor', interval: price.interval });
  const { url } = await api.billingCheckout(price.id);
  return url;
}
