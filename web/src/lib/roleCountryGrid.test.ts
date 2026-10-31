import { describe, expect, it } from 'vitest';
import type { InsightRoleCountryCell } from './api';
import { MIN_PAIR_OPEN } from './roleLandings';
import {
  ALL_COUNTRIES,
  GROWTH_MIN_BASE,
  buildGrid,
  cellTarget,
  growthLevel,
  growthPercent,
  roundedGrowth,
  shareLevel,
} from './roleCountryGrid';

const cell = (category: string, country: string, open: number, prev = open): InsightRoleCountryCell => ({
  category,
  country,
  open_count: open,
  open_count_prev: prev,
});

describe('buildGrid', () => {
  const grid = buildGrid(
    [
      cell('design', '', 10),
      cell('design', 'us', 4),
      cell('backend', '', 100),
      cell('backend', 'de', 30),
      cell('backend', 'us', 60),
      cell('other', '', 500),
      cell('management', '', 400),
    ],
    ['us', 'de'],
  );

  it('puts the all-countries column first, then the endpoint order', () => {
    expect(grid.columns).toEqual([ALL_COUNTRIES, 'us', 'de']);
  });

  it('orders rows by all-countries open count, keeping only categories with a landing', () => {
    // `other` is unsearchable and `management` retired: rows whose every link would
    // open an empty search are not rows at all.
    expect(grid.rows.map((r) => r.category)).toEqual(['backend', 'design']);
  });

  it('leaves a cell the endpoint did not send undefined, not zero', () => {
    const design = grid.rows.find((r) => r.category === 'design');
    expect(design?.cells.get('de')).toBeUndefined();
    expect(design?.cells.get('us')).toEqual({ open: 4, prev: 4 });
  });

  it('totals each column so a share is relative to its country', () => {
    expect(grid.columnTotals.get('us')).toBe(64);
    // Totals count the cells the endpoint sent, dropped rows included: a country's
    // share is of everything posted there, not of the rows we chose to draw.
    expect(grid.columnTotals.get(ALL_COUNTRIES)).toBe(1010);
  });

  it('records the largest share in the grid for the shading scale', () => {
    // backend in de: 30/30 = 1.
    expect(grid.maxShare).toBe(1);
  });
});

describe('growthPercent', () => {
  it('is the change over the prior count', () => {
    expect(growthPercent({ open: 150, prev: 100 })).toBeCloseTo(50);
    expect(growthPercent({ open: 50, prev: 100 })).toBeCloseTo(-50);
  });

  it('is null under the base floor, where a percentage is noise', () => {
    expect(growthPercent({ open: 40, prev: GROWTH_MIN_BASE - 1 })).toBeNull();
    expect(growthPercent({ open: 40, prev: 0 })).toBeNull();
    expect(growthPercent({ open: 40, prev: GROWTH_MIN_BASE })).not.toBeNull();
  });
});

describe('roundedGrowth', () => {
  it('rounds to a whole percent, and keeps null under the floor', () => {
    expect(roundedGrowth({ open: 1006, prev: 1000 })).toBe(1);
    expect(roundedGrowth({ open: 994, prev: 1000 })).toBe(-1);
    expect(roundedGrowth({ open: 10, prev: GROWTH_MIN_BASE - 1 })).toBeNull();
  });
});

describe('shareLevel', () => {
  it('never draws a non-empty cell as empty', () => {
    expect(shareLevel(0.0001, 1)).toBe(1);
  });

  it('is zero only for nothing', () => {
    expect(shareLevel(0, 1)).toBe(0);
  });

  it('reaches the top level at the largest share', () => {
    expect(shareLevel(0.4, 0.4)).toBe(4);
  });
});

describe('growthLevel', () => {
  it('is neutral for no figure and for small moves', () => {
    expect(growthLevel(null)).toBe(0);
    expect(growthLevel(3)).toBe(0);
    expect(growthLevel(-3)).toBe(0);
  });

  it('is signed and saturates', () => {
    expect(growthLevel(20)).toBe(1);
    expect(growthLevel(-60)).toBe(-2);
    expect(growthLevel(400)).toBe(3);
    expect(growthLevel(-99)).toBe(-3);
  });
});

describe('cellTarget', () => {
  const big = 2 * MIN_PAIR_OPEN;

  it('opens the role landing for a large cell of a landing category', () => {
    expect(cellTarget('backend', 'de', big)).toEqual({ kind: 'pair', category: 'backend', country: 'germany' });
  });

  it('opens the search for a cell under twice the landing floor', () => {
    expect(cellTarget('backend', 'de', big - 1)).toEqual({ kind: 'search', category: 'backend', country: 'de' });
  });

  it('opens the search for a country with no slug', () => {
    expect(cellTarget('backend', 'zz', big)).toEqual({ kind: 'search', category: 'backend', country: 'zz' });
  });

  it('opens the category country table for the all-countries cell', () => {
    expect(cellTarget('backend', ALL_COUNTRIES, 5)).toEqual({ kind: 'category', category: 'backend' });
  });
});
