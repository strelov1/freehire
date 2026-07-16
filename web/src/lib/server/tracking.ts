import { serverApi } from './api';

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
