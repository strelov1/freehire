import type { components } from '#lib/generated/api.js';

import type { Card as JobCard } from '../generated/contracts';

/** A signed-in user's interaction with one job: when they viewed it, saved it
 *  for later, and (once they confirm an application) applied. `saved_at` and
 *  `applied_at` are null until then. Returned by the view/apply/save endpoints. */
export interface UserJob {
  job_id: number;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  // Swipe-mode triage mark; null until the job is dismissed (swipe left).
  dismissed_at: string | null;
  // Application pipeline stage + free-text notes; null until set via `track`.
  stage: string | null;
  notes: string | null;
}

/** One item of the my-jobs listing: the job in the shared wire shape with the
 *  caller's interaction timestamps riding alongside. */
export interface MyJob {
  /** First save or application creation, independent of later pipeline activity. */
  added_to_board_at?: string | null;
  /** Addresses the row. The board keys, routes and opens by this rather than by the
   *  posting's slug: an application whose posting was pruned has no slug to borrow. */
  id: string;
  /** Read from the application record, so a card renders whether or not `job` is there. */
  company_slug: string;
  role_title: string;
  /** The card projection of the posting — what a row draws. Null once the catalogue has
   *  removed the posting: the application outlives it. The full posting, description
   *  included, is on the single-application read (`getTrackedApplication`), which the
   *  panel already fetches; the listing carrying it cost 84% of its own payload. */
  job: JobCard | null;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  stage: string | null;
  notes: string | null;
  /** Live inbox messages linked to this job — the board card's ✉ badge. 0 for
   *  users without a connected mailbox / no linked mail. */
  email_count: number;
  /** When this application last moved: its apply date, or the newest message
   *  linked to it when that is later. Null on any row that is not an
   *  application awaiting a reply. */
  last_activity_at: string | null;
  /** Whole days since last_activity_at. Null together with the others. */
  days_silent: number | null;
  /** 'active' — inside the tolerated silence for its stage; 'silent' — past it;
   *  'unconfirmed' — it would read as silent, but mail awaiting confirmation may
   *  say otherwise. Null means nothing is owed here (viewed/saved only, or a
   *  settled application), which is not the same as owed-and-answered-promptly. */
  silence_state: 'active' | 'silent' | 'unconfirmed' | null;
  /** When the candidate last recorded chasing this application (RFC3339), or null
   *  for one never chased. Deliberately independent of the silence fields above: a
   *  chase is not a reply, so a chased application keeps counting its silence and
   *  the card shows both readings. */
  followed_up_at: string | null;
  /** When somebody last opened a CV sent for this application. Outside the silence derivation:
   *  a recruiter reading a CV is not a reply. */
  cv_opened_at: string | null;
  /** The seven-value status of this job's live auto-apply attempt — the board card's own
   *  "needs your review" badge (openspec/changes/auto-apply-review-tracking). Absent when
   *  there is no live attempt. The full answer preview lives on `TrackedApplication.auto_apply`
   *  instead, which the drawer fetches separately. */
  auto_apply_status?: AutoApplyStatus;
}

/** The seven-value candidate-facing status for a live auto-apply attempt. */
type AutoApplyStatus =
  | 'tailoring'
  | 'pending_review'
  | 'approved'
  | 'blocked'
  | 'declined'
  | 'failed'
  /** The tailoring run itself gave up without producing a CV — distinct from `failed`,
   *  which is a submission that gave up with one in hand. */
  | 'tailor_failed';

/** One resolved question/answer pair in an auto-apply answer preview. */
interface AutoApplyPreviewField {
  label: string;
  value: string;
}

/** One required question an auto-apply answer preview has no answer for yet.
 *  `will_draft_at_submission` distinguishes "the real submission will fill this in
 *  automatically" from "nothing can answer this". */
export interface AutoApplyPreviewPending {
  label: string;
  will_draft_at_submission: boolean;
}

/** The candidate-facing snapshot of what an unattended auto-apply submission would send —
 *  computed once, before review, never a live guess made when the drawer opens. */
interface AutoApplyResolvedPreview {
  fields: AutoApplyPreviewField[];
  pending?: AutoApplyPreviewPending[];
}

/** One application question an auto-apply attempt could not answer — candidate-facing
 *  detail only; the attempt's internal error text is never part of this shape. */
interface AutoApplyUnmappedField {
  id: string;
  label: string;
  required: boolean;
  reason: string;
}

/** What a tracked job's own drawer read (`TrackedApplication.auto_apply`) surfaces about
 *  its live auto-apply attempt (openspec/changes/auto-apply-review-tracking).
 *  `resolved_preview` is present only when `status` is `pending_review`; `unmapped` only
 *  when `status` is `blocked`. */
export interface AutoApplyReviewInfo {
  status: AutoApplyStatus;
  /** Addresses the attempt for `api.reviewAutoApply(queue_id, decision)`. */
  queue_id: number;
  resolved_preview?: AutoApplyResolvedPreview;
  unmapped?: AutoApplyUnmappedField[];
  /** Set on a `failed` attempt the employer may already hold. Such an attempt is never
   *  retried — the server refuses — so no surface offers it. */
  may_have_reached_employer?: boolean;
}

/** What a connected MCP agent may do beyond reading and tracking. */
export interface AgentPermissions {
  /** Whether a connected agent may start, review, retry and cancel auto-apply attempts. */
  agents_may_apply: boolean;
}

/** One answer the candidate has banked for a screening question. */
export type BankedAnswer = components['schemas']['BankedAnswer'];
