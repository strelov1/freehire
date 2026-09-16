import { afterEach, describe, expect, it } from 'vitest';
import {
  TALENT_INVITE_DISMISSED_KEY,
  dismissTalentInvite,
  isTalentInviteDismissed,
} from './talentInvite';

// The node environment has no localStorage at all, so each test installs the one it
// needs and removes it afterwards — the same shape cliPromo.test.ts uses.
function installStorage(store: Map<string, string>) {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    },
  });
}

/** A blocked origin or storage switched off: every access throws. */
function installHostileStorage() {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    get() {
      throw new Error('access denied');
    },
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, 'localStorage');
});

describe('Talent Network invitation dismissal', () => {
  it('round-trips through storage', () => {
    installStorage(new Map());

    expect(isTalentInviteDismissed()).toBe(false);
    dismissTalentInvite();
    expect(isTalentInviteDismissed()).toBe(true);
  });

  it('writes the kebab key its siblings use, not the camelCase one it shipped as', () => {
    const store = new Map<string, string>();
    installStorage(store);

    dismissTalentInvite();

    expect(store.get(TALENT_INVITE_DISMISSED_KEY)).toBe('1');
    expect(TALENT_INVITE_DISMISSED_KEY).toBe('hire.talent-invite-dismissed');
  });

  // The card mounts in the account shell, so an unguarded read would fail the mount of
  // every `my/*` page rather than one banner. Both directions are asserted: a throwing
  // read must answer "not dismissed", and a throwing write must not escape at all.
  it('reads as not dismissed when storage is unavailable', () => {
    installHostileStorage();

    expect(isTalentInviteDismissed()).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileStorage();

    expect(() => dismissTalentInvite()).not.toThrow();
  });
});
