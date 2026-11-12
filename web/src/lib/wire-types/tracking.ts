import type { components } from '#lib/generated/api.js';

import type { Card as JobCard } from '../generated/contracts';

/** A signed-in user's interaction with one job: when they viewed it, saved it
 *  for later, and (once they confirm an application) applied. `saved_at` and
 *  `applied_at` are null until then. Returned by the view/apply/save endpoints. */
export type UserJob = components['schemas']['Interaction'];

/** One item of the my-jobs listing: the job in the shared wire shape with the
 *  caller's interaction timestamps riding alongside. */
export type MyJob = Omit<components['schemas']['TrackedJob'], 'job'> & {
  /** The card projection of the posting — what a row draws. Null once the catalogue has
   *  removed the posting: the application outlives it. The full posting, description
   *  included, is on the single-application read (`getTrackedApplication`), which the
   *  panel already fetches; the listing carrying it cost 84% of its own payload. The
   *  contract leaves it raw: the card type is generated from Go, not from the contract. */
  job: JobCard | null;
};

/** One required question an auto-apply answer preview has no answer for yet.
 *  `will_draft_at_submission` distinguishes "the real submission will fill this in
 *  automatically" from "nothing can answer this". */
export type AutoApplyPreviewPending = components['schemas']['AutoApplyPreviewPending'];

/** What a tracked job's own drawer read (`TrackedApplication.auto_apply`) surfaces about
 *  its live auto-apply attempt (openspec/changes/auto-apply-review-tracking).
 *  `resolved_preview` is present only when `status` is `pending_review`; `unmapped` only
 *  when `status` is `blocked`. `queue_id` addresses the attempt for
 *  `api.reviewAutoApply(queue_id, decision)`; `may_have_reached_employer` is set on a
 *  `failed` attempt the employer may already hold, which is never retried. */
export type AutoApplyReviewInfo = components['schemas']['AutoApplyReviewInfo'];

/** What a connected MCP agent may do beyond reading and tracking. */
export type AgentPermissions = components['schemas']['AgentPermissions'];

/** One answer the candidate has banked for a screening question. */
export type BankedAnswer = components['schemas']['BankedAnswer'];
