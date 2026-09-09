// Pure derivations over a roast response, kept out of +page.svelte so they can be unit
// tested directly (web/'s vitest config runs in plain Node with no Svelte plugin — see
// page.test.ts's own note). Neither function has a component to mount, and neither needs
// one: both are total functions over the wire shape.

import type { RoastResponse } from '$lib/api';
import type { Gap } from '$lib/types';

/** Whether the reading is scoped to a named role, and what to show for it. Reads
 *  `market_scoped` rather than testing `role` for emptiness — the two can disagree in
 *  neither direction today, but the backend's own contract (cv_roast.go) is explicit
 *  that `market_scoped` is the field to trust, so this mirrors that rather than
 *  re-deriving it. */
export interface RoleDisplay {
  scoped: boolean;
  role: string;
}

export function roleDisplay(res: Pick<RoastResponse, 'role' | 'market_scoped'>): RoleDisplay {
  return { scoped: res.market_scoped, role: res.role };
}

/** The market line's data, or `null` when the backend could not answer it
 *  (`market_available` false). Returning `null` — never a zeroed object — is what stops
 *  the page rendering "0 of 0 roles" as though it were a real measurement: the caller
 *  must branch on this being `null` and show an unavailability note instead. */
export interface MarketLine {
  covered: number;
  total: number;
  coveragePercent: number;
  /** The single highest-yield missing skill, or `null` when the CV's skills already
   *  cover every gap the role has (an empty `gaps` array is a real outcome, not a
   *  missing one). */
  topGap: Gap | null;
}

export function marketLine(res: Pick<RoastResponse, 'market_available' | 'market'>): MarketLine | null {
  if (!res.market_available || !res.market) return null;
  const { covered, total, coverage_percent: coveragePercent, gaps } = res.market;
  return { covered, total, coveragePercent, topGap: gaps[0] ?? null };
}
