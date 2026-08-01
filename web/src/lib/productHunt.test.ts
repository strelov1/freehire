import { describe, expect, it } from 'vitest';
import { LAUNCH_OPENS_AT, launchPhase } from './productHunt';

const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

describe('launchPhase', () => {
  it('announces the launch until the Product Hunt day opens', () => {
    expect(launchPhase(Date.UTC(2026, 7, 1))).toBe('before');
    expect(launchPhase(LAUNCH_OPENS_AT - 1)).toBe('before');
  });

  it('opens on Pacific midnight, not UTC midnight', () => {
    // 26 August 00:00 UTC is still the 25th in California — the day has not opened.
    expect(launchPhase(Date.UTC(2026, 7, 26, 0, 0))).toBe('before');
    expect(launchPhase(LAUNCH_OPENS_AT)).toBe('live');
  });

  it('stays live for the whole 24-hour day', () => {
    expect(launchPhase(LAUNCH_OPENS_AT + 12 * HOUR)).toBe('live');
    expect(launchPhase(LAUNCH_OPENS_AT + DAY - 1)).toBe('live');
  });

  it('retires itself once the day is over, with no deploy', () => {
    expect(launchPhase(LAUNCH_OPENS_AT + DAY)).toBe('over');
    expect(launchPhase(Date.UTC(2026, 8, 15))).toBe('over');
  });
});
