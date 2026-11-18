// The tracking board's pure halves: laying rows out into columns, the filters it mirrors
// into the URL, and what the filters and sort leave on screen. JobBoard owns the state.
import {
  BOARD_COLUMNS,
  columnOf,
  parseBoardSort,
  sortBoardItems,
  type BoardColumnId,
  type BoardItem,
  type BoardSort,
} from '#lib/board.js';
import type { MyJob } from '#lib/types.js';

export type BoardColumns = Record<BoardColumnId, BoardItem[]>;

export function emptyColumns(): BoardColumns {
  return { unstaged: [], preparing: [], applied: [], interview: [], offer: [], closed: [] };
}

/**
 * Lay the fetched rows out into the columns, with each card's column (cardCol) so a
 * finalize can tell which card actually moved into a zone.
 *
 * The 'board' filter returns saved ∪ applied ∪ stage, and a saved-only row lands in the
 * no-stage inbox (see columnOf) rather than being dropped here — saving a job is taking it
 * on, so the board is where it belongs.
 *
 * Those rows always counted toward the 500 cap; showing them makes an existing consequence
 * visible rather than creating it. A user with hundreds of bookmarks can push older active
 * applications outside the fetched window, and now sees a wide inbox column while it
 * happens. Still acceptable at this scale; the fix if it bites is the same one as before —
 * a server-side filter that pages the board's columns separately, so a long backlog cannot
 * crowd out the live applications.
 */
export function placeOnBoard(rows: MyJob[]): { columns: BoardColumns; cardCol: Record<string, BoardColumnId> } {
  const columns = emptyColumns();
  const cardCol: Record<string, BoardColumnId> = {};
  for (const row of rows) {
    const item: BoardItem = { ...row, id: row.id };
    const col = columnOf(item);
    if (!col) continue; // neither saved nor tracked — nothing to place
    columns[col].push(item);
    cardCol[item.id] = col;
  }
  return { columns, cardCol };
}

export type BoardFilters = { q: string; attention: boolean; stage: string; sort: BoardSort };

/**
 * Search, the "needs attention" toggle and the stage filter, mirrored into
 * `?q=`/`?attention=1`/`?stage=` so a shared or reloaded link shows the same rows. One
 * UrlSyncedState over all three — not separate ones — because each instance's write replaces
 * the WHOLE query string with its own serialization; independent states on the same page
 * would each erase the others' params on every change. It writes the URL synchronously on
 * every keystroke/click (the structural fix for the dropped-character race) and seeds from
 * location.search rather than page.url, which lags after a shallow-routing back/forward.
 * Filtering is local and instant, so there is nothing to debounce — setNow writes and
 * applies together, and the view reads `value`.
 */
export const BOARD_FILTERS = {
  parse: (p: URLSearchParams): BoardFilters => ({
    q: p.get('q') ?? '',
    attention: p.get('attention') === '1',
    stage: p.get('stage') ?? '',
    sort: parseBoardSort(p.get('sort')),
  }),
  serialize: (v: BoardFilters) =>
    new URLSearchParams({
      ...(v.q ? { q: v.q } : {}),
      ...(v.attention ? { attention: '1' } : {}),
      ...(v.stage ? { stage: v.stage } : {}),
      ...(v.sort !== 'default' ? { sort: v.sort } : {}),
    }),
};

/** Each column's cards that pass the filters (all of them with no filter), sorted. */
export function shownColumns(
  columns: BoardColumns,
  matches: ((item: BoardItem) => boolean) | null,
  sort: BoardSort,
): BoardColumns {
  return Object.fromEntries(
    BOARD_COLUMNS.map((c) => [c.id, sortBoardItems(matches ? columns[c.id].filter(matches) : columns[c.id], sort)]),
  ) as BoardColumns;
}

/** The list view's order: most recent activity first by default, else the chosen sort. */
export function listOrder(shown: BoardColumns, sort: BoardSort): BoardItem[] {
  return sort === 'default'
    ? Object.values(shown)
        .flat()
        .sort((a, b) => (b.last_activity_at ?? '').localeCompare(a.last_activity_at ?? ''))
    : sortBoardItems(Object.values(shown).flat(), sort);
}
