import type { components } from '#lib/generated/api.js';
/** The account-level notification rule: whether notifications are on, and the
 *  channels to deliver over. Gates saved-job reminders and both lifecycle nudges
 *  (follow-up, interview-prep) — one shared rule, no per-kind or per-job control. */
export type NotificationSettings = components['schemas']['NotificationSettings'];

/** The notification-center event kinds a `user_notifications` row can carry — the kinds
 *  internal/engage/{notify,reminder,nudge,buddy,messaging} and auto-apply's own write paths
 *  record (`auto_apply_tailor_ready` only on historical rows). */
export type NotificationKind = components['schemas']['NotificationKind'];

/** One row in the notification center (GET /me/notifications) — a durable,
 *  readable-by-the-owner record of a delivery event, independent of which channel
 *  carried it. `public_slug` is the job it concerns, or null for a subscription
 *  digest that matched more than one job (nothing single to deep-link to) — that
 *  case instead carries `jobs`, a snapshot of what matched as of delivery (only
 *  present on the list response for a multi-job digest; absent/null otherwise).
 *  `read_at` is null until the caller marks it read. */
export type NotificationItem = components['schemas']['Notification'];
