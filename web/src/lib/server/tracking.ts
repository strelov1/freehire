import { serverApi } from './api';
import { rangeForMonth } from '$lib/calendarModel';

/** Fetch the caller's Kanban board rows for the tracking routes' server load, so
 *  the board renders with the page instead of after a client fetch on mount. A
 *  transient API failure returns undefined, letting JobBoard fall back to its own
 *  client fetch + friendly error state rather than 500ing the page. */
export async function loadBoard(fetchImpl: typeof fetch, cookie: string | null) {
  try {
    const board = await serverApi(fetchImpl, cookie).listMyJobs('board', 500, 0);
    return board.items;
  } catch {
    return undefined;
  }
}

/** Fetch the caller's application events for a month's grid, so the calendar paints with
 *  the page instead of after a client fetch on mount.
 *
 *  The month is the SERVER's, in UTC — this render cannot know the reader's zone. The
 *  margin rangeForMonth adds is what makes that safe: whatever the reader's offset, their
 *  own current month falls inside the span asked for here, and moving between months
 *  refetches from the browser where the zone is known.
 *
 *  A transient failure returns undefined, letting the view fall back to its own client
 *  fetch and a friendly error rather than 500ing the page — same contract as loadBoard. */
export async function loadTimeline(fetchImpl: typeof fetch, cookie: string | null) {
  const now = new Date();
  const { from, to } = rangeForMonth(now.getUTCFullYear(), now.getUTCMonth());
  try {
    return await serverApi(fetchImpl, cookie).myTimeline(from, to);
  } catch {
    return undefined;
  }
}
