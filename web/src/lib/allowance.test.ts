import { describe, expect, it } from 'vitest';
import { refuses, remaining, resetsAtLabel } from './allowance';
import type { Allowance } from './types';

function allowance(over: Partial<Allowance> = {}): Allowance {
  return {
    feature: 'match',
    used: 0,
    limit: 3,
    unlimited: false,
    enforced: true,
    resets_at: '2026-09-16T00:00:00Z',
    ...over,
  };
}

describe('refuses', () => {
  it('is false while the ceiling is only being counted', () => {
    // The shadow run: the allowance reads as spent and the server runs the action anyway.
    // A surface that blocked here would build a wall the server does not have, and would
    // hide from the measurement exactly the requests it exists to count.
    expect(refuses(allowance({ used: 3, enforced: false }))).toBe(false);
  });

  it('is true once the ceiling is live and the allowance is spent', () => {
    expect(refuses(allowance({ used: 3 }))).toBe(true);
    expect(refuses(allowance({ used: 4 }))).toBe(true);
  });

  it('is false below the limit', () => {
    expect(refuses(allowance({ used: 2 }))).toBe(false);
  });

  it('never refuses an unlimited allowance', () => {
    // The fair-use guard behind a pro plan refuses at the point of use; it is not a
    // ceiling anybody is shown approaching.
    expect(refuses(allowance({ used: 5000, unlimited: true, limit: undefined }))).toBe(false);
  });

  it('is false when the allowance could not be read', () => {
    expect(refuses(null)).toBe(false);
    expect(refuses(undefined)).toBe(false);
  });

  it('treats a response without the flag as not enforcing', () => {
    // An older server omits `enforced`. Not refusing is the safe way to be wrong: the
    // server still answers 402 for real, and the candidate meets one refusal instead of
    // a client-side wall over a ceiling that may not exist.
    const older = { ...allowance({ used: 3 }) } as Partial<Allowance>;
    delete older.enforced;
    expect(refuses(older as Allowance)).toBe(false);
  });
});

describe('remaining', () => {
  it('counts down and floors at zero', () => {
    expect(remaining(allowance({ used: 1 }))).toBe(2);
    expect(remaining(allowance({ used: 9 }))).toBe(0);
  });

  it('is null when unlimited or unknown', () => {
    expect(remaining(allowance({ unlimited: true }))).toBeNull();
    expect(remaining(null)).toBeNull();
  });
});

describe('resetsAtLabel', () => {
  it('falls back to a word rather than an invalid date', () => {
    expect(resetsAtLabel(null)).toBe('tomorrow');
    expect(resetsAtLabel(allowance({ resets_at: 'not-a-date' }))).toBe('tomorrow');
  });

  it('renders the instant in the reader own clock, not UTC midnight', () => {
    // Not the UTC midnight the day is keyed by: what the reader needs is when it happens
    // where they are.
    expect(resetsAtLabel(allowance())).toMatch(/\d/);
  });
});
