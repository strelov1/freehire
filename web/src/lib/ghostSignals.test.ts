import { describe, expect, it } from 'vitest';
import { CRITERIA } from './ghost';
import { GHOST_SIGNALS } from './ghostSignals';

describe('GHOST_SIGNALS', () => {
  // The point of this test: a criterion cannot join the product without being
  // explained on the landing. A page that silently falls behind the thing it
  // describes is worse than no page.
  it('explains every criterion the classifier can report', () => {
    expect(GHOST_SIGNALS.map((s) => s.code)).toEqual(CRITERIA.map((c) => c.code));
    for (const s of GHOST_SIGNALS) {
      expect(s.fact, `${s.code} has no example fact`).toBeTruthy();
      expect(s.gist, `${s.code} has no short summary`).toBeTruthy();
      expect(s.why, `${s.code} has no explanation`).toBeTruthy();
    }
  });

  // `gist` is what the reader sees without expanding anything; `why` is the full
  // account, one disclosure away. The ceiling is not a style rule — it is the failure
  // mode: a gist that grows into a second `why` rebuilds the wall of grey the page was
  // restructured to remove, and does it without anything going red.
  it('keeps the visible summary short', () => {
    for (const s of GHOST_SIGNALS) {
      const words = s.gist.trim().split(/\s+/).length;
      expect(words, `${s.code} gist runs ${words} words`).toBeLessThanOrEqual(40);
      expect(words, `${s.code} gist is shorter than the fact it summarises`).toBeGreaterThan(5);
    }
  });

  it('keeps both tiers represented', () => {
    const tiers = new Set(GHOST_SIGNALS.map((s) => s.tier));
    expect(tiers).toEqual(new Set(['structural', 'outcome']));
  });

  // The gates were pinned here when the page stated them in prose. They are pinned in
  // ghost.test.ts now, beside the rule that reads them — the matrix interpolates the
  // constants instead of quoting them, so there is no second copy left to keep honest.

  // Nothing here may claim the system knows what an employer intends: it observes
  // postings and outcomes, and that limit is the whole reason the wording hedges.
  it('never claims to know an employer intent', () => {
    for (const s of GHOST_SIGNALS) {
      expect(`${s.label} ${s.fact} ${s.gist} ${s.why}`.toLowerCase()).not.toMatch(
        /\bfake\b|\bscam\b|not really hiring|no intention/,
      );
    }
  });
});
