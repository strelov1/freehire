import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { CvBaseItem } from '#lib/cv.js';

const { getJobMatch, myPlan, listBaseCvs, plans } = vi.hoisted(() => ({
  getJobMatch: vi.fn(),
  myPlan: vi.fn(),
  listBaseCvs: vi.fn(),
  plans: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ api: { getJobMatch, myPlan, listBaseCvs, plans } }));

import {
  askConfirmTailor,
  chooseConfirmTailorBase,
  confirmTailorDialog,
  refreshConfirmTailorAllowance,
  settleConfirmTailorDialog,
} from './confirmTailorDialog.svelte';

const base = (id: string, isDefault = false): CvBaseItem => ({
  id,
  title: `CV ${id}`,
  template_id: 'classic-ats',
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-01T10:00:00Z',
  is_default: isDefault,
  hand_edited: false,
});

/** Lets the dialog's own fire-and-forget reads settle. */
const flush = () => new Promise((r) => setTimeout(r, 0));

beforeEach(() => {
  getJobMatch.mockResolvedValue({});
  myPlan.mockResolvedValue({ allowances: [] });
  listBaseCvs.mockReset();
  plans.mockReset();
});

describe('askConfirmTailor base choice', () => {
  it('asks for no base when the candidate has only one — no extra step', async () => {
    listBaseCvs.mockResolvedValue([base('a', true)]);
    const result = askConfirmTailor('slug-1');
    await flush();
    settleConfirmTailorDialog(true);
    expect(await result).toEqual({});
  });

  it('preselects the default base and resolves with the one picked when there are two', async () => {
    listBaseCvs.mockResolvedValue([base('a'), base('b', true)]);
    const result = askConfirmTailor('slug-2');
    await flush();
    expect(confirmTailorDialog.baseCvId).toBe('b');
    chooseConfirmTailorBase('a');
    settleConfirmTailorDialog(true);
    expect(await result).toEqual({ baseCvId: 'a' });
  });

  it('resolves null when cancelled', async () => {
    listBaseCvs.mockResolvedValue([base('a'), base('b', true)]);
    const result = askConfirmTailor('slug-3');
    await flush();
    settleConfirmTailorDialog(false);
    expect(await result).toBeNull();
  });

  it('still lets the candidate proceed, on the default, when the bases cannot be read', async () => {
    listBaseCvs.mockRejectedValue(new Error('offline'));
    const result = askConfirmTailor('slug-4');
    await flush();
    settleConfirmTailorDialog(true);
    expect(await result).toEqual({});
  });
});

describe('askConfirmTailor Pro offer', () => {
  const monthly = { id: 'price_m', tier: 'pro', interval: 'month', amount_cents: 500, currency: 'usd' };
  const yearly = { ...monthly, id: 'price_y', interval: 'year', amount_cents: 4800 };

  it('keeps a newer plan read when an older one fails later', async () => {
    const tailor = { feature: 'tailor', used: 0, unlimited: false };
    let failOld!: (e: Error) => void;
    myPlan
      .mockReturnValueOnce(new Promise((_, j) => (failOld = j)))
      .mockResolvedValueOnce({ plan: 'free', allowances: [tailor] });
    plans.mockResolvedValue({ prices: [monthly] });
    listBaseCvs.mockResolvedValue([base('a', true)]);
    const result = askConfirmTailor('senior-go-acme-1');
    refreshConfirmTailorAllowance();
    await flush();
    failOld(new Error('stale'));
    await flush();
    expect(confirmTailorDialog.allowance).toEqual(tailor);
    settleConfirmTailorDialog(false);
    await result;
  });

  it('offers the monthly Pro price to a free account whose plan has no tailoring sessions', async () => {
    myPlan.mockResolvedValue({ plan: 'free', allowances: [{ feature: 'tailor', used: 0, unlimited: false }] });
    plans.mockResolvedValue({ prices: [yearly, monthly] });
    listBaseCvs.mockResolvedValue([base('a', true)]);
    const result = askConfirmTailor('senior-go-acme-1');
    await flush();
    expect(confirmTailorDialog.proOffer).toEqual(monthly);
    expect(confirmTailorDialog.slug).toBe('senior-go-acme-1');
    expect(confirmTailorDialog.baseCvId).toBe('a');
    settleConfirmTailorDialog(false);
    await result;
  });

  it('makes no offer to an account that already has Pro, and does not read prices', async () => {
    myPlan.mockResolvedValue({ plan: 'pro', allowances: [{ feature: 'tailor', used: 0, limit: 5, unlimited: false }] });
    listBaseCvs.mockResolvedValue([base('a', true)]);
    const result = askConfirmTailor('slug-5');
    await flush();
    expect(confirmTailorDialog.proOffer).toBeNull();
    expect(plans).not.toHaveBeenCalled();
    settleConfirmTailorDialog(false);
    await result;
  });

  it('makes no offer when the price is not for sale or cannot be read', async () => {
    myPlan.mockResolvedValue({ plan: 'free', allowances: [{ feature: 'tailor', used: 0, unlimited: false }] });
    plans.mockRejectedValue(new Error('offline'));
    listBaseCvs.mockResolvedValue([]);
    const result = askConfirmTailor('slug-6');
    await flush();
    expect(confirmTailorDialog.proOffer).toBeNull();
    settleConfirmTailorDialog(false);
    await result;
  });

  it('clears a previous offer when the next ask is for an account that cannot be offered one', async () => {
    myPlan.mockResolvedValueOnce({ plan: 'free', allowances: [{ feature: 'tailor', used: 0, unlimited: false }] });
    plans.mockResolvedValue({ prices: [monthly] });
    listBaseCvs.mockResolvedValue([]);
    const first = askConfirmTailor('slug-7');
    await flush();
    expect(confirmTailorDialog.proOffer).toEqual(monthly);
    settleConfirmTailorDialog(false);
    await first;
    myPlan.mockResolvedValueOnce({ plan: 'pro', allowances: [] });
    const second = askConfirmTailor('slug-8');
    expect(confirmTailorDialog.proOffer).toBeNull();
    await flush();
    expect(confirmTailorDialog.proOffer).toBeNull();
    settleConfirmTailorDialog(false);
    await second;
  });
});
