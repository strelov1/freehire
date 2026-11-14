import type { MyJob, TrackedApplication } from '#lib/types.js';

/** Project a directly loaded application into the board's row shape. */
export function trackingItem(id: string, application: TrackedApplication): MyJob {
  return {
    id,
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
}
