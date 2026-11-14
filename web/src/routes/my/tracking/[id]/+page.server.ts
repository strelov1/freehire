import { error, redirect } from '@sveltejs/kit';
import { signinUrl } from '#lib/signin.js';
import { columnOf } from '#lib/board.js';
import { ApiError } from '#lib/api.js';
import { serverApi } from '#lib/server/api.js';
import type { MyJob } from '#lib/types.js';
import { loadBoard } from '#lib/server/tracking.js';
import type { PageServerLoad } from './$types';

// /my/tracking/[id] renders the tracking board with the application's drawer open
// (deep-linkable — the inbox links here, and a refresh/share reopens the same card).
// The board itself is for any signed-in user; the Emails tab inside the drawer
// self-gates to moderators. Guard auth like /my/tracking and pass the row id through.
// The board is server-fetched (same as /my/tracking) so opening a deep link paints
// the board + drawer in one round trip instead of a client fetch on mount.
export const load: PageServerLoad = async ({ parent, params, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
  const cookie = request.headers.get('cookie');
  const autoApply = url.searchParams.get('tab') === 'auto_apply';
  let board = await loadBoard(fetch, cookie);
  let initialItem: MyJob | undefined;
  if (autoApply && !board?.some((row) => row.id === params.id)) {
    // A live attempt can outlast its board placement or fall outside the first page.
    // Read this application directly rather than silently opening the bare board.
    let application;
    try {
      application = await serverApi(fetch, cookie).getTrackedApplication(params.id);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        redirect(302, `/jobs/${encodeURIComponent(params.id)}`);
      }
      error(503, 'Could not load this application. Please try again.');
    }
    const item: MyJob = {
      id: params.id,
      job: application.job,
      added_to_board_at: null,
      company_slug: application.job.company_slug ?? '',
      role_title: application.job.title,
      viewed_at: application.viewed_at ?? '',
      saved_at: application.saved_at,
      applied_at: application.applied_at,
      stage: application.stage ?? null,
      notes: application.notes ?? null,
      email_count: application.emails.length,
      last_activity_at: null,
      days_silent: null,
      silence_state: null,
      followed_up_at: application.followed_up_at,
      cv_opened_at: null,
      auto_apply_status: application.auto_apply?.status,
    };
    if (columnOf(item) === null) redirect(302, `/jobs/${encodeURIComponent(params.id)}`);
    if (board) board = [...board, item];
    else initialItem = item;
  }
  return { id: params.id, autoApply, board, initialItem };
};
