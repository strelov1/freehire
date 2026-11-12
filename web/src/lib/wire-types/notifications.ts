import type { components } from '#lib/generated/api.js';
/** The account-level notification rule: whether notifications are on, and the
 *  channels to deliver over. Gates saved-job reminders and both lifecycle nudges
 *  (follow-up, interview-prep) — one shared rule, no per-kind or per-job control. */
export type NotificationSettings = components['schemas']['NotificationSettings'];

/** The notification-center event kinds a `user_notifications` row can carry — matches
 *  internal/engage/{notify,reminder,nudge}'s kind strings exactly (see
 *  openspec/changes/add-notification-center/design.md decision 1), plus two recorded
 *  directly by auto-apply's own write paths rather than by one of those background
 *  engines: `auto_apply_tailor_ready` (auto-apply-tailored-resume; historical rows only —
 *  auto-apply-review-tracking moved this notification to fire later, as
 *  `auto_apply_ready_for_review`, once there is an answer preview to review, not merely a
 *  tailored CV) and `auto_apply_ready_for_review` itself — and the three buddy mock
 *  interview notices internal/engage/buddy records (`buddy_mock_*`). */
export type NotificationKind =
  | 'subscription_digest'
  | 'reminder'
  | 'nudge_follow_up'
  | 'nudge_interview_prep'
  | 'nudge_job_closed'
  | 'nudge_auto_apply_submitted'
  | 'nudge_auto_apply_blocked'
  | 'nudge_auto_apply_failed'
  | 'auto_apply_tailor_ready'
  | 'auto_apply_ready_for_review'
  | 'buddy_mock_booked'
  | 'buddy_mock_cancelled'
  | 'buddy_mock_reminder'
  // Interview buddies: a reaction to one of your feed events (internal/engage/buddy), and
  // the messaging core's unread notices for a buddy conversation (buddy/kind.go).
  | 'buddy_reaction'
  | 'buddy_message'
  | 'buddy_invitation';

/** One matched job as recorded into a multi-job subscription digest's `jobs`
 *  snapshot — the same {title, company, slug} shape as everywhere else in this
 *  app, no internal id. */
interface NotificationDigestJob {
  title: string;
  company: string;
  slug: string;
}

/** One row in the notification center (GET /me/notifications) — a durable,
 *  readable-by-the-owner record of a delivery event, independent of which channel
 *  carried it. `public_slug` is the job it concerns, or null for a subscription
 *  digest that matched more than one job (nothing single to deep-link to) — that
 *  case instead carries `jobs`, a snapshot of what matched as of delivery (only
 *  present on the list response for a multi-job digest; absent/null otherwise).
 *  `read_at` is null until the caller marks it read. */
export interface NotificationItem {
  id: number;
  kind: NotificationKind;
  title: string;
  body: string;
  public_slug: string | null;
  jobs?: NotificationDigestJob[] | null;
  created_at: string | null;
  read_at: string | null;
}
