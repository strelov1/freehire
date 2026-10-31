import type { InsightRoleCountryCell } from './api';
import { countrySlug } from './facets';
import { MIN_PAIR_OPEN, categoryLandingLink } from './roleLandings';

/** The all-countries column's key — the empty string the endpoint uses for it. */
export const ALL_COUNTRIES = '';

/** Below this many postings a window ago, a percentage is noise: 3 → 9 is "+200%". The
 *  growth view prints "—" instead. Presentation only — the endpoint sends raw counts. */
export const GROWTH_MIN_BASE = 50;

/** A column whose open total is at least this multiple of its total a window ago is a
 *  country freehire started covering, not one whose market grew: on prod on 2026-10-31
 *  Taiwan's cells read +300% to +800% across every role the month its boards were added.
 *  No per-posting rule can tell the two apart (publication dates were tried and made it
 *  worse), so the grid prints no change for such a column at all. The figure sits in the
 *  gap the live columns showed that day: ordinary countries ×0.92–×1.46, the two whose
 *  boards were being onboarded ×1.94 (France) and ×6.99 (Taiwan). */
export const COVERAGE_JUMP_RATIO = 1.75;

/** Rows shown before "show all". */
export const VISIBLE_ROWS = 15;

export interface GridCell {
  open: number;
  prev: number;
}

interface GridRow {
  category: string;
  /** Keyed by country code, ALL_COUNTRIES included. A missing key is a cell the
   *  endpoint did not send — nothing open there now or a window ago. */
  cells: Map<string, GridCell>;
}

interface Grid {
  /** ALL_COUNTRIES, then the endpoint's countries in its order. */
  columns: string[];
  rows: GridRow[];
  /** Open postings per column, over every cell sent — rows not drawn included. */
  columnTotals: Map<string, number>;
  /** The largest cell-to-column share among drawn cells: the top of the shading scale. */
  maxShare: number;
  /** Columns whose coverage jumped (COVERAGE_JUMP_RATIO): their change is not shown. */
  coverageJumps: Set<string>;
}

/** Arranges the endpoint's cells into rows. Only categories with a role landing become
 *  rows: `other` is unsearchable and a retired category has no search either, so every
 *  link in such a row would open an empty page. Their postings still count toward the
 *  column totals, because a share is of everything posted in that country. */
export function buildGrid(cells: InsightRoleCountryCell[], countries: string[]): Grid {
  const columns = [ALL_COUNTRIES, ...countries];
  const columnTotals = new Map<string, number>();
  const columnPrevTotals = new Map<string, number>();
  const byCategory = new Map<string, Map<string, GridCell>>();
  for (const c of cells) {
    columnTotals.set(c.country, (columnTotals.get(c.country) ?? 0) + c.open_count);
    columnPrevTotals.set(c.country, (columnPrevTotals.get(c.country) ?? 0) + c.open_count_prev);
    if (!categoryLandingLink(c.category)) continue;
    let row = byCategory.get(c.category);
    if (!row) byCategory.set(c.category, (row = new Map()));
    row.set(c.country, { open: c.open_count, prev: c.open_count_prev });
  }

  const allOpen = (row: Map<string, GridCell>) => row.get(ALL_COUNTRIES)?.open ?? 0;
  const rows = [...byCategory.entries()]
    .map(([category, rowCells]) => ({ category, cells: rowCells }))
    .sort((a, b) => allOpen(b.cells) - allOpen(a.cells) || a.category.localeCompare(b.category));

  let maxShare = 0;
  for (const row of rows) {
    for (const [country, c] of row.cells) {
      maxShare = Math.max(maxShare, columnShare(c.open, columnTotals.get(country) ?? 0));
    }
  }
  const coverageJumps = new Set(
    columns.filter((country) => {
      const open = columnTotals.get(country) ?? 0;
      return open > 0 && open >= COVERAGE_JUMP_RATIO * (columnPrevTotals.get(country) ?? 0);
    }),
  );
  return { columns, rows, columnTotals, maxShare, coverageJumps };
}

/** A cell's part of its column: the category's share of that country's postings. */
export function columnShare(open: number, columnTotal: number): number {
  return columnTotal > 0 ? open / columnTotal : 0;
}

/** Change over the window in percent, or null when the base is under GROWTH_MIN_BASE. */
export function growthPercent(cell: GridCell): number | null {
  if (cell.prev < GROWTH_MIN_BASE) return null;
  return ((cell.open - cell.prev) / cell.prev) * 100;
}

/** The change as a whole percent, or null under the base floor — what the cell prints. */
export function roundedGrowth(cell: GridCell): number | null {
  const pct = growthPercent(cell);
  return pct === null ? null : Math.round(pct);
}

/** Shading step 0–4 for the open-count view, relative to the largest share drawn. A cell
 *  with anything in it is never step 0: an empty-looking square would claim nothing is
 *  posted there. */
export function shareLevel(share: number, maxShare: number): number {
  if (share <= 0 || maxShare <= 0) return 0;
  return Math.min(4, Math.max(1, Math.ceil((share / maxShare) * 4)));
}

/** Signed shading step −3…3 for the growth view. Moves under 5% stay neutral, and the
 *  scale saturates at 75% so a single +900% cell cannot wash out every other one. */
export function growthLevel(percent: number | null): number {
  if (percent === null) return 0;
  const size = Math.abs(percent);
  if (size < 5) return 0; // not `sign * 0`, which is -0 for a fall
  let step = 3;
  if (size < 25) step = 1;
  else if (size < 75) step = 2;
  return Math.sign(percent) * step;
}

/** Where a cell leads. A route, not an href: the component resolves it, so every link
 *  goes through `resolve()` like the rest of the site's. */
export type CellTarget =
  | { kind: 'pair'; category: string; country: string }
  | { kind: 'category'; category: string }
  | { kind: 'search'; category: string; country?: string };

/** The pair landing gates on the LIVE search count (MIN_PAIR_OPEN), while this grid reads
 *  the rollup, which can be a run stale and skips the search's description check — so a
 *  cell linked at the bare floor could point at a page that answers 404. Twice the floor
 *  is the margin; below it a cell opens the plain search, which always exists. */
export function cellTarget(category: string, country: string, open: number): CellTarget {
  const landing = categoryLandingLink(category);
  if (country === ALL_COUNTRIES) {
    return landing ? { kind: 'category', category: landing.slug } : { kind: 'search', category };
  }
  if (!landing) return { kind: 'search', category, country };
  const slug = countrySlug(country);
  if (slug && open >= 2 * MIN_PAIR_OPEN) return { kind: 'pair', category: landing.slug, country: slug };
  return { kind: 'search', category, country };
}
