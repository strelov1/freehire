import { describe, expect, it } from 'vitest';

import {
  arrivalAction,
  createDraftStore,
  PENDING_SIGNUP_MS,
  type DraftBackend,
  type OnboardingDraft,
} from './onboardingDraft';
import type { OnboardingParse } from './types';

function memoryBackend(): DraftBackend & { value: unknown } {
  const box = {
    value: undefined as unknown,
    get: () => Promise.resolve(box.value),
    put: (v: unknown) => {
      box.value = v;
      return Promise.resolve();
    },
    delete: () => {
      box.value = undefined;
      return Promise.resolve();
    },
  };
  return box;
}

function draft(savedAt: number): OnboardingDraft {
  return {
    file: new Blob(['%PDF'], { type: 'application/pdf' }),
    fileName: 'cv.pdf',
    parse: { first_name: 'Ana', skills: ['go'] } as unknown as OnboardingParse,
    answers: { where: 'remote' },
    savedAt,
  };
}

describe('onboarding draft store', () => {
  it('reads back what it saved', async () => {
    const backend = memoryBackend();
    const store = createDraftStore(() => Promise.resolve(backend));
    await store.save(draft(1000));
    const got = await store.load(1000 + 60_000);
    expect(got?.fileName).toBe('cv.pdf');
    expect(got?.answers).toEqual({ where: 'remote' });
  });

  it('ignores and deletes a draft older than a started sign-up may take', async () => {
    const backend = memoryBackend();
    const store = createDraftStore(() => Promise.resolve(backend));
    await store.save(draft(1000));
    expect(await store.load(1000 + PENDING_SIGNUP_MS + 1)).toBeNull();
    expect(backend.value).toBeUndefined();
  });

  it('clears the draft', async () => {
    const backend = memoryBackend();
    const store = createDraftStore(() => Promise.resolve(backend));
    await store.save(draft(1000));
    await store.clear();
    expect(backend.value).toBeUndefined();
    expect(await store.load(1000)).toBeNull();
  });

  it('treats a malformed record as no draft', async () => {
    const backend = memoryBackend();
    backend.value = { savedAt: 'yesterday' };
    const store = createDraftStore(() => Promise.resolve(backend));
    expect(await store.load(1000)).toBeNull();
  });

  // Private browsing can refuse IndexedDB outright; the flow then simply has no draft to
  // carry, and nothing it does here may throw.
  it('does nothing without a backend', async () => {
    const store = createDraftStore(() => Promise.resolve(null));
    await expect(store.save(draft(1000))).resolves.toBeUndefined();
    await expect(store.load(1000)).resolves.toBeNull();
    await expect(store.clear()).resolves.toBeUndefined();
  });

  it('swallows a failing backend', async () => {
    const failing: DraftBackend = {
      get: () => Promise.reject(new Error('quota')),
      put: () => Promise.reject(new Error('quota')),
      delete: () => Promise.reject(new Error('quota')),
    };
    const store = createDraftStore(() => Promise.resolve(failing));
    await expect(store.save(draft(1000))).resolves.toBeUndefined();
    await expect(store.load(1000)).resolves.toBeNull();
    await expect(store.clear()).resolves.toBeUndefined();
  });

  // startOver clears while a save from the last answer may still be in flight; the clear
  // must land last, or the late save resurrects the draft.
  it('applies operations in the order they were asked for', async () => {
    const backend = memoryBackend();
    const slowPut = backend.put;
    backend.put = (v: unknown) => new Promise((r) => setTimeout(() => void slowPut(v).then(r), 20));
    const store = createDraftStore(() => Promise.resolve(backend));
    void store.save(draft(1000));
    await store.clear();
    expect(backend.value).toBeUndefined();
  });

  it('has no IndexedDB backend in an environment without one', async () => {
    const { onboardingDraft } = await import('./onboardingDraft');
    await expect(onboardingDraft.load()).resolves.toBeNull();
  });
});

describe('arrivalAction', () => {
  const now = 10_000_000;
  const done = { where: 'remote', role: 'any', challenge: [] as string[] } as const;
  const pending = (over: Partial<OnboardingDraft> = {}): OnboardingDraft => ({
    ...draft(now - 60_000),
    answers: { ...done },
    pendingSignupAt: now - 60_000,
    ...over,
  });

  it('restores a fresh draft for a signed-out visitor — a sign-up left on the sign-in page', () => {
    expect(arrivalAction(pending(), { authenticated: false, accountCreatedAt: now, now })).toBe('restore');
  });

  it('discards a stale or unmarked draft even for a signed-out visitor', () => {
    const stale = pending({ pendingSignupAt: now - PENDING_SIGNUP_MS - 1 });
    expect(arrivalAction(stale, { authenticated: false, accountCreatedAt: now, now })).toBe('discard');
    const unmarked = pending({ pendingSignupAt: undefined });
    expect(arrivalAction(unmarked, { authenticated: false, accountCreatedAt: now, now })).toBe('discard');
  });

  it('carries a fresh sign-up into an account that had not onboarded', () => {
    expect(arrivalAction(pending(), { authenticated: true, accountCreatedAt: now, now })).toBe('carry');
  });

  // Migration 0207 cleared onboarding_completed_at for existing accounts, so an old account
  // can still look un-onboarded: only the creation time says the sign-up made it.
  it('treats an account created before the sign-up started as existing, onboarded or not', () => {
    const old = { authenticated: true, accountCreatedAt: now - 10 * 60_000, now };
    expect(arrivalAction(pending(), old)).toBe('offer-save');
    expect(arrivalAction(pending(), { authenticated: true, accountCreatedAt: null, now })).toBe('offer-save');
  });

  it('only offers to save into an account that already existed', () => {
    expect(arrivalAction(pending(), { authenticated: true, accountCreatedAt: now - 365 * 86_400_000, now })).toBe(
      'offer-save',
    );
  });

  // A shared browser: somebody else's anonymous run must never reach this account.
  it('discards a draft no sign-up was started from', () => {
    expect(
      arrivalAction(pending({ pendingSignupAt: undefined }), { authenticated: true, accountCreatedAt: now, now }),
    ).toBe('discard');
  });

  it('discards a sign-up marker older than half an hour', () => {
    const stale = pending({ pendingSignupAt: now - PENDING_SIGNUP_MS - 1 });
    expect(arrivalAction(stale, { authenticated: true, accountCreatedAt: now, now })).toBe('discard');
  });

  it('discards an unfinished draft for a signed-in visitor', () => {
    const unfinished = pending({ answers: { where: 'remote' } });
    expect(arrivalAction(unfinished, { authenticated: true, accountCreatedAt: now, now })).toBe('discard');
  });
});
