// Where tapping a notification card navigates — the pure decision behind design.md's
// decision 5 (openspec/changes/add-notification-center), kept out of the component so
// it can be tested without mounting Svelte.

import type { NotificationItem } from './types';

export type NotificationTarget =
  | { kind: 'job'; slug: string }
  | { kind: 'tracking' }
  | { kind: 'digest'; id: number }
  | { kind: 'tailor'; slug: string }
  | { kind: 'none' };

/** A card with no `public_slug` and no `jobs` snapshot has nothing to open.
 *  `nudge_follow_up`/`nudge_interview_prep` point at the tracking board on web —
 *  matching the existing Telegram/email link target for those two kinds — even
 *  though the row itself always carries a slug. `auto_apply_tailor_ready` points
 *  at the tailoring workspace, not the job page — `/tailor/[slug]` idempotently
 *  resolves the same tailored CV a fresh tailoring bootstrap would (see
 *  openspec/changes/auto-apply-tailored-resume), so no CV id needs to travel with
 *  the notification. Every other slug-bearing kind (reminder, nudge_job_closed, a
 *  single-job subscription_digest) points at the job. A multi-job subscription
 *  digest carries no slug but does carry `jobs` — it points at its own jobs-list
 *  page instead. */
export function notificationTarget(
  item: Pick<NotificationItem, 'kind' | 'public_slug'> & Partial<Pick<NotificationItem, 'id' | 'jobs'>>,
): NotificationTarget {
  if (item.public_slug) {
    if (item.kind === 'nudge_follow_up' || item.kind === 'nudge_interview_prep') {
      return { kind: 'tracking' };
    }
    if (item.kind === 'auto_apply_tailor_ready') {
      return { kind: 'tailor', slug: item.public_slug };
    }
    return { kind: 'job', slug: item.public_slug };
  }
  if (item.kind === 'subscription_digest' && item.jobs && item.jobs.length > 0 && item.id != null) {
    return { kind: 'digest', id: item.id };
  }
  return { kind: 'none' };
}
