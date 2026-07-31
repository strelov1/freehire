import { describe, expect, it } from 'vitest';
import { CONVERGENCE, CRITERIA, WITNESS_GATE, ghostLevel } from './ghost';
import { PREVALENCE, WAFFLE_CELLS, gateMatrix, prevalenceWaffle } from './ghostDiagrams';

describe('prevalenceWaffle', () => {
  it('always fills the whole grid', () => {
    expect(prevalenceWaffle(PREVALENCE)).toHaveLength(WAFFLE_CELLS);
  });

  // The lower bound is what every study agrees on; the cells above it are the width of
  // the disagreement. Drawing them alike would claim a precision the sources do not have,
  // on a page whose whole argument is that it states only what it can check.
  it('separates the agreed floor from the uncertain band', () => {
    const cells = prevalenceWaffle(PREVALENCE);
    const count = (kind: string) => cells.filter((c) => c === kind).length;

    expect(count('solid')).toBe(PREVALENCE.low);
    expect(count('banded')).toBe(PREVALENCE.high - PREVALENCE.low);
    expect(count('empty')).toBe(WAFFLE_CELLS - PREVALENCE.high);
  });

  it('orders the grid so the band sits on top of the floor', () => {
    const cells = prevalenceWaffle(PREVALENCE);
    expect(cells.slice(0, PREVALENCE.low).every((c) => c === 'solid')).toBe(true);
    expect(cells.slice(PREVALENCE.high).every((c) => c === 'empty')).toBe(true);
  });

  // A published figure is copy, and copy gets edited. The grid must degrade rather than
  // spill: a range someone widens past the grid still yields a drawable hundred cells.
  it('never spills past the grid', () => {
    expect(prevalenceWaffle({ low: 40, high: 130 })).toHaveLength(WAFFLE_CELLS);
    expect(prevalenceWaffle({ low: 0, high: 0 })).toHaveLength(WAFFLE_CELLS);
  });
});

describe('gateMatrix', () => {
  it('covers every combination of the two gates once', () => {
    const cells = gateMatrix();
    expect(cells).toHaveLength(4);
    expect(new Set(cells.map((c) => `${c.converged}:${c.witnessed}`)).size).toBe(4);
  });

  // The cells are asked, not captioned. A hand-written label is a second statement of the
  // rule, free to drift from the one the product runs on — which is the drift this whole
  // page exists to prevent.
  it('takes every level from the rule itself', () => {
    for (const cell of gateMatrix()) {
      expect(ghostLevel(cell.criteria, cell.contributors)).toBe(cell.level);
    }
  });

  // The axes are labels, and a label can be put on the wrong cell. Checking the level
  // came from the rule does not catch that: a mislabelled axis leaves every level
  // correct and the diagram still wrong.
  it('labels each axis by what its example actually does', () => {
    const outcome = new Set<string>(
      CRITERIA.filter((c) => c.tier === 'outcome').map((c) => c.code),
    );
    for (const cell of gateMatrix()) {
      expect(cell.criteria.length >= CONVERGENCE, `converged on ${cell.criteria}`).toBe(
        cell.converged,
      );
      const witnesses =
        cell.contributors >= WITNESS_GATE && cell.criteria.some((c) => outcome.has(c));
      expect(witnesses, `witnessed on ${cell.criteria}`).toBe(cell.witnessed);
    }
  });

  it('shows the ceiling on structural evidence', () => {
    const structural = new Set<string>(
      CRITERIA.filter((c) => c.tier === 'structural').map((c) => c.code),
    );
    for (const cell of gateMatrix()) {
      if (cell.criteria.every((code) => structural.has(code))) {
        expect(cell.level, 'structural-only cells must never read likely').not.toBe('likely');
      }
    }
  });

  it('reaches likely in exactly one cell', () => {
    expect(gateMatrix().filter((c) => c.level === 'likely')).toHaveLength(1);
    expect(gateMatrix().filter((c) => c.level === 'none')).toHaveLength(1);
    expect(gateMatrix().filter((c) => c.level === 'possible')).toHaveLength(2);
  });
});
