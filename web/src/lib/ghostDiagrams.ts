// Pure models for the two computed diagrams on the /features/ghost-jobs landing: the
// prevalence waffle and the gate matrix. Kept out of the Svelte components so the parts
// that can be wrong quietly — cell accounting, and which level a gate combination
// produces — are unit-testable without rendering. Same split as activityChart.ts, and for
// the same reason.
//
// The other four diagrams have no model here on purpose. They are drawings with literal
// coordinates: nothing about them can be computed, so a model would be ceremony.

import { CRITERIA, WITNESS_GATE, ghostLevel } from './ghost';
import type { GhostLevel } from './ghost';

/** The share of listings nobody is working to fill, as published rather than averaged.
 *  Studies put it between these bounds; Greenhouse, which can see its own customers'
 *  pipelines, reported 18–22% per quarter. A single figure would claim a precision the
 *  sources do not have. */
export const PREVALENCE = { low: 18, high: 27 } as const;

/** One cell per percentage point, so the grid reads as "this many in a hundred" with no
 *  scale to interpret. */
export const WAFFLE_CELLS = 100;

/** `solid` — the floor every study agrees on. `banded` — the width of the disagreement.
 *  `empty` — the rest of the catalogue.
 *
 *  Three states rather than two because the band is the honest part: drawn like the floor
 *  it would overstate what is known, and drawn like the remainder it would hide it. */
export type WaffleCell = 'solid' | 'banded' | 'empty';

/** prevalenceWaffle lays the range out over a fixed hundred cells, floor first.
 *
 *  The bounds are clamped rather than trusted. They are published copy, and copy gets
 *  edited — a range widened past the grid should still yield something drawable instead
 *  of a row that runs off the component. */
export function prevalenceWaffle(range: { low: number; high: number }): WaffleCell[] {
  const low = clamp(range.low);
  const high = clamp(Math.max(range.high, low));

  return Array.from({ length: WAFFLE_CELLS }, (_, i) => {
    if (i < low) return 'solid';
    return i < high ? 'banded' : 'empty';
  });
}

function clamp(n: number): number {
  return Math.min(Math.max(Math.round(n), 0), WAFFLE_CELLS);
}

/** One cell of the gate matrix: which gates its example passes, the example that gets it
 *  there, and the level the rule returns for that example.
 *
 *  The example travels with the cell so the diagram can be checked against `ghostLevel`
 *  rather than against a caption. */
export interface GateCell {
  converged: boolean;
  witnessed: boolean;
  criteria: string[];
  contributors: number;
  level: GhostLevel;
}

const byTier = (tier: string): string[] =>
  CRITERIA.filter((c) => c.tier === tier).map((c) => c.code);

/** The gates a cell passes are declared, not recomputed — recomputing them here would be
 *  a second copy of `ghostLevel`'s predicates, free to disagree with it. The example
 *  beside them is what makes the declaration checkable. */
const cell = (
  converged: boolean,
  witnessed: boolean,
  criteria: string[],
  contributors: number,
): GateCell => ({
  converged,
  witnessed,
  criteria,
  contributors,
  level: ghostLevel(criteria, contributors),
});

/** gateMatrix builds the four combinations of the two gates, asking the rule for each
 *  level rather than stating it.
 *
 *  Row-major from the corner that shows nothing: neither gate, convergence only, witnesses
 *  only, both. The top-right cell — structural convergence with nobody to witness it — is
 *  the one the page is really about, because no amount of posting shape moves it down to
 *  the last one. */
export function gateMatrix(): GateCell[] {
  const structural = byTier('structural');
  const outcome = byTier('outcome');

  return [
    cell(false, false, [], 0),
    cell(true, false, structural, 0),
    cell(false, true, outcome.slice(0, 1), WITNESS_GATE),
    cell(true, true, [...structural.slice(0, 1), ...outcome.slice(0, 1)], WITNESS_GATE),
  ];
}
