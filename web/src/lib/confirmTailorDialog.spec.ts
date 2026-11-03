import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { CvBaseItem } from '#lib/cv.js';

const { getJobMatch, myPlan, listBaseCvs } = vi.hoisted(() => ({
  getJobMatch: vi.fn(),
  myPlan: vi.fn(),
  listBaseCvs: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ api: { getJobMatch, myPlan, listBaseCvs } }));

import {
  askConfirmTailor,
  chooseConfirmTailorBase,
  confirmTailorDialog,
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
