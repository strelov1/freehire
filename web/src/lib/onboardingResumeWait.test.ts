import { describe, expect, it } from 'vitest';
import { nextResumePollDelayMs, RESUME_POLL_BUDGET_MS } from './onboardingResumeWait';

describe('nextResumePollDelayMs', () => {
  it('starts under a second, so a parse that lands quickly is picked up on the next step', () => {
    expect(nextResumePollDelayMs(0)).toBeLessThan(1000);
  });

  it('never goes backwards, so the reads spread out rather than bunching up', () => {
    let previous = 0;
    for (let attempt = 0; ; attempt++) {
      const delay = nextResumePollDelayMs(attempt);
      if (delay === null) break;
      expect(delay).toBeGreaterThanOrEqual(previous);
      previous = delay;
    }
  });

  it('gives up, so a parse that never lands cannot poll for as long as the tab is open', () => {
    // The table's own length is what bounds this; the loop finds it rather than restating it.
    let attempt = 0;
    while (nextResumePollDelayMs(attempt) !== null) {
      attempt++;
      expect(attempt).toBeLessThan(1000); // a table that never ends would hang the test, not fail it
    }
    expect(attempt).toBeGreaterThan(0);
  });

  it('waits about a minute in total — long enough to outlast the wizard, short of forever', () => {
    expect(RESUME_POLL_BUDGET_MS).toBeGreaterThan(30_000);
    expect(RESUME_POLL_BUDGET_MS).toBeLessThan(180_000);
  });

  it('reports the same budget the table adds up to', () => {
    let total = 0;
    for (let attempt = 0; ; attempt++) {
      const delay = nextResumePollDelayMs(attempt);
      if (delay === null) break;
      total += delay;
    }
    expect(total).toBe(RESUME_POLL_BUDGET_MS);
  });
});
