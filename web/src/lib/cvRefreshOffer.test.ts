import { afterEach, describe, expect, it } from 'vitest';

import {
  BASE_REFRESH_MESSAGE,
  CV_REFRESH_DISMISSED_KEY,
  TAILOR_REFRESH_MESSAGE,
  dismissCvRefreshOffer,
  isCvRefreshDismissed,
  offerCvRefresh,
} from './cvRefreshOffer';
import { must } from './utils';

function memStorage(initial: Record<string, string> = {}): Storage {
  const data = { ...initial };
  return {
    get length() {
      return Object.keys(data).length;
    },
    clear() {
      for (const k of Object.keys(data)) Reflect.deleteProperty(data, k);
    },
    getItem(key: string) {
      return Object.prototype.hasOwnProperty.call(data, key) ? must(data[key]) : null;
    },
    key() {
      return null;
    },
    removeItem(key: string) {
      Reflect.deleteProperty(data, key);
    },
    setItem(key: string, value: string) {
      data[key] = value;
    },
  };
}

afterEach(() => {
  if (typeof sessionStorage !== 'undefined') sessionStorage.clear();
});

describe('isCvRefreshDismissed', () => {
  it('is false until the candidate declines', () => {
    expect(isCvRefreshDismissed(memStorage())).toBe(false);
  });

  it('is true after dismiss', () => {
    const s = memStorage();
    dismissCvRefreshOffer(s);
    expect(isCvRefreshDismissed(s)).toBe(true);
    expect(s.getItem(CV_REFRESH_DISMISSED_KEY)).toBe('1');
  });
});

describe('offerCvRefresh', () => {
  it('applies when the candidate agrees and does not dismiss', async () => {
    const applied: string[] = [];
    const result = await offerCvRefresh({
      message: TAILOR_REFRESH_MESSAGE,
      confirm: () => true,
      dismissed: false,
      apply: async () => {
        applied.push('yes');
      },
    });
    expect(result).toBe('applied');
    expect(applied).toEqual(['yes']);
  });

  it('declines without applying and records dismiss', async () => {
    const s = memStorage();
    const applied: string[] = [];
    const result = await offerCvRefresh({
      message: BASE_REFRESH_MESSAGE,
      confirm: () => false,
      dismissed: false,
      onDismiss: () => dismissCvRefreshOffer(s),
      apply: async () => {
        applied.push('yes');
      },
    });
    expect(result).toBe('declined');
    expect(applied).toEqual([]);
    expect(isCvRefreshDismissed(s)).toBe(true);
  });

  it('skips confirm and apply when already dismissed this session', async () => {
    let asked = false;
    const result = await offerCvRefresh({
      message: TAILOR_REFRESH_MESSAGE,
      confirm: () => {
        asked = true;
        return true;
      },
      dismissed: true,
      apply: async () => {
        throw new Error('must not apply');
      },
    });
    expect(result).toBe('skipped');
    expect(asked).toBe(false);
  });
});
