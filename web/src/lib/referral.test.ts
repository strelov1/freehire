import { describe, expect, it } from 'vitest';
import { PROMO_COOKIE, REF_COOKIE, captureRef, capturePromo } from './referral';

const VALID_REF = 'AbCdEfGhIjKlMnOp';

describe('captureRef', () => {
  it('stores a well-formed code when nothing is held', () => {
    expect(captureRef(VALID_REF, undefined)).toEqual({ name: REF_COOKIE, value: VALID_REF });
  });

  it('keeps the first code when a second link is opened', () => {
    // First toucher wins. Otherwise a link somebody talks a pending invitee into opening
    // takes over an attribution that has already been earned.
    expect(captureRef('ZzZzZzZzZzZzZzZz', VALID_REF)).toBeNull();
  });

  it('ignores a code the invite table could never hold', () => {
    expect(captureRef('short', undefined)).toBeNull();
    expect(captureRef('has spaces in it and is long', undefined)).toBeNull();
  });

  it('ignores an absent parameter', () => {
    expect(captureRef(null, undefined)).toBeNull();
  });
});

describe('capturePromo', () => {
  it('folds a code up, the way the table stores it', () => {
    expect(capturePromo(' zztest90 ')).toEqual({ name: PROMO_COOKIE, value: 'ZZTEST90' });
  });

  it('lets a later code win', () => {
    // Unlike the invite code: this one only prefills a form field, so the most recent
    // offer somebody clicked is the one they meant.
    expect(capturePromo('ZZTEST50')).toEqual({ name: PROMO_COOKIE, value: 'ZZTEST50' });
  });

  it('ignores a code the promo table could never hold', () => {
    expect(capturePromo('!!')).toBeNull();
    expect(capturePromo('a'.repeat(40))).toBeNull();
  });
});
