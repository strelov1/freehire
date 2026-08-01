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
 *  The month is the one this PROCESS is in, which is not necessarily the reader's: a
 *  render at 18:00 in Los Angeles on 31 July happens on 1 August here. The margin
 *  rangeForMonth adds is sized for a day boundary and cannot absorb a month boundary, so
 *  the month fetched is reported back rather than assumed — the component compares it
 *  with the reader's own and refetches when they differ. Guessing instead would serve a
 *  July grid populated only in its last week, with nothing to say so.
 *
 *  A transient failure returns undefined, letting the view fall back to its own client
 *  fetch and a friendly error rather than 500ing the page — same contract as loadBoard. */
export async function loadTimeline(fetchImpl: typeof fetch, cookie: string | null) {
  const now = new Date();
  const year = now.getFullYear();
  const month = now.getMonth();
  const { from, to } = rangeForMonth(year, month);
  try {
    return { events: await serverApi(fetchImpl, cookie).myTimeline(from, to), year, month };
  } catch {
    return undefined;
  }
}
