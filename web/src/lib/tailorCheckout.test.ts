import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicPrice } from './types';

const { billingCheckout, track } = vi.hoisted(() => ({ billingCheckout: vi.fn(), track: vi.fn() }));
vi.mock('./api', () => ({ api: { billingCheckout } }));
vi.mock('./analytics', () => ({ track }));

import {
  clearTailorContext,
  readTailorContext,
  saveTailorContext,
  startTailorCheckout,
  TAILOR_CONTEXT_TTL_MS,
} from './tailorCheckout';

function storage() {
  const values = new Map<string, string>();
  return {
    values,
    getItem: (k: string) => values.get(k) ?? null,
    setItem: (k: string, v: string) => void values.set(k, v),
    removeItem: (k: string) => void values.delete(k),
  };
}
const brokenStore = {
  getItem: () => {
    throw new Error('denied');
  },
  setItem: () => {
    throw new Error('denied');
  },
  removeItem: () => {
    throw new Error('denied');
  },
};
const price = { id: 'price_m', amount_cents: 500, currency: 'usd', interval: 'month', tier: 'pro' } as PublicPrice;
const now = Date.parse('2026-11-16T10:00:00Z');

describe('tailor context storage', () => {
  it('returns exactly the vacancy and base that were saved, for the same account', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'senior-go-acme-1', baseCvId: 'base-2' }, store, now);
    expect(readTailorContext(7, store, now + 1000)).toEqual({ slug: 'senior-go-acme-1', baseCvId: 'base-2' });
  });

  it('keeps a context without a base', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'a-1' }, store, now);
    expect(readTailorContext(7, store, now)).toEqual({ slug: 'a-1' });
  });

  it('does not hand one account the context of another, and drops it', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'a-1' }, store, now);
    expect(readTailorContext(8, store, now)).toBeNull();
    expect(store.values.size).toBe(0);
  });

  it('does not return a signed-out reader anything', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'a-1' }, store, now);
    expect(readTailorContext(undefined, store, now)).toBeNull();
  });

  it('expires a stale context and drops it', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'a-1' }, store, now);
    expect(readTailorContext(7, store, now + TAILOR_CONTEXT_TTL_MS + 1)).toBeNull();
    expect(store.values.size).toBe(0);
  });

  it('rejects a context saved in the future (clock tampering)', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'a-1' }, store, now + 60_000);
    expect(readTailorContext(7, store, now)).toBeNull();
  });

  it.each([
    ['not json', '{'],
    ['an array', '[]'],
    ['a missing slug', JSON.stringify({ v: 1, userId: 7, savedAt: now })],
    ['a path-like slug', JSON.stringify({ v: 1, userId: 7, slug: '../my/cvs', savedAt: now })],
    ['an absolute-URL slug', JSON.stringify({ v: 1, userId: 7, slug: 'https://evil.example/x', savedAt: now })],
    ['a query-injecting base', JSON.stringify({ v: 1, userId: 7, slug: 'a-1', baseCvId: 'x&y=1', savedAt: now })],
    ['a non-string base', JSON.stringify({ v: 1, userId: 7, slug: 'a-1', baseCvId: 5, savedAt: now })],
    ['another version', JSON.stringify({ v: 2, userId: 7, slug: 'a-1', savedAt: now })],
    ['a string account id', JSON.stringify({ v: 1, userId: '7', slug: 'a-1', savedAt: now })],
  ])('rejects %s', (_name, raw) => {
    const store = storage();
    store.setItem('fh:tailor-context', raw);
    expect(readTailorContext(7, store, now)).toBeNull();
    expect(store.values.size).toBe(0);
  });

  it('survives unavailable storage on every operation', () => {
    expect(() => saveTailorContext(7, { slug: 'a-1' }, brokenStore, now)).not.toThrow();
    expect(readTailorContext(7, brokenStore, now)).toBeNull();
    expect(() => clearTailorContext(brokenStore)).not.toThrow();
  });

  it('clears', () => {
    const store = storage();
    saveTailorContext(7, { slug: 'a-1' }, store, now);
    clearTailorContext(store);
    expect(readTailorContext(7, store, now)).toBeNull();
  });
});

describe('startTailorCheckout', () => {
  beforeEach(() => {
    billingCheckout.mockReset();
    track.mockReset();
  });

  it('stores the context before leaving, opens the monthly price and reports without personal data', async () => {
    const store = storage();
    billingCheckout.mockImplementation(async () => {
      // The task must already be recoverable by the time the provider is asked for a session.
      expect(readTailorContext(7, store)).toEqual({ slug: 'a-1', baseCvId: 'b-1' });
      return { url: 'https://checkout.example/s' };
    });
    const url = await startTailorCheckout(7, { slug: 'a-1', baseCvId: 'b-1' }, price, 'confirm_tailor', store);
    expect(url).toBe('https://checkout.example/s');
    expect(billingCheckout).toHaveBeenCalledWith('price_m');
    expect(track).toHaveBeenCalledWith('contextual_pro_checkout_start', {
      surface: 'confirm_tailor',
      interval: 'month',
    });
    expect(JSON.stringify(track.mock.calls)).not.toMatch(/a-1|b-1/);
  });

  it('reports the surface that started the checkout', async () => {
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    await startTailorCheckout(7, { slug: 'a-1' }, price, 'checkout_resume', storage());
    expect(track).toHaveBeenCalledWith('contextual_pro_checkout_start', {
      surface: 'checkout_resume',
      interval: 'month',
    });
  });

  it('still sends a user to checkout when storage is unavailable', async () => {
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    await expect(startTailorCheckout(7, { slug: 'a-1' }, price, 'confirm_tailor', brokenStore)).resolves.toBe(
      'https://checkout.example/s',
    );
  });

  it('keeps the context for a retry when the session cannot be created', async () => {
    const store = storage();
    billingCheckout.mockRejectedValue(new Error('down'));
    await expect(startTailorCheckout(7, { slug: 'a-1' }, price, 'confirm_tailor', store)).rejects.toThrow('down');
    expect(readTailorContext(7, store)).toEqual({ slug: 'a-1' });
  });

  it('does not store a context for an unknown account', async () => {
    const store = storage();
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    await startTailorCheckout(undefined, { slug: 'a-1' }, price, 'confirm_tailor', store);
    expect(store.values.size).toBe(0);
  });
});
