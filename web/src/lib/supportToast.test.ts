import { afterEach, describe, expect, it } from 'vitest';
import { LAUNCH_OPENS_AT, PH_DISMISSED_KEY } from './productHunt';
import {
  readDismissed,
  readPhBannerDismissed,
  shouldShow,
  SUPPORT_DISMISSED_KEY,
  writeDismissed,
} from './supportToast';

const DAY = 24 * 60 * 60 * 1000;

const BEFORE_LAUNCH = LAUNCH_OPENS_AT - DAY;
const AFTER_LAUNCH = LAUNCH_OPENS_AT + DAY + 1;

describe('shouldShow', () => {
  it('stays hidden while the Product Hunt strip is still asking', () => {
    expect(
      shouldShow({ now: BEFORE_LAUNCH, phBannerDismissed: false, selfDismissed: false }),
    ).toBe(false);
  });

  it('appears once the visitor has closed the Product Hunt strip', () => {
    expect(
      shouldShow({ now: BEFORE_LAUNCH, phBannerDismissed: true, selfDismissed: false }),
    ).toBe(true);
  });

  it('appears after the launch day, when the strip can no longer be closed', () => {
    expect(
      shouldShow({ now: AFTER_LAUNCH, phBannerDismissed: false, selfDismissed: false }),
    ).toBe(true);
  });

  it('stays hidden once the visitor has answered it', () => {
    expect(
      shouldShow({ now: AFTER_LAUNCH, phBannerDismissed: true, selfDismissed: true }),
    ).toBe(false);
  });
});

// The node environment has no localStorage at all, so each test installs the one it
// needs and removes it afterwards.
function installStorage(store: Map<string, string>) {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    },
  });
}

/** A blocked origin or Safari private mode: every access throws. */
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

describe('dismissal', () => {
  it('round-trips through storage', () => {
    installStorage(new Map());

    expect(readDismissed()).toBe(false);
    writeDismissed();
    expect(readDismissed()).toBe(true);
  });

  it('reads the Product Hunt strip’s own key', () => {
    installStorage(new Map([[PH_DISMISSED_KEY, '1']]));

    expect(readPhBannerDismissed()).toBe(true);
    expect(readDismissed()).toBe(false);
  });

  it('reads as unanswered when storage is unavailable', () => {
    installHostileStorage();

    expect(readDismissed()).toBe(false);
    expect(readPhBannerDismissed()).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileStorage();

    expect(() => writeDismissed()).not.toThrow();
  });

  it('keeps the two surfaces on separate keys', () => {
    expect(SUPPORT_DISMISSED_KEY).not.toBe(PH_DISMISSED_KEY);
  });
});
