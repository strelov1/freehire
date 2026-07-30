import { describe, expect, it } from 'vitest';
import { CRITERIA } from './ghost';
import { CONVERGENCE, GHOST_SIGNALS, WITNESS_GATE } from './ghostSignals';

describe('GHOST_SIGNALS', () => {
  // The point of this test: a criterion cannot join the product without being
  // explained on the landing. A page that silently falls behind the thing it
  // describes is worse than no page.
  it('explains every criterion the classifier can report', () => {
    expect(GHOST_SIGNALS.map((s) => s.code)).toEqual(CRITERIA.map((c) => c.code));
    for (const s of GHOST_SIGNALS) {
      expect(s.fact, `${s.code} has no example fact`).toBeTruthy();
      expect(s.why, `${s.code} has no explanation`).toBeTruthy();
    }
  });

  it('keeps both tiers represented', () => {
    const tiers = new Set(GHOST_SIGNALS.map((s) => s.tier));
    expect(tiers).toEqual(new Set(['structural', 'outcome']));
  });

  // The page states these numbers in prose. If the product changes them, the prose
  // becomes a lie, so they are read from one place rather than typed twice.
  it('pins the two gates the page describes', () => {
    expect(CONVERGENCE).toBe(2);
    expect(WITNESS_GATE).toBe(2);
  });

  // Nothing here may claim the system knows what an employer intends: it observes
  // postings and outcomes, and that limit is the whole reason the wording hedges.
  it('never claims to know an employer intent', () => {
    for (const s of GHOST_SIGNALS) {
      expect(`${s.label} ${s.fact} ${s.why}`.toLowerCase()).not.toMatch(
        /\bfake\b|\bscam\b|not really hiring|no intention/,
      );
    }
  });
});
