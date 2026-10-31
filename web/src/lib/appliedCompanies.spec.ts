import { beforeEach, describe, expect, it, vi } from 'vitest';

// The store only runs its load in a browser; the components project's stub says otherwise.
vi.mock('$app/env', () => ({ browser: true }));

const { listAppliedCompanySlugs } = vi.hoisted(() => ({ listAppliedCompanySlugs: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { listAppliedCompanySlugs } }));

import { resetUserStores } from '#lib/userResource.svelte.js';
import {
  ensureAppliedCompaniesLoaded,
  hasAppliedToCompany,
  markAppliedToCompany,
} from './appliedCompanies.svelte';

describe('appliedCompanies', () => {
  beforeEach(() => {
    resetUserStores();
    listAppliedCompanySlugs.mockReset();
  });

  it('marks every posting of an employer the server says was applied to', async () => {
    listAppliedCompanySlugs.mockResolvedValue(['acme', 'globex']);
    await ensureAppliedCompaniesLoaded();
    expect(hasAppliedToCompany('acme')).toBe(true);
    expect(hasAppliedToCompany('initech')).toBe(false);
  });

  it('never matches a posting with no employer', async () => {
    listAppliedCompanySlugs.mockResolvedValue(['']);
    await ensureAppliedCompaniesLoaded();
    expect(hasAppliedToCompany('')).toBe(false);
    expect(hasAppliedToCompany(undefined)).toBe(false);
  });

  it('picks up a fresh application without a reload, and ignores an empty slug', async () => {
    listAppliedCompanySlugs.mockResolvedValue([]);
    await ensureAppliedCompaniesLoaded();
    markAppliedToCompany('acme');
    markAppliedToCompany('');
    expect(hasAppliedToCompany('acme')).toBe(true);
    expect(hasAppliedToCompany('')).toBe(false);
  });

  it('leaves no badge when the load fails', async () => {
    listAppliedCompanySlugs.mockRejectedValue(new Error('offline'));
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    await ensureAppliedCompaniesLoaded();
    expect(hasAppliedToCompany('acme')).toBe(false);
  });

  it('forgets the previous account on sign-out', async () => {
    listAppliedCompanySlugs.mockResolvedValue(['acme']);
    await ensureAppliedCompaniesLoaded();
    resetUserStores();
    expect(hasAppliedToCompany('acme')).toBe(false);
  });
});
